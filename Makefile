.PHONY: run test lint up down build tidy seed

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
