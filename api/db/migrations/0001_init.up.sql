-- 0001_init: tabelas iniciais do Auditor NF-e
--
-- notas   -> uma linha por arquivo XML enviado
-- achados -> problemas encontrados pelo worker em cada nota

CREATE TABLE IF NOT EXISTS notas (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Chave de acesso de 44 dígitos. UNIQUE garante idempotência:
    -- a mesma NF-e enviada duas vezes não gera duas auditorias.
    chave_acesso   CHAR(44)    NOT NULL UNIQUE,
    nome_arquivo   TEXT        NOT NULL,
    s3_key         TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'RECEBIDA'
                   CHECK (status IN ('RECEBIDA', 'PROCESSANDO', 'CONCLUIDA', 'ERRO')),
    mensagem_erro  TEXT,
    criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notas_status_criado ON notas (status, criado_em DESC);

CREATE TABLE IF NOT EXISTS achados (
    id          BIGSERIAL   PRIMARY KEY,
    nota_id     UUID        NOT NULL REFERENCES notas (id) ON DELETE CASCADE,
    regra       TEXT        NOT NULL,          -- ex.: R002_CFOP_DESTINO
    severidade  TEXT        NOT NULL CHECK (severidade IN ('INFO', 'ALERTA', 'ERRO')),
    campo       TEXT,                          -- ex.: det[1].prod.CFOP
    mensagem    TEXT        NOT NULL,
    origem      TEXT        NOT NULL DEFAULT 'REGRA' CHECK (origem IN ('REGRA', 'LLM')),
    criado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_achados_nota ON achados (nota_id);
