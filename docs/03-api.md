# 3. API (contrato HTTP)

Base local: `http://localhost:8088` (porta do host em `API_HOST_PORT`, ver `.env`). Todas as respostas são JSON. Erros
seguem sempre o formato:

```json
{ "erro": "mensagem legível" }
```

Toda resposta traz o cabeçalho `X-Request-ID`. Se o cliente mandar um, ele é
reaproveitado. O mesmo id aparece nos logs, o que permite rastrear uma
requisição de ponta a ponta.

---

## `POST /v1/notas`: enviar NF-e

`multipart/form-data` com o arquivo no campo **`arquivo`** (máx.
`MAX_UPLOAD_MB`, padrão 2 MB).

```bash
curl -i -F arquivo=@samples/nfe-com-problemas.xml localhost:8088/v1/notas
```

**202 Accepted**: recebida, auditoria em andamento. Cabeçalho
`Location: /v1/notas/{id}`.

```json
{
  "id": "ff5f0c5c-4390-461e-9547-57a435436bde",
  "chave_acesso": "42251011222333000181550010000012351000012350",
  "nome_arquivo": "nfe-com-problemas.xml",
  "status": "RECEBIDA",
  "criado_em": "2026-11-10T09:47:09.80381-03:00",
  "atualizado_em": "2026-11-10T09:47:09.80381-03:00"
}
```

| Status | Quando |
|---|---|
| 202 | Nota aceita e enfileirada |
| 400 | Campo `arquivo` ausente ou requisição não é multipart |
| 409 | Essa chave de acesso já foi enviada. O corpo traz a nota existente em `nota` |
| 413 | Arquivo maior que o limite |
| 422 | Não é um XML de NF-e (mal formado, sem `infNFe`, chave sem 44 dígitos) |
| 500 | Falha interna (banco, S3 ou fila). A nota fica com status `ERRO` |

**Por que 202 e não 201?** 201 diz "recurso criado e pronto". Aqui a nota
foi registrada, mas o trabalho (a auditoria) ainda vai acontecer. 202 é o
código HTTP para "aceito para processamento".

**Por que 422 e não 400?** A requisição está bem formada (é multipart, tem o
campo), mas o **conteúdo** não é processável. Muitas APIs usam 400 para
tudo; separar ajuda o cliente a saber o que corrigir.

---

## `GET /v1/notas/{id}`: consultar nota e achados

```bash
curl -s localhost:8088/v1/notas/ff5f0c5c-4390-461e-9547-57a435436bde
```

**200 OK**

```json
{
  "id": "ff5f0c5c-4390-461e-9547-57a435436bde",
  "chave_acesso": "42251011222333000181550010000012351000012350",
  "nome_arquivo": "nfe-com-problemas.xml",
  "status": "CONCLUIDA",
  "criado_em": "...",
  "atualizado_em": "...",
  "achados": [
    {
      "regra": "R002_CFOP_DESTINO",
      "severidade": "ERRO",
      "campo": "det[1].prod.CFOP",
      "mensagem": "CFOP 6102 indica operação interestadual, mas a nota está como interna (idDest=1).",
      "origem": "REGRA"
    },
    {
      "regra": "R004_NCM",
      "severidade": "ALERTA",
      "campo": "det[2].prod.NCM",
      "mensagem": "Item 'Kit diversos' com NCM genérico 00000000; verificar classificação.",
      "origem": "REGRA"
    }
  ]
}
```

- `status`: `RECEBIDA` | `PROCESSANDO` | `CONCLUIDA` | `ERRO`
- `mensagem_erro`: só aparece quando `status = ERRO`
- `achados`: omitido enquanto não houver achados (nota em processamento ou
  nota sem problemas)
- `origem`: `REGRA` (determinística) ou `LLM`

| Status | Quando |
|---|---|
| 200 | Encontrada |
| 404 | Id inexistente ou mal formado |

O cliente faz *polling* nesta rota até o status sair de `RECEBIDA` ou
`PROCESSANDO`. Alternativa futura: webhook ou SSE (ver roadmap).

---

## `GET /v1/notas`: listar

```bash
curl -s "localhost:8088/v1/notas?limite=20&deslocamento=0"
```

```json
{ "dados": [ { "id": "...", "status": "CONCLUIDA", "...": "..." } ], "limite": 20, "deslocamento": 0 }
```

- `limite`: 1 a 100 (padrão 20). Valores fora da faixa são ajustados.
- `deslocamento`: padrão 0.
- Ordenação: mais recentes primeiro. A listagem não traz achados (para isso,
  use a rota por id).

---

## `GET /health` e `GET /ready`

| Rota | Pergunta que responde | Verifica |
|---|---|---|
| `/health` | "o processo está vivo?" | nada externo; sempre 200 se respondeu |
| `/ready` | "posso receber tráfego?" | `ping` no PostgreSQL. 503 se cair |

Orquestradores (ECS, Kubernetes, Cloud Run) usam essa separação: se o
*health* falha, o container é reiniciado; se o *ready* falha, ele só deixa de
receber tráfego até o banco voltar.
