# PipelineSherlock developer tasks.
# `make help` lists every target. The one-command local path is `make dev`.

# Local development database/redis URLs (match docker-compose.yml).
DATABASE_URL ?= postgres://ps:ps@localhost:5432/pipeline_sherlock?sslmode=disable
REDIS_URL    ?= redis://localhost:6379/0
export DATABASE_URL
export REDIS_URL

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: dev
dev: deps-up migrate run ## Bring up deps, migrate, then run the API (one command)

.PHONY: deps-up
deps-up: ## Start Postgres and Redis via docker compose
	docker compose up -d db redis

.PHONY: deps-down
deps-down: ## Stop and remove the docker compose stack
	docker compose down

.PHONY: run
run: ## Run the API server (requires DATABASE_URL)
	go run ./cmd/server serve

.PHONY: migrate
migrate: ## Apply pending database migrations
	go run ./cmd/server migrate

.PHONY: migrate-down
migrate-down: ## Revert the most recent migration
	go run ./cmd/server rollback

.PHONY: seed
seed: ## Load demo data (requires a migrated database)
	psql "$(DATABASE_URL)" -f tests/fixtures/seed.sql

.PHONY: build
build: ## Compile the server binary into ./bin
	go build -o bin/server ./cmd/server

.PHONY: test
test: ## Run unit tests
	go test ./...

.PHONY: test-race
test-race: ## Run unit tests with the race detector (needs 64-bit cgo; used in CI)
	go test -race ./...

.PHONY: cover
cover: ## Run tests with a coverage summary
	go test -cover ./...

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod/go.sum
	go mod tidy
	go mod verify
