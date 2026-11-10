"""Fluxo de auditoria como um grafo LangGraph.

    [ler_xml] -> [aplicar_regras] --(tem ALERTA e LLM ligado?)--> [analisar_com_llm] -> FIM
                                  \\--------------(não)-----------------------------> FIM

Por que um grafo e não uma função com if/else?
- Cada etapa é um nó isolado e testável.
- O estado (EstadoAuditoria) é explícito: dá para ver o que entra e sai.
- Fica fácil crescer: um nó de RAG antes do LLM, um nó de "revisão humana"
  (interrupt do LangGraph) quando a confiança for baixa, tentativas, etc.
"""

from __future__ import annotations

from typing import TypedDict

from langgraph.graph import END, START, StateGraph

from auditor import llm
from auditor.nfe import NotaFiscal, ler_nfe
from auditor.regras import Achado, aplicar_regras


class EstadoAuditoria(TypedDict, total=False):
    xml: bytes
    nota: NotaFiscal
    achados: list[Achado]


def no_ler_xml(estado: EstadoAuditoria) -> EstadoAuditoria:
    return {"nota": ler_nfe(estado["xml"])}


def no_aplicar_regras(estado: EstadoAuditoria) -> EstadoAuditoria:
    return {"achados": aplicar_regras(estado["nota"])}


def no_analisar_com_llm(estado: EstadoAuditoria) -> EstadoAuditoria:
    alertas = [a for a in estado["achados"] if a.severidade == "ALERTA"]
    return {"achados": estado["achados"] + llm.analisar(estado["nota"], alertas)}


def rota_apos_regras(estado: EstadoAuditoria) -> str:
    tem_alerta = any(a.severidade == "ALERTA" for a in estado["achados"])
    return "analisar_com_llm" if tem_alerta and llm.habilitado() else END


def construir_grafo():
    g = StateGraph(EstadoAuditoria)
    g.add_node("ler_xml", no_ler_xml)
    g.add_node("aplicar_regras", no_aplicar_regras)
    g.add_node("analisar_com_llm", no_analisar_com_llm)

    g.add_edge(START, "ler_xml")
    g.add_edge("ler_xml", "aplicar_regras")
    g.add_conditional_edges("aplicar_regras", rota_apos_regras, ["analisar_com_llm", END])
    g.add_edge("analisar_com_llm", END)
    return g.compile()


GRAFO = construir_grafo()


def auditar(xml: bytes) -> list[Achado]:
    """Ponto de entrada usado pelo worker e pelos testes."""
    return GRAFO.invoke({"xml": xml})["achados"]
