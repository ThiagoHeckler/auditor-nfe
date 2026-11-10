# 8. Guia para entrevista

## Pitch de 30 segundos

> "Construí um auditor de NF-e. Uma API em Go recebe o XML, registra no
> PostgreSQL, guarda no S3 e coloca um pedido numa fila SQS. Um worker em
> Python consome a fila e roda um fluxo em LangGraph: aplica regras fiscais,
> como CFOP incompatível com o destino, NCM inválido e total que não fecha,
> e, quando há casos ambíguos, pede a um LLM uma explicação estruturada. A
> ideia veio da minha experiência com rotina fiscal: é o tipo de conferência
> que se faz à mão e que dá para automatizar."

## Versão de 2 minutos (o roteiro)

1. **Problema** (15 s): conferência manual de NF-e é lenta e falha.
2. **Arquitetura** (40 s): API Go → S3 + SQS → worker Python → Postgres.
   Por que assíncrono: o LLM é lento e falha às vezes.
3. **Uma decisão interessante** (30 s): escolha uma, por exemplo a
   idempotência pela chave de acesso com `UNIQUE` + erro 23505, ou "o LLM
   nunca gera ERRO, só as regras".
4. **Uma limitação que você conhece** (20 s): o problema de consistência
   banco/S3/fila e o Transactional Outbox como solução.
5. **Próximo passo** (15 s): Terraform na AWS e RAG com a legislação.

Falar de uma limitação **por conta própria** passa mais confiança do que
fingir que o projeto é perfeito.

## Perguntas prováveis e como responder

**"Por que Go e Python juntos? Não complica?"**
Cada um no que faz melhor: Go para o serviço de I/O (binário leve, rápido,
concorrência), Python porque o ecossistema de IA é Python-first. O custo é
manter o contrato da mensagem sincronizado; o contrato é pequeno (2 campos)
e está documentado nos dois lados.

**"Por que 202 e não 201?"**
A auditoria ainda não aconteceu. 202 significa "aceito para processamento".
O cabeçalho `Location` diz onde consultar.

**"O que acontece se o worker cair no meio de uma nota?"**
A mensagem não foi apagada da fila. Depois do visibility timeout (120 s) ela
reaparece e outro worker pega. O UPDATE condicional de status e a gravação
dos achados em transação (apaga os antigos e insere os novos) tornam o
reprocessamento seguro.

**"E se a mesma mensagem chegar duas vezes?"**
O SQS padrão é *at-least-once*. O worker só processa se a nota estiver
`RECEBIDA` ou `PROCESSANDO`; se já estiver `CONCLUIDA`, ignora.

**"E se uma nota sempre der erro?"**
Depois de 3 tentativas, o SQS move a mensagem para a DLQ. Ela não trava a
fila e fica disponível para análise. Erros permanentes (XML inválido) nem
chegam a ser tentados de novo: são marcados como `ERRO` na primeira vez.

**"Por que não usou um ORM?"**
sqlc: escrevo SQL e o código Go tipado é gerado. Erro de coluna aparece na
geração, não em produção, e não há consulta escondida. Trade-off: consultas
dinâmicas ficam mais trabalhosas.

**"Como você testa sem AWS?"**
Duas camadas. Testes unitários com fakes, porque o serviço depende de
interfaces e não do S3/SQS concretos. E o ambiente local com LocalStack, que
emula S3 e SQS.

**"Como garante que o LLM não alucina um erro fiscal?"**
Ele não tem esse poder: só gera ALERTA ou INFO, com `origem = LLM`.
Decisão objetiva é código determinístico e testado. Além disso, a saída é
estruturada (Pydantic), `temperature=0`, e ele só recebe os itens já
marcados como suspeitos.

**"Como escalaria para 1 milhão de notas por mês?"**
São ~23 notas por minuto, pouco para essa arquitetura. A API é *stateless*
(escala horizontal). O worker escala por profundidade da fila (autoscaling no
ECS pela métrica `ApproximateNumberOfMessagesVisible`). Gargalos prováveis:
custo e limite de taxa do LLM (por isso ele só roda em ALERTA) e conexões
do banco (pool, ou RDS Proxy).

**"E segurança?"**
Limite de tamanho no upload (`MaxBytesReader`), `defusedxml` contra XXE,
UUID na URL em vez de id sequencial, container distroless rodando como
não-root, credenciais por IAM Role. Falta autenticação, que é o primeiro item
do roadmap.

**"Qual foi a parte mais difícil?"**
Responda com a sua experiência real. Bons candidatos: entender o cálculo do
dígito verificador, a consistência entre banco/S3/fila ou o comportamento do
SQS (visibility timeout e DLQ).

## Cuidados

- **Não decore, entenda.** Abra cada arquivo citado acima e confira que
  consegue explicar linha a linha. O entrevistador pode pedir para
  compartilhar a tela.
- **Só cite o que existe.** Se o Terraform ainda não estiver pronto, diga
  "é o próximo passo", não "está na AWS".
- **Seja claro sobre as simplificações fiscais** (ver
  [01-visao-geral.md](01-visao-geral.md)).

## Exemplo de linha para o currículo

> **Auditor NF-e**: API em Go e worker em Python (LangGraph) que auditam XMLs
> de NF-e de forma assíncrona com S3 e SQS (DLQ, retentativa, idempotência),
> PostgreSQL com sqlc e análise opcional por LLM com saída estruturada.
> Testes automatizados e ambiente local com Docker e LocalStack.
