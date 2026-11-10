# 4. Banco de dados

PostgreSQL 17. Esquema em `api/db/migrations/0001_init.up.sql`.

## Tabelas

### `notas`: uma linha por XML enviado

| Coluna | Tipo | Observação |
|---|---|---|
| `id` | `UUID` PK | gerado pelo banco (`gen_random_uuid()`) |
| `chave_acesso` | `CHAR(44)` **UNIQUE** | garante que a mesma NF-e não é auditada duas vezes |
| `nome_arquivo` | `TEXT` | nome original do upload |
| `s3_key` | `TEXT` | onde o XML está no bucket (`notas/{chave}.xml`) |
| `status` | `TEXT` + `CHECK` | `RECEBIDA`, `PROCESSANDO`, `CONCLUIDA`, `ERRO` |
| `mensagem_erro` | `TEXT` nulo | preenchido só quando `status = ERRO` |
| `criado_em` / `atualizado_em` | `TIMESTAMPTZ` | sempre com fuso |

Índice: `(status, criado_em DESC)`, útil para "listar as notas com ERRO
mais recentes" ou para um job que reprocessa notas presas.

### `achados`: problemas encontrados

| Coluna | Tipo | Observação |
|---|---|---|
| `id` | `BIGSERIAL` PK | |
| `nota_id` | `UUID` FK → `notas.id` | `ON DELETE CASCADE` |
| `regra` | `TEXT` | ex.: `R002_CFOP_DESTINO` |
| `severidade` | `TEXT` + `CHECK` | `INFO`, `ALERTA`, `ERRO` |
| `campo` | `TEXT` nulo | caminho no XML, ex.: `det[2].prod.NCM` |
| `mensagem` | `TEXT` | explicação legível |
| `origem` | `TEXT` + `CHECK` | `REGRA` ou `LLM` |

Índice: `(nota_id)`, porque toda consulta de achados filtra por nota.

## Decisões de modelagem

- **Por que UUID e não serial na nota?** O id aparece na URL pública. Um
  serial deixa adivinhar quantas notas existem e varrer as dos outros
  (`/notas/1`, `/notas/2`...).
- **Por que `CHECK` em vez de `ENUM` do Postgres?** Adicionar um valor num
  `CHECK` é uma migração simples. Mudar um `ENUM` tem mais restrições.
- **Por que o XML não fica no banco?** Arquivos em object storage (S3) são
  mais baratos, não incham backups do banco e podem ter política de
  retenção própria. No banco fica só a referência (`s3_key`).
- **Por que `TIMESTAMPTZ`?** Sem fuso, "10:00" é ambíguo. Com fuso, o
  Postgres guarda em UTC e converte na leitura.

## Quem escreve o quê

| Operação | API (Go) | Worker (Python) |
|---|---|---|
| Criar nota | ✔ | |
| Marcar `ERRO` (falha de S3/fila) | ✔ | |
| Marcar `PROCESSANDO` | | ✔ |
| Gravar achados + `CONCLUIDA` | | ✔ (uma transação) |
| Marcar `ERRO` (XML inválido) | | ✔ |
| Consultar | ✔ | |

O **esquema pertence à API**: migrações ficam em `api/db/migrations`. O
worker só lê e escreve nessas tabelas.

## sqlc: SQL puro, Go tipado

Em vez de um ORM, as consultas são escritas em SQL em
`api/db/queries/notas.sql`, com um comentário de nome:

```sql
-- name: BuscarNotaPorID :one
SELECT * FROM notas WHERE id = $1;
```

O `sqlc generate` lê o esquema e as queries e **gera** `internal/db/*.go`
com structs e funções tipadas:

```go
func (q *Queries) BuscarNotaPorID(ctx context.Context, id pgtype.UUID) (Nota, error)
```

Ganhos:
- erro de SQL (coluna errada, tipo errado) aparece **na geração**, não em
  produção;
- você vê exatamente o SQL que roda (sem "mágica" de ORM, sem N+1 escondido);
- zero reflexão em tempo de execução.

Para mudar uma consulta: edite o `.sql` e rode `make sqlc`. **Nunca edite
`internal/db/` à mão**, porque será sobrescrito.

## Migrações

Hoje o Postgres do Docker aplica `0001_init.up.sql` na primeira subida (pasta
`docker-entrypoint-initdb.d`). Isso só funciona com o volume vazio: para
recriar, use `make down` e depois `make up`.

Próximo passo (roadmap): usar uma ferramenta de migração (`golang-migrate`
ou `goose`) para aplicar migrações novas em bancos existentes, como o RDS.
Os arquivos já seguem o padrão `NNNN_nome.up.sql` / `.down.sql` dessas
ferramentas.
