-- Consultas usadas pela API. O sqlc lê este arquivo e gera
-- código Go tipado em internal/db (não edite os arquivos gerados).

-- name: CriarNota :one
INSERT INTO notas (chave_acesso, nome_arquivo, s3_key)
VALUES ($1, $2, $3)
RETURNING *;

-- name: BuscarNotaPorID :one
SELECT * FROM notas
WHERE id = $1;

-- name: BuscarNotaPorChave :one
SELECT * FROM notas
WHERE chave_acesso = $1;

-- name: ListarNotas :many
SELECT * FROM notas
ORDER BY criado_em DESC
LIMIT $1 OFFSET $2;

-- name: MarcarNotaComErro :exec
UPDATE notas
SET status = 'ERRO', mensagem_erro = $2, atualizado_em = now()
WHERE id = $1;

-- name: ListarAchadosDaNota :many
SELECT * FROM achados
WHERE nota_id = $1
ORDER BY id;
