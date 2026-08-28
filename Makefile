.PHONY: run test lint up down build tidy seed docs

run:
	go run ./cmd/api

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
