# Auditor NF-e

Serviço que recebe XMLs de NF-e, audita com regras fiscais automáticas e,
quando há casos ambíguos, pede uma segunda opinião a um LLM orquestrado com
**LangGraph**. O processamento é assíncrono: uma **API em Go** recebe os
arquivos e um **worker em Python** faz a auditoria, ligados por fila (**SQS**)
e armazenamento de objetos (**S3**).

> Projeto de portfólio. As NF-e em `samples/` são fictícias.

```
Cliente ──► API Go ──► PostgreSQL (nota RECEBIDA)
              │
              ├──► S3  (guarda o XML)
              └──► SQS (pede auditoria) ──► Worker Python
                                               │  LangGraph:
                                               │  ler XML → regras → [LLM se houver ALERTA]
                                               ▼
                                          PostgreSQL (achados + CONCLUIDA)
Cliente ◄── API Go (GET /v1/notas/{id}) ◄──────┘
```

## O que ele verifica

| Regra | O que checa | Severidade |
|---|---|---|
| R001 | Dígito verificador da chave de acesso (módulo 11) | ERRO |
| R002 | CFOP x destino da operação (interna / interestadual / exterior) | ERRO |
| R003 | CFOP x tipo da nota (entrada / saída) | ERRO |
| R004 | NCM com 8 dígitos; NCM genérico `00000000` | ERRO / ALERTA |
| R005 | Soma dos itens = total de produtos | ERRO |
| R006 | Dígitos verificadores do CNPJ do emitente | ERRO |
| LLM | Analisa os ALERTAS e explica o que verificar (opcional) | ALERTA / INFO |

## Stack

| Camada | Tecnologia | Por quê (resumo) |
|---|---|---|
| API | Go 1.24, `net/http` (sem framework) | binário pequeno, rápido, ótimo para container/Lambda |
| Acesso a dados (Go) | `pgx` + `sqlc` | SQL puro com código Go tipado gerado |
| Worker | Python 3.12, LangGraph, LangChain | ecossistema de IA é Python-first |
| Banco | PostgreSQL 17 | relacional, transações, `UNIQUE` para idempotência |
| Nuvem | AWS S3 + SQS (LocalStack no ambiente local) | desacopla recebimento e processamento |
| Infra local | Docker Compose | sobe tudo com um comando |

Detalhes e justificativas: [docs/06-decisoes-tecnicas.md](docs/06-decisoes-tecnicas.md).

## Rodando localmente

Pré-requisito: Docker com Compose.

```bash
cp .env.example .env        # LLM desligado por padrão (sem custo)
make up                     # postgres + localstack + api + worker
make demo                   # envia as duas notas de exemplo
curl -s localhost:8088/v1/notas | jq
ID=$(curl -s localhost:8088/v1/notas | jq -r '.dados[0].id')
curl -s localhost:8088/v1/notas/$ID | jq    # traz os achados
make down                   # derruba e apaga os dados
```

Sem `make`: `docker compose up --build -d` e os `curl` do `Makefile`.

Portas no host (definidas no `.env`): API em `8088` (`API_HOST_PORT`) e
Postgres em `5435` (`POSTGRES_HOST_PORT`). Sem `.env`, valem 8080 e 5432.

Para ligar o LLM: no `.env`, `LLM_HABILITADO=true` e `ANTHROPIC_API_KEY=...`.

### Testes

```bash
make test        # Go + Python
# ou separadamente:
cd api && go test ./...
cd worker && pip install -r requirements-dev.txt && python -m pytest -q
```

## Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `POST` | `/v1/notas` | Envia XML (multipart, campo `arquivo`). Retorna **202** |
| `GET` | `/v1/notas` | Lista notas (`?limite=20&deslocamento=0`) |
| `GET` | `/v1/notas/{id}` | Nota com status e achados |
| `GET` | `/health` | Processo vivo (liveness) |
| `GET` | `/ready` | Banco acessível (readiness) |

Contrato completo com exemplos: [docs/03-api.md](docs/03-api.md).

## Estrutura

```
auditor-nfe/
├── api/                      # API em Go
│   ├── cmd/api/main.go       # monta as dependências e sobe o servidor
│   ├── internal/
│   │   ├── config/           # variáveis de ambiente
│   │   ├── httpapi/          # rotas, handlers, middlewares (camada HTTP)
│   │   ├── audit/            # regra de negócio (sem HTTP, sem AWS)
│   │   ├── storage/          # S3 (atrás de uma interface)
│   │   ├── queue/            # SQS (atrás de uma interface)
│   │   └── db/               # código GERADO pelo sqlc (não editar)
│   ├── db/migrations/        # esquema do banco
│   ├── db/queries/           # SQL que o sqlc transforma em Go
│   └── Dockerfile
├── worker/                   # worker em Python
│   ├── auditor/
│   │   ├── main.py           # loop da fila SQS
│   │   ├── grafo.py          # fluxo LangGraph
│   │   ├── nfe.py            # leitura segura do XML
│   │   ├── regras.py         # regras fiscais R001–R006
│   │   ├── llm.py            # análise opcional por LLM
│   │   └── repositorio.py    # gravação no PostgreSQL
│   └── tests/
├── infra/localstack/         # cria bucket e filas no ambiente local
├── samples/                  # NF-e fictícias para teste
└── docs/                     # documentação (comece pelo 01)
```

## Documentação

1. [Visão geral](docs/01-visao-geral.md): problema, objetivo e escopo
2. [Arquitetura](docs/02-arquitetura.md): componentes e fluxo ponta a ponta
3. [API](docs/03-api.md): contrato HTTP
4. [Banco de dados](docs/04-banco-de-dados.md): tabelas, status, sqlc
5. [Worker e regras](docs/05-worker-e-regras.md): LangGraph, regras e LLM
6. [Decisões técnicas](docs/06-decisoes-tecnicas.md): o porquê de cada escolha
7. [Guia de estudo](docs/07-guia-de-estudo.md): como ler o código e os conceitos de Go usados
8. [Guia para entrevista](docs/08-guia-entrevista.md): como apresentar e perguntas prováveis
9. [Roadmap](docs/09-roadmap.md): próximos passos (Terraform, CI/CD, RAG)
