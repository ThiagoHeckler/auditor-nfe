# 6. Decisões técnicas

Formato curto de ADR (*Architecture Decision Record*): contexto, decisão,
consequência. Em entrevista, "por que você fez X?" é a pergunta mais comum.
Estas são as respostas.

---

### ADR-01: Go para a API, Python para o worker

**Contexto:** a API faz I/O (HTTP, banco, S3, fila). O worker faz IA.
**Decisão:** Go na API; Python no worker.
**Por quê:** Go gera um binário estático pequeno, sobe em milissegundos
(ótimo para container e Lambda) e tem concorrência nativa. O ecossistema de
IA (LangChain, LangGraph, CrewAI) é Python-first; as versões em outras
linguagens ficam atrás.
**Consequência:** duas linguagens no repositório e um contrato de mensagem
que precisa ficar sincronizado (`queue/sqs.go` ↔ `mensagem.py`). Por isso
os dois arquivos se referenciam nos comentários.

### ADR-02: `net/http` da biblioteca padrão, sem framework

**Decisão:** sem Gin, Echo ou Fiber.
**Por quê:** desde o Go 1.22 o `ServeMux` aceita método e parâmetros
(`"GET /v1/notas/{id}"`), que era o principal motivo para usar framework. Menos
dependências = menos atualização de segurança, e o código mostra que você
entende HTTP, não só um framework.
**Consequência:** middlewares escritos à mão (são ~80 linhas em
`middleware.go`). Se o projeto crescer, `chi` é compatível com `net/http` e
entra sem reescrever handlers.

### ADR-03: sqlc + pgx em vez de ORM (GORM)

**Decisão:** SQL escrito à mão, código Go gerado pelo sqlc.
**Por quê:** erros de SQL aparecem na geração do código; o SQL executado é
explícito (sem N+1 escondido); performance de driver puro.
**Consequência:** consultas dinâmicas (filtros opcionais combinados) são
mais trabalhosas. Para este domínio, as consultas são fixas.

### ADR-04: processamento assíncrono com fila

**Decisão:** API devolve 202 e enfileira; o worker processa.
**Por quê:** a chamada ao LLM é lenta e falha às vezes. Desacoplar dá
resposta rápida, escala independente e retentativa automática.
**Consequência:** o cliente precisa consultar o status (polling). Há mais
peças para operar (fila, DLQ).

### ADR-05: interfaces na camada de negócio

**Decisão:** `audit.Service` depende de `Repository`, `storage.Storage` e
`queue.Publisher` (interfaces), não de tipos concretos.
**Por quê:** testes sem infraestrutura (veja `service_test.go`, com fakes em
memória), e trocar S3 por Google Cloud Storage é escrever uma struct nova,
sem tocar no serviço.
**Consequência:** um pouco mais de código de "ligação" no `main.go`
(injeção de dependência manual, sem framework de DI).

**Detalhe de Go:** as interfaces ficam **onde são usadas** (no pacote
`audit`), não onde são implementadas. É a convenção idiomática: "aceite
interfaces, devolva structs".

### ADR-06: idempotência pela chave de acesso

**Decisão:** `UNIQUE (chave_acesso)` + tratamento do erro `23505`.
**Por quê:** a chave identifica a NF-e no país inteiro. Reenvio (usuário
clicou duas vezes, cliente refez por timeout) não pode gerar duas
auditorias. A checagem é no banco, não só no código, então funciona mesmo com
duas requisições simultâneas (*race condition*).
**Consequência:** para reauditar uma nota será preciso uma rota específica
(roadmap).

### ADR-07: consistência entre banco, S3 e fila (limitação conhecida)

**Contexto:** a API faz 3 escritas em sistemas diferentes (banco → S3 →
fila). Não existe transação que abranja os três.
**Decisão atual:** escrever na ordem banco → S3 → fila e, se S3 ou fila
falharem, marcar a nota como `ERRO`.
**Limitação:** se o processo morrer **entre** o INSERT e o SendMessage (queda
do container), a nota fica `RECEBIDA` para sempre.
**Solução robusta (roadmap): Transactional Outbox.** No mesmo INSERT da
nota, grava-se uma linha numa tabela `outbox` (mesma transação). Um processo
separado lê a `outbox` e publica na fila. Assim o "pedido de auditoria"
nunca se perde. Saber nomear esse padrão e explicar o problema é um ótimo
sinal em entrevista.

### ADR-08: Decimal para dinheiro

**Decisão:** `Decimal` no Python. A API não faz conta com valores.
**Por quê:** `float` não representa 0,10 exatamente. Em auditoria fiscal,
diferença de centavo é achado falso.

### ADR-09: defusedxml no worker

**Decisão:** nunca usar `xml.etree` puro para XML externo.
**Por quê:** proteção contra XXE e *billion laughs*. A API em Go usa
`encoding/xml`, que não resolve entidades externas por padrão.

### ADR-10: LLM opcional, só para ALERTA, nunca gera ERRO

**Por quê:** custo (só chama quando precisa), previsibilidade (decisão
objetiva é código testável) e explicabilidade (o contador sabe qual regra
disparou). O LLM complementa o código, não substitui.

### ADR-11: LocalStack fixado na versão 4.14

**Contexto:** em março de 2026 o LocalStack unificou as imagens e passou a
exigir conta e `LOCALSTACK_AUTH_TOKEN` a partir da versão 2026.03.
**Decisão:** fixar `localstack/localstack:4.14`, a última versão que roda sem
conta. Para S3 e SQS básicos ela é suficiente.
**Alternativas, se a imagem deixar de funcionar:** criar a conta gratuita do
LocalStack e usar a imagem atual com o token, ou trocar por um emulador
compatível (ex.: Floci), que usa a mesma porta 4566. Nenhuma das duas
exige mudança de código, só no `docker-compose.yml`.

### ADR-12: configuração por variáveis de ambiente (12-factor)

**Por quê:** o mesmo binário/imagem roda local, em homologação e em
produção. Segredos nunca ficam no código nem na imagem. Na AWS, as
credenciais vêm da IAM Role; a chave do LLM, do Secrets Manager.

### ADR-13: logs estruturados em JSON

**Decisão:** `log/slog` com `JSONHandler` no Go; formato JSON no Python.
**Por quê:** CloudWatch e Cloud Logging indexam campos JSON. Dá para filtrar
`status = 500` ou buscar por `request_id` em vez de usar grep em texto.
