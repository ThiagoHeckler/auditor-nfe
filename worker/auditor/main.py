"""Loop principal do worker: consome a fila SQS e audita cada nota.

Ciclo de vida de uma mensagem:
1. receive_message com long polling (espera até 20s por mensagem: menos
   chamadas vazias e menor custo do que ficar perguntando toda hora).
2. Processa: baixa o XML do S3, roda o grafo, grava os achados.
3. Sucesso            -> apaga a mensagem da fila.
   Erro permanente    -> (XML inválido) marca ERRO e apaga: tentar de novo
                         não resolve.
   Erro temporário    -> (banco/S3/LLM fora do ar) NÃO apaga: a mensagem
                         volta à fila após o visibility timeout e é tentada
                         de novo. Depois de 3 tentativas o SQS a move para a
                         DLQ (fila de mensagens mortas) para análise manual.
"""

from __future__ import annotations

import json
import logging
import os
import signal
import sys

import boto3

from auditor.grafo import auditar
from auditor.mensagem import AuditoriaSolicitada
from auditor.nfe import XMLInvalidoError
from auditor.repositorio import Repositorio

logging.basicConfig(
    level=logging.INFO,
    format='{"nivel":"%(levelname)s","msg":"%(message)s","logger":"%(name)s"}',
    stream=sys.stdout,
)
log = logging.getLogger("worker")

_rodando = True


def _parar(*_):
    global _rodando
    log.info("sinal recebido, terminando a mensagem atual e saindo")
    _rodando = False


def processar(msg: AuditoriaSolicitada, s3, bucket: str, repo: Repositorio) -> None:
    if not repo.marcar_processando(msg.nota_id):
        log.info(f"nota {msg.nota_id} já concluída, ignorando mensagem repetida")
        return
    xml = s3.get_object(Bucket=bucket, Key=msg.s3_key)["Body"].read()
    achados = auditar(xml)
    repo.concluir(msg.nota_id, achados)
    log.info(f"nota {msg.nota_id} auditada: {len(achados)} achado(s)")


def main() -> None:
    signal.signal(signal.SIGTERM, _parar)
    signal.signal(signal.SIGINT, _parar)

    endpoint = os.getenv("AWS_ENDPOINT_URL") or None  # LocalStack no ambiente local
    regiao = os.getenv("AWS_REGION", "us-east-1")
    s3 = boto3.client("s3", endpoint_url=endpoint, region_name=regiao)
    sqs = boto3.client("sqs", endpoint_url=endpoint, region_name=regiao)
    fila = os.environ["SQS_QUEUE_URL"]
    bucket = os.environ["S3_BUCKET"]
    repo = Repositorio(os.environ["DATABASE_URL"])

    log.info("worker iniciado, aguardando mensagens")
    while _rodando:
        resposta = sqs.receive_message(QueueUrl=fila, MaxNumberOfMessages=5, WaitTimeSeconds=20)
        for m in resposta.get("Messages", []):
            try:
                msg = AuditoriaSolicitada.de_json(m["Body"])
            except (json.JSONDecodeError, KeyError) as exc:
                log.error(f"mensagem com formato inválido, descartando: {exc}")
                sqs.delete_message(QueueUrl=fila, ReceiptHandle=m["ReceiptHandle"])
                continue

            try:
                processar(msg, s3, bucket, repo)
            except XMLInvalidoError as exc:
                log.warning(f"nota {msg.nota_id} com XML inválido: {exc}")
                repo.marcar_erro(msg.nota_id, str(exc))
            except Exception:
                log.exception(f"falha temporária na nota {msg.nota_id}; será tentada de novo")
                continue  # não apaga: volta para a fila

            sqs.delete_message(QueueUrl=fila, ReceiptHandle=m["ReceiptHandle"])


if __name__ == "__main__":
    main()
