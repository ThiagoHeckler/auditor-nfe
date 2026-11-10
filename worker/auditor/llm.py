"""Análise complementar por LLM (opcional).

Entra SÓ quando as regras deixaram algum ALERTA (caso ambíguo). O LLM
recebe os dados da nota e os alertas e devolve observações estruturadas.

Decisões importantes (boas para explicar em entrevista):
- Saída estruturada (Pydantic): o modelo devolve JSON validado, não texto
  livre que precisaria ser "parseado" com regex.
- O LLM nunca gera ERRO: suas observações entram como ALERTA ou INFO,
  com origem='LLM'. Decisão fiscal objetiva fica nas regras.
- Desligado por padrão (LLM_HABILITADO=false): o projeto roda e é testável
  sem chave de API e sem custo.
- Próximo passo (Projeto 1): antes de chamar o modelo, buscar trechos da
  legislação no RAG e incluí-los no prompt, para respostas com citação.
"""

from __future__ import annotations

import os
from typing import Literal

from pydantic import BaseModel, Field

from auditor.nfe import NotaFiscal
from auditor.regras import Achado


class Observacao(BaseModel):
    severidade: Literal["INFO", "ALERTA"]
    campo: str | None = Field(None, description="Campo da NF-e, ex.: det[2].prod.NCM")
    mensagem: str = Field(description="Explicação curta e objetiva em português")


class RespostaAnalise(BaseModel):
    observacoes: list[Observacao]


PROMPT = """Você é um auditor fiscal brasileiro revisando uma NF-e.
As regras automáticas marcaram os alertas abaixo. Para cada alerta, diga se
parece um problema real e o que o contador deve verificar. Seja objetivo.
Não invente dados que não estão na nota.

Nota:
- Tipo de operação (tpNF): {tipo}  | Destino (idDest): {destino}
- UF emitente: {uf_emit} | UF destinatário: {uf_dest}
- Itens:
{itens}

Alertas:
{alertas}
"""


def habilitado() -> bool:
    return os.getenv("LLM_HABILITADO", "false").lower() == "true"


def analisar(nota: NotaFiscal, alertas: list[Achado]) -> list[Achado]:
    # Import aqui dentro: quem não usa LLM não precisa da dependência carregada.
    from langchain_anthropic import ChatAnthropic

    modelo = ChatAnthropic(
        model=os.getenv("LLM_MODELO", "claude-haiku-5-5"),
        temperature=0,  # auditoria pede respostas estáveis, não criativas
        max_tokens=1024,
    ).with_structured_output(RespostaAnalise)

    itens = "\n".join(
        f"  {i.numero}. {i.descricao} | NCM {i.ncm} | CFOP {i.cfop} | R$ {i.valor_produto}"
        for i in nota.itens
    )
    texto_alertas = "\n".join(f"- [{a.regra}] {a.campo}: {a.mensagem}" for a in alertas)

    resposta: RespostaAnalise = modelo.invoke(PROMPT.format(
        tipo=nota.tipo_operacao, destino=nota.destino,
        uf_emit=nota.uf_emitente, uf_dest=nota.uf_destinatario,
        itens=itens, alertas=texto_alertas,
    ))
    return [
        Achado("LLM_ANALISE", o.severidade, o.mensagem, o.campo, origem="LLM")
        for o in resposta.observacoes
    ]
