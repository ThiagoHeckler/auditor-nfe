# 1. Visão geral

## O problema

Escritórios de contabilidade recebem centenas de XMLs de NF-e por mês. Erros
como CFOP de operação interestadual numa venda interna, NCM genérico ou total
que não fecha com a soma dos itens passam despercebidos e só aparecem depois,
na apuração ou numa fiscalização. Conferir nota por nota à mão é lento e
cansativo, e por isso propenso a erro.

## O objetivo

Um serviço que:

1. recebe XMLs de NF-e por uma API;
2. aplica regras fiscais objetivas automaticamente;
3. para casos ambíguos, pede a um LLM uma explicação do que o contador deve
   conferir;
4. guarda o resultado da auditoria para consulta.

## Escopo desta versão

**Está dentro:**
- Upload individual de XML (layout NF-e 4.00, modelo 55)
- 6 regras determinísticas (ver [05-worker-e-regras.md](05-worker-e-regras.md))
- Análise opcional por LLM só nos itens com ALERTA
- Processamento assíncrono com fila, novas tentativas e DLQ
- Ambiente local completo com Docker

**Está fora (de propósito, ver [09-roadmap.md](09-roadmap.md)):**
- Autenticação de usuários
- Upload em lote (ZIP)
- Consulta de situação da nota na SEFAZ
- Regras de cálculo de impostos (ICMS-ST, DIFAL etc.)
- Interface web
- Deploy na AWS (Terraform é o próximo passo)

## Simplificações conscientes

As regras são **simplificadas** para fins de portfólio. Exemplo: a R002
compara o primeiro dígito do CFOP com o `idDest`, mas a legislação tem
exceções. O objetivo do projeto é demonstrar arquitetura e engenharia, não
substituir um validador fiscal homologado. Saber dizer isso em entrevista
mostra maturidade.

## Por que este projeto

Ele junta experiência real com rotina fiscal (CFOP, NCM, NF-e) com engenharia
de software: API, fila, banco, nuvem e IA. Um problema real com regras reais
diz mais num portfólio do que um CRUD genérico.
