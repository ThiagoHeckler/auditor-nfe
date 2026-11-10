# Atalhos do dia a dia. Rode `make help` para ver a lista.
# Lê as portas do .env (se existir).
-include .env
API_HOST_PORT ?= 8080

.PHONY: help up down logs test test-api test-worker sqlc lint demo

help: ## Mostra os comandos
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

up: ## Sobe tudo (postgres, localstack, api, worker)
	docker compose up --build -d

down: ## Derruba e apaga os dados locais
	docker compose down -v

logs: ## Acompanha os logs da api e do worker
	docker compose logs -f api worker

test: test-api test-worker ## Roda todos os testes

test-api: ## Testes da API Go
	cd api && go test ./...

test-worker: ## Testes do worker Python
	cd worker && python -m pytest -q

sqlc: ## Regenera o código Go das queries (após mudar .sql)
	cd api && sqlc generate

lint: ## Formatação e lint
	cd api && gofmt -l . && go vet ./...
	cd worker && ruff check .

demo: ## Envia as duas notas de exemplo para a API
	@echo "--- nota válida ---"
	curl -s -F arquivo=@samples/nfe-valida.xml localhost:$(API_HOST_PORT)/v1/notas; echo
	@echo "--- nota com problemas ---"
	curl -s -F arquivo=@samples/nfe-com-problemas.xml localhost:$(API_HOST_PORT)/v1/notas; echo
	@echo "Aguarde alguns segundos e consulte: curl -s localhost:$(API_HOST_PORT)/v1/notas"
