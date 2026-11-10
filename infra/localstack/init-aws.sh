#!/bin/bash
# Executado pelo LocalStack quando fica pronto (pasta ready.d).
# Cria localmente os mesmos recursos que o Terraform criará na AWS real.
set -euo pipefail

BUCKET=auditor-nfe-xmls
FILA=auditor-nfe-auditorias
DLQ=auditor-nfe-auditorias-dlq

awslocal s3 mb "s3://${BUCKET}"

# Fila de mensagens mortas: recebe o que falhou 3 vezes seguidas.
awslocal sqs create-queue --queue-name "${DLQ}"
DLQ_ARN=$(awslocal sqs get-queue-attributes \
  --queue-url "http://localhost:4566/000000000000/${DLQ}" \
  --attribute-names QueueArn --query 'Attributes.QueueArn' --output text)

# Fila principal:
#  - VisibilityTimeout 120s: tempo que o worker tem para processar antes de
#    a mensagem "reaparecer" para outro worker (precisa ser maior que o
#    tempo de uma auditoria com LLM).
#  - maxReceiveCount 3: depois de 3 tentativas, vai para a DLQ.
awslocal sqs create-queue --queue-name "${FILA}" --attributes "{
  \"VisibilityTimeout\": \"120\",
  \"RedrivePolicy\": \"{\\\"deadLetterTargetArn\\\":\\\"${DLQ_ARN}\\\",\\\"maxReceiveCount\\\":\\\"3\\\"}\"
}"

echo "LocalStack pronto: bucket ${BUCKET}, filas ${FILA} e ${DLQ}"
