# 9. Roadmap

Ordem sugerida: cada etapa gera algo que dá para mostrar.

## Fase 1: deixar redondo
- [ ] Repositório no GitHub com este README
- [ ] GitHub Actions: `go test`, `go vet`, `pytest`, `ruff` a cada push
- [ ] Ferramenta de migração (`goose` ou `golang-migrate`) em vez do
      `initdb.d`
- [ ] Regra R007 (UF da chave x UF do emitente) e filtro `?status=` na
      listagem (exercícios do guia de estudo)

## Fase 2: AWS de verdade (Terraform)
- [ ] `infra/terraform/`: S3 (com bloqueio de acesso público e
      criptografia), SQS + DLQ, RDS PostgreSQL, ECR, ECS Fargate (api e
      worker), ALB, IAM Roles com permissão mínima, Secrets Manager
- [ ] Autoscaling do worker pela profundidade da fila
- [ ] Alarme no CloudWatch quando chegar mensagem na DLQ
- [ ] Deploy pelo GitHub Actions com OIDC (sem chave AWS guardada no GitHub)
- [ ] **Antes de tudo: alerta de orçamento (AWS Budgets)** e
      `terraform destroy` quando não estiver usando

## Fase 3: robustez
- [ ] Transactional Outbox (ver ADR-07)
- [ ] `POST /v1/notas/{id}/reprocessar`
- [ ] Autenticação (JWT ou API key por cliente)
- [ ] Upload em lote (ZIP) com URL pré-assinada do S3: o cliente envia direto
      ao S3, sem passar o arquivo pela API
- [ ] Tracing distribuído (OpenTelemetry) da API até o worker

## Fase 4: IA
- [ ] Nó de RAG no grafo: legislação (RICMS-SC, tabela NCM, tabela CFOP)
      indexada em pgvector; o LLM recebe os trechos e **cita a fonte**
- [ ] Conjunto de avaliação: notas com problemas conhecidos e medição de
      acerto do LLM (vale um número no currículo)
- [ ] `interrupt` do LangGraph para revisão humana quando a confiança for baixa
- [ ] Opção de usar Amazon Bedrock como provedor do modelo

## Fase 5: produto
- [ ] Front em Next.js: upload com arrastar e soltar, lista de notas,
      achados destacados
- [ ] Atualização em tempo real (SSE) no lugar de polling
- [ ] Versão no Google Cloud: Cloud Run + Cloud SQL + Pub/Sub + Cloud
      Storage. As interfaces `Storage` e `Publisher` já preparam isso
