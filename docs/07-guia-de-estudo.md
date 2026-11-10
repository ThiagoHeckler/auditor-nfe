# 7. Guia de estudo: como ler este código

## Ordem de leitura recomendada

Siga o caminho de uma requisição, do começo ao fim:

1. `api/cmd/api/main.go`: como tudo é montado
2. `api/internal/httpapi/handlers.go`: rotas e o `enviarNota`
3. `api/internal/audit/service.go`: o método `Enviar` (coração da API)
4. `api/internal/audit/xml.go`: extração da chave
5. `api/internal/storage/s3.go` e `queue/sqs.go`: saída para a AWS
6. `worker/auditor/main.py`: o loop da fila
7. `worker/auditor/grafo.py`: o fluxo LangGraph
8. `worker/auditor/regras.py`: as regras
9. Os testes: `service_test.go`, `handlers_test.go`, `test_regras.py`

Depois, rode o projeto (`make up` + `make demo`) e acompanhe os logs com
`make logs` enquanto envia notas.

## Conceitos de Go usados (e onde)

Se você vem de PHP/Java, estes são os pontos que mais mudam.

### Pacotes e `internal/`
Tudo em `internal/` só pode ser importado por código deste módulo. É o jeito
de Go dizer "isso não é API pública".

### Structs no lugar de classes; métodos com receptor
```go
type Service struct { repo Repository; ... }
func (s *Service) Enviar(ctx context.Context, ...) (Nota, error)
```
`(s *Service)` é o "this". Ponteiro (`*`) porque o método não deve copiar a
struct.

### Interfaces implícitas
Não existe `implements`. Se um tipo tem os métodos, ele satisfaz a interface.
`*db.Queries` (gerado pelo sqlc) satisfaz `audit.Repository` sem saber que
ela existe. Em Java você precisaria declarar isso.

### Erros são valores
Não há exceções. Funções devolvem `(resultado, error)`:
```go
chave, err := ExtrairChaveAcesso(conteudo)
if err != nil {
    return Nota{}, err
}
```
- **Embrulhar** erro com contexto: `fmt.Errorf("inserir nota: %w", err)`
- **Erro sentinela**: `var ErrNotaDuplicada = errors.New(...)`
- **Comparar** atravessando embrulhos: `errors.Is(err, audit.ErrXMLInvalido)`
- **Extrair tipo**: `errors.As(err, &pgErr)` para ler o código `23505`

### `context.Context`
Primeiro parâmetro de toda função que faz I/O. Carrega **cancelamento** e
**prazo**: se o cliente fecha a conexão ou o servidor está desligando, as
consultas ao banco e as chamadas à AWS são canceladas juntas. No `main.go`,
`signal.NotifyContext` cancela o contexto ao receber SIGTERM.

### Goroutines e channels (no `main.go`)
```go
go func() { erros <- srv.ListenAndServe() }()
select {
case err := <-erros:  // servidor falhou
case <-ctx.Done():    // recebeu sinal para parar
}
```
O servidor roda numa goroutine (thread leve); o `select` espera o que vier
primeiro. É o padrão de **graceful shutdown**.

### `defer`
`defer pool.Close()` executa ao sair da função, como um `finally`, só que
declarado ao lado de quem abriu o recurso.

### Middlewares = funções que embrulham handlers
```go
requestID(logRequests(log, recoverPanic(log, mux)))
```
Cada um recebe um `http.Handler` e devolve outro. A requisição atravessa de
fora para dentro.

### Testes de tabela
Padrão idiomático em Go (`TestExtrairChaveAcesso`, `TestEnviarNota_StatusHTTP`):
uma lista de casos e um laço com `t.Run`. Para adicionar um caso, basta uma
linha.

### `httptest`
Testa handlers sem subir servidor: `httptest.NewRecorder()` captura a resposta.

## Conceitos de Python usados

- `dataclass(frozen=True)`: objetos de valor imutáveis (`Achado`)
- `TypedDict`: tipagem do estado do LangGraph
- `Literal["INFO", "ALERTA", "ERRO"]`: tipos com valores fixos
- `pydantic.BaseModel`: validação da saída do LLM
- `pytest.mark.parametrize` e `monkeypatch`: testes de tabela e substituição
  de funções

## Exercícios para fixar (e para ter o que contar)

1. Crie a regra **R007**: `cUF` da chave (2 primeiros dígitos) deve bater com
   a UF do emitente (42 = SC).
2. Crie a rota `POST /v1/notas/{id}/reprocessar`, que só aceita notas em `ERRO`.
3. Adicione `?status=ERRO` na listagem (nova query no sqlc).
4. Escreva um teste para `config.Load` com variável faltando.
5. Implemente o Transactional Outbox (ADR-07).

Cada exercício feito vira um commit e uma história real de "eu evoluí o projeto".
