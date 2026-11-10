"""Contrato da mensagem entre a API (Go) e o worker (Python).

Espelha queue.AuditoriaSolicitada em api/internal/queue/sqs.go.
Se um lado mudar, o outro precisa mudar junto.
"""

from __future__ import annotations

import json
from dataclasses import dataclass


@dataclass(frozen=True)
class AuditoriaSolicitada:
    nota_id: str
    s3_key: str

    @classmethod
    def de_json(cls, corpo: str) -> AuditoriaSolicitada:
        dados = json.loads(corpo)
        return cls(nota_id=dados["nota_id"], s3_key=dados["s3_key"])
