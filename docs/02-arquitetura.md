# 2. Arquitetura

## Componentes

```mermaid
flowchart LR
    C[Cliente / curl] -->|POST /v1/notas| A[API Go]
    A -->|INSERT nota RECEBIDA| P[(PostgreSQL)]
    A -->|PutObject XML| S[(S3)]
    A -->|SendMessage| Q[[SQS: auditorias]]
    Q -->|ReceiveMessage| W[Worker Python]
    W -->|GetObject XML| S
    W -->|LangGraph: regras + LLM| W
    W -->|INSERT achados, status CONCLUIDA| P
    Q -.->|3 falhas| D[[SQS: DLQ]]
    C -->|GET /v1/notas/id| A
    A -->|SELECT| P
```

| Componente | Responsabilidade | Não faz |
|---|---|---|
| **API Go** | validar o upload, registrar a nota, salvar no S3, enfileirar, servir consultas | regra fiscal |
| **S3** | guardar o XML original (fonte da verdade do arquivo) | — |
| **SQS** | entregar "pedidos de auditoria" ao worker, com retentativa | — |
| **DLQ** | guardar mensagens que falharam 3 vezes para análise | — |
| **Worker Python** | ler o XML, aplicar regras, chamar o LLM, gravar achados | HTTP |
| **PostgreSQL** | estado das notas e achados | guardar o XML |

## Fluxo de uma nota, passo a passo

```mermaid
sequenceDiagram
    participant C as Cliente
    participant A as API Go
    participant DB as PostgreSQL
    participant S3
    participant Q as SQS
    participant W as Worker

    C->>A: POST /v1/notas (arquivo.xml)
    A->>A: extrai chave de acesso do XML
    A->>DB: INSERT nota (RECEBIDA)
    Note over A,DB: chave UNIQUE: duplicada vira 409
    A->>S3: PutObject notas/{chave}.xml
    A->>Q: SendMessage {nota_id, s3_key}
    A-->>C: 202 Accepted + id

    W->>Q: ReceiveMessage (long polling 20s)
    W->>DB: UPDATE status = PROCESSANDO
    W->>S3: GetObject
    W->>W: grafo: ler_xml → regras → (LLM?)
    W->>DB: INSERT achados + status CONCLUIDA (1 transação)
    W->>Q: DeleteMessage

    C->>A: GET /v1/notas/{id}
    A->>DB: SELECT nota + achados
    A-->>C: 200 + achados
```

## Estados de uma nota

```mermaid
stateDiagram-v2
    [*] --> RECEBIDA: API registrou
    RECEBIDA --> PROCESSANDO: worker pegou a mensagem
    RECEBIDA --> ERRO: falha no S3 ou na fila
    PROCESSANDO --> CONCLUIDA: achados gravados
    PROCESSANDO --> ERRO: XML inválido (erro permanente)
    PROCESSANDO --> PROCESSANDO: falha temporária, nova tentativa
```

## Por que assíncrono?

Se a API auditasse durante o request:
- uma chamada ao LLM pode levar de 5 a 20 s, e o cliente ficaria esperando;
- um pico de uploads derrubaria a API, porque todo o trabalho pesado
  aconteceria nela;
- uma falha do LLM viraria erro 500 para o cliente.

Com fila:
- a API responde em milissegundos (**202 Accepted**);
- o worker escala separadamente (mais réplicas = mais vazão);
- falhas temporárias são tentadas de novo automaticamente;
- o que falha sempre vai para a DLQ em vez de travar a fila.

## Ambiente local x AWS real

O código é o mesmo. Só muda a configuração (variáveis de ambiente):

| Recurso | Local (docker compose) | AWS (alvo do Terraform) |
|---|---|---|
| API | container `api` | ECS Fargate ou App Runner |
| Worker | container `worker` | ECS Fargate (serviço separado) |
| S3 / SQS | LocalStack (`AWS_ENDPOINT_URL`) | S3 / SQS reais (sem endpoint) |
| Credenciais | `test`/`test` falsas | IAM Role da task (sem chave no código) |
| Banco | container `postgres` | RDS PostgreSQL |
| LLM | Anthropic API | Anthropic API ou Amazon Bedrock |
| Logs | `docker compose logs` | CloudWatch Logs (os logs já saem em JSON) |
