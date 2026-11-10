from dataclasses import replace
from decimal import Decimal
from pathlib import Path

import pytest

from auditor import llm
from auditor.grafo import auditar, rota_apos_regras
from auditor.nfe import XMLInvalidoError, ler_nfe
from auditor.regras import (
    Achado,
    r001_dv_chave_acesso,
    r005_soma_itens,
    r006_dv_cnpj_emitente,
)

SAMPLES = Path(__file__).resolve().parents[2] / "samples"


@pytest.fixture
def nota_valida():
    return ler_nfe((SAMPLES / "nfe-valida.xml").read_bytes())


def test_leitura_do_xml(nota_valida):
    assert nota_valida.chave_acesso == "42251011222333000181550010000012341000012344"
    assert nota_valida.uf_emitente == "SC"
    assert len(nota_valida.itens) == 2
    assert nota_valida.total_produtos == Decimal("199.90")


def test_nota_valida_nao_gera_achados():
    assert auditar((SAMPLES / "nfe-valida.xml").read_bytes()) == []


def test_nota_com_problemas_gera_achados_esperados():
    achados = auditar((SAMPLES / "nfe-com-problemas.xml").read_bytes())
    regras = sorted(a.regra for a in achados)
    assert regras == ["R002_CFOP_DESTINO", "R003_CFOP_TIPO", "R004_NCM", "R005_TOTAL_PRODUTOS"]
    ncm = next(a for a in achados if a.regra == "R004_NCM")
    assert ncm.severidade == "ALERTA"
    assert ncm.campo == "det[2].prod.NCM"


def test_r001_detecta_digito_verificador_errado(nota_valida):
    chave_errada = nota_valida.chave_acesso[:43] + "9"
    achados = r001_dv_chave_acesso(replace(nota_valida, chave_acesso=chave_errada))
    assert len(achados) == 1 and "esperado 4" in achados[0].mensagem


@pytest.mark.parametrize("cnpj,valido", [
    ("11222333000181", True),
    ("11222333000182", False),
    ("11111111111111", False),  # todos iguais: passa no cálculo, mas é inválido
    ("123", False),
])
def test_r006_cnpj(nota_valida, cnpj, valido):
    achados = r006_dv_cnpj_emitente(replace(nota_valida, cnpj_emitente=cnpj))
    assert (achados == []) == valido


def test_r005_usa_decimal_sem_erro_de_arredondamento(nota_valida):
    # 0.1 + 0.2 em float dá 0.30000000000000004; em Decimal fecha certinho.
    itens = [replace(nota_valida.itens[0], valor_produto=Decimal("0.1")),
             replace(nota_valida.itens[1], valor_produto=Decimal("0.2"))]
    nota = replace(nota_valida, itens=itens, total_produtos=Decimal("0.3"))
    assert r005_soma_itens(nota) == []


def test_xml_mal_formado():
    with pytest.raises(XMLInvalidoError):
        ler_nfe(b"<nfeProc><NFe>")


def test_xml_com_entidade_externa_e_bloqueado():
    xxe = b"""<?xml version="1.0"?>
    <!DOCTYPE x [<!ENTITY segredo SYSTEM "file:///etc/passwd">]>
    <nfeProc xmlns="http://www.portalfiscal.inf.br/nfe"><NFe><infNFe Id="&segredo;"/></NFe></nfeProc>"""
    with pytest.raises(XMLInvalidoError):
        ler_nfe(xxe)


def test_rota_so_chama_llm_com_alerta_e_flag_ligada(monkeypatch):
    alerta = {"achados": [Achado("R004_NCM", "ALERTA", "x")]}
    so_erro = {"achados": [Achado("R002_CFOP_DESTINO", "ERRO", "x")]}

    monkeypatch.setattr(llm, "habilitado", lambda: False)
    assert rota_apos_regras(alerta) == "__end__"

    monkeypatch.setattr(llm, "habilitado", lambda: True)
    assert rota_apos_regras(alerta) == "analisar_com_llm"
    assert rota_apos_regras(so_erro) == "__end__"
