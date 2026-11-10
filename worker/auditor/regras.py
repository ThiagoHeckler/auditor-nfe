"""Regras fiscais determinísticas.

Cada regra é uma função pura: recebe a NotaFiscal e devolve uma lista de
Achados. Sem banco, sem rede, sem LLM — por isso são rápidas, baratas e
fáceis de testar. O LLM só entra depois, para os casos que as regras
marcam como ALERTA (ambíguos), e nunca substitui uma regra objetiva.

Para adicionar uma regra: escreva a função e inclua em REGRAS no final.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from decimal import Decimal
from typing import Literal

from auditor.nfe import NotaFiscal

Severidade = Literal["INFO", "ALERTA", "ERRO"]


@dataclass(frozen=True)
class Achado:
    regra: str
    severidade: Severidade
    mensagem: str
    campo: str | None = None
    origem: Literal["REGRA", "LLM"] = "REGRA"


def _dv_modulo11(numeros: str, pesos_ate: int) -> int:
    """Dígito verificador módulo 11, pesos de 2 até `pesos_ate` da direita
    para a esquerda (reiniciando). Usado na chave de acesso e no CNPJ."""
    soma, peso = 0, 2
    for digito in reversed(numeros):
        soma += int(digito) * peso
        peso = 2 if peso == pesos_ate else peso + 1
    resto = soma % 11
    return 0 if resto < 2 else 11 - resto


# --- R001 ------------------------------------------------------------------
def r001_dv_chave_acesso(nota: NotaFiscal) -> list[Achado]:
    """O 44º dígito da chave é um DV módulo 11 dos 43 anteriores."""
    chave = nota.chave_acesso
    if len(chave) != 44 or not chave.isdigit():
        return [Achado("R001_DV_CHAVE", "ERRO", "Chave de acesso não tem 44 dígitos.", "infNFe@Id")]
    esperado = _dv_modulo11(chave[:43], pesos_ate=9)
    if int(chave[43]) != esperado:
        return [Achado("R001_DV_CHAVE", "ERRO",
                       f"Dígito verificador da chave inválido: esperado {esperado}, veio {chave[43]}.",
                       "infNFe@Id")]
    return []


# --- R002 ------------------------------------------------------------------
# 1º dígito do CFOP: 1/5 = mesma UF, 2/6 = outra UF, 3/7 = exterior.
_DESTINO_POR_DIGITO = {"1": "1", "5": "1", "2": "2", "6": "2", "3": "3", "7": "3"}
_DESTINO_NOME = {"1": "interna", "2": "interestadual", "3": "com o exterior"}


def r002_cfop_x_destino(nota: NotaFiscal) -> list[Achado]:
    """O CFOP de cada item deve concordar com o idDest da nota."""
    achados = []
    for item in nota.itens:
        destino_cfop = _DESTINO_POR_DIGITO.get(item.cfop[:1])
        if destino_cfop and nota.destino and destino_cfop != nota.destino:
            achados.append(Achado(
                "R002_CFOP_DESTINO", "ERRO",
                f"CFOP {item.cfop} indica operação {_DESTINO_NOME[destino_cfop]}, "
                f"mas a nota está como {_DESTINO_NOME.get(nota.destino, nota.destino)} (idDest={nota.destino}).",
                f"det[{item.numero}].prod.CFOP",
            ))
    return achados


# --- R003 ------------------------------------------------------------------
def r003_cfop_x_tipo_operacao(nota: NotaFiscal) -> list[Achado]:
    """CFOP iniciado em 1/2/3 é entrada; 5/6/7 é saída. Deve bater com tpNF."""
    achados = []
    for item in nota.itens:
        primeiro = item.cfop[:1]
        if primeiro in "123" and nota.tipo_operacao == "1":
            tipo_cfop, tipo_nota = "entrada", "saída"
        elif primeiro in "567" and nota.tipo_operacao == "0":
            tipo_cfop, tipo_nota = "saída", "entrada"
        else:
            continue
        achados.append(Achado(
            "R003_CFOP_TIPO", "ERRO",
            f"CFOP {item.cfop} é de {tipo_cfop}, mas a nota é de {tipo_nota} (tpNF={nota.tipo_operacao}).",
            f"det[{item.numero}].prod.CFOP",
        ))
    return achados


# --- R004 ------------------------------------------------------------------
def r004_formato_ncm(nota: NotaFiscal) -> list[Achado]:
    """NCM tem 8 dígitos. '00000000' é aceito pela SEFAZ só em casos
    específicos (ex.: serviços), então vira ALERTA para análise."""
    achados = []
    for item in nota.itens:
        campo = f"det[{item.numero}].prod.NCM"
        if len(item.ncm) != 8 or not item.ncm.isdigit():
            achados.append(Achado("R004_NCM", "ERRO", f"NCM '{item.ncm}' não tem 8 dígitos numéricos.", campo))
        elif item.ncm == "00000000":
            achados.append(Achado("R004_NCM", "ALERTA",
                                  f"Item '{item.descricao}' com NCM genérico 00000000; verificar classificação.",
                                  campo))
    return achados


# --- R005 ------------------------------------------------------------------
def r005_soma_itens(nota: NotaFiscal) -> list[Achado]:
    """A soma de vProd dos itens deve ser igual ao total vProd da nota."""
    soma = sum((i.valor_produto for i in nota.itens), start=Decimal("0"))
    if soma != nota.total_produtos:
        return [Achado("R005_TOTAL_PRODUTOS", "ERRO",
                       f"Soma dos itens ({soma}) difere do total de produtos ({nota.total_produtos}).",
                       "total.ICMSTot.vProd")]
    return []


# --- R006 ------------------------------------------------------------------
def r006_dv_cnpj_emitente(nota: NotaFiscal) -> list[Achado]:
    """CNPJ tem dois dígitos verificadores módulo 11 (pesos 2 a 9)."""
    cnpj = nota.cnpj_emitente
    if len(cnpj) != 14 or not cnpj.isdigit() or len(set(cnpj)) == 1:
        return [Achado("R006_CNPJ_EMITENTE", "ERRO", f"CNPJ do emitente '{cnpj}' inválido.", "emit.CNPJ")]
    dv1 = _dv_modulo11(cnpj[:12], pesos_ate=9)
    dv2 = _dv_modulo11(cnpj[:12] + str(dv1), pesos_ate=9)
    if cnpj[12:] != f"{dv1}{dv2}":
        return [Achado("R006_CNPJ_EMITENTE", "ERRO", f"Dígitos verificadores do CNPJ {cnpj} inválidos.",
                       "emit.CNPJ")]
    return []


REGRAS: list[Callable[[NotaFiscal], list[Achado]]] = [
    r001_dv_chave_acesso,
    r002_cfop_x_destino,
    r003_cfop_x_tipo_operacao,
    r004_formato_ncm,
    r005_soma_itens,
    r006_dv_cnpj_emitente,
]


def aplicar_regras(nota: NotaFiscal) -> list[Achado]:
    achados: list[Achado] = []
    for regra in REGRAS:
        achados.extend(regra(nota))
    return achados
