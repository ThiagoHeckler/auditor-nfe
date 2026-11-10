"""Acesso ao PostgreSQL pelo worker (psycopg 3, SQL puro).

Mesmas tabelas da API (o esquema é dono da API, em api/db/migrations).
"""

from __future__ import annotations

import psycopg

from auditor.regras import Achado


class Repositorio:
    def __init__(self, dsn: str) -> None:
        self._conn = psycopg.connect(dsn, autocommit=True)

    def marcar_processando(self, nota_id: str) -> bool:
        """Devolve False se a nota já foi concluída (mensagem repetida).

        O SQS garante entrega "pelo menos uma vez": a mesma mensagem pode
        chegar duas vezes. Este UPDATE condicional torna o worker idempotente.
        """
        cur = self._conn.execute(
            """UPDATE notas SET status = 'PROCESSANDO', atualizado_em = now()
               WHERE id = %s AND status IN ('RECEBIDA', 'PROCESSANDO')""",
            (nota_id,),
        )
        return cur.rowcount == 1

    def concluir(self, nota_id: str, achados: list[Achado]) -> None:
        # Uma transação: ou grava todos os achados + status, ou nada.
        with self._conn.transaction():
            self._conn.execute("DELETE FROM achados WHERE nota_id = %s", (nota_id,))
            with self._conn.cursor() as cur:
                cur.executemany(
                    """INSERT INTO achados (nota_id, regra, severidade, campo, mensagem, origem)
                       VALUES (%s, %s, %s, %s, %s, %s)""",
                    [(nota_id, a.regra, a.severidade, a.campo, a.mensagem, a.origem) for a in achados],
                )
            self._conn.execute(
                "UPDATE notas SET status = 'CONCLUIDA', mensagem_erro = NULL, atualizado_em = now() WHERE id = %s",
                (nota_id,),
            )

    def marcar_erro(self, nota_id: str, mensagem: str) -> None:
        self._conn.execute(
            "UPDATE notas SET status = 'ERRO', mensagem_erro = %s, atualizado_em = now() WHERE id = %s",
            (mensagem[:500], nota_id),
        )
