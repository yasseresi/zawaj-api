.PHONY: run dev db-setup db-reset test lint up down build tidy seed docs

run:
	go run ./cmd/api

# One-shot local start: bootstrap the Postgres role/db, then run the API
# (migrations apply automatically on boot). Fixes the classic
# 'role "zawaj" does not exist' cold-start error.
dev: db-setup run

# Create the local role + database the API expects (idempotent).
db-setup:
	./scripts/setup-db.sh

# Drop + recreate the local database, then re-run migrations from scratch.
# Use when migrations fail with "already exists" after a partial init.
db-reset:
	./scripts/setup-db.sh --reset

build:
	go build -o bin/api ./cmd/api

test:
	go test ./... -race -cover

lint:
	golangci-lint run

tidy:
	go mod tidy

up:
	docker compose up -d --build

down:
	docker compose down

seed:
	go run ./cmd/seed

# Regenerate the OpenAPI spec + Swagger UI assets from handler annotations.
# Install once: go install github.com/swaggo/swag/cmd/swag@v1.16.4
docs:
	swag init -g cmd/api/main.go --parseInternal --parseDependency -o docs/swagger
