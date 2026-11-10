"""Leitura do XML da NF-e para estruturas Python simples.

Usamos defusedxml em vez de xml.etree puro: XML vindo de fora pode conter
ataques como XXE (entidade externa que lê arquivos do servidor) ou
"billion laughs" (entidades que explodem a memória). O defusedxml bloqueia
esses casos.

Só extraímos os campos que as regras usam. O layout completo da NF-e 4.00
tem centenas de campos; não faz sentido mapear todos.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from decimal import Decimal, InvalidOperation
from xml.etree.ElementTree import Element

from defusedxml import ElementTree as SafeET

NS = {"nfe": "http://www.portalfiscal.inf.br/nfe"}


class XMLInvalidoError(Exception):
    """Erro permanente: reprocessar não vai resolver."""


@dataclass
class Item:
    numero: int          # atributo nItem
    codigo: str          # cProd
    descricao: str       # xProd
    ncm: str             # NCM
    cfop: str            # CFOP
    valor_produto: Decimal  # vProd


@dataclass
class NotaFiscal:
    chave_acesso: str
    tipo_operacao: str   # tpNF: 0 = entrada, 1 = saída
    destino: str         # idDest: 1 = interna, 2 = interestadual, 3 = exterior
    cnpj_emitente: str
    uf_emitente: str
    uf_destinatario: str
    total_produtos: Decimal  # ICMSTot/vProd
    itens: list[Item] = field(default_factory=list)


def ler_nfe(conteudo: bytes) -> NotaFiscal:
    try:
        raiz = SafeET.fromstring(conteudo)
    except Exception as exc:  # defusedxml levanta tipos variados
        raise XMLInvalidoError(f"XML mal formado ou inseguro: {exc}") from exc

    inf = raiz.find(".//nfe:infNFe", NS)
    if inf is None:
        raise XMLInvalidoError("tag infNFe não encontrada (namespace da NF-e esperado)")

    itens = [
        Item(
            numero=int(det.get("nItem", "0")),
            codigo=_texto(det, "nfe:prod/nfe:cProd"),
            descricao=_texto(det, "nfe:prod/nfe:xProd"),
            ncm=_texto(det, "nfe:prod/nfe:NCM"),
            cfop=_texto(det, "nfe:prod/nfe:CFOP"),
            valor_produto=_decimal(det, "nfe:prod/nfe:vProd"),
        )
        for det in inf.findall("nfe:det", NS)
    ]

    return NotaFiscal(
        chave_acesso=inf.get("Id", "").removeprefix("NFe"),
        tipo_operacao=_texto(inf, "nfe:ide/nfe:tpNF"),
        destino=_texto(inf, "nfe:ide/nfe:idDest"),
        cnpj_emitente=_texto(inf, "nfe:emit/nfe:CNPJ"),
        uf_emitente=_texto(inf, "nfe:emit/nfe:enderEmit/nfe:UF"),
        uf_destinatario=_texto(inf, "nfe:dest/nfe:enderDest/nfe:UF"),
        total_produtos=_decimal(inf, "nfe:total/nfe:ICMSTot/nfe:vProd"),
        itens=itens,
    )


def _texto(elemento: Element, caminho: str) -> str:
    achado = elemento.find(caminho, NS)
    return (achado.text or "").strip() if achado is not None else ""


def _decimal(elemento: Element, caminho: str) -> Decimal:
    # Dinheiro sempre em Decimal, nunca float: 0.1 + 0.2 != 0.3 em float.
    valor = _texto(elemento, caminho)
    try:
        return Decimal(valor) if valor else Decimal("0")
    except InvalidOperation as exc:
        raise XMLInvalidoError(f"valor numérico inválido em {caminho}: {valor!r}") from exc
