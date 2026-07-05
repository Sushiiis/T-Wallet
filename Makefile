# Makefile
ENV_FILE := .env
COMPOSE  := docker compose --env-file $(ENV_FILE) -f deployments/docker-compose.yml

include $(ENV_FILE)
export

MIGRATE_DSN := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=$(POSTGRES_SSLMODE)

.PHONY: up down run migrate-up migrate-down migrate-create tidy proto test test-integration

test-integration:
	go test -tags=integration ./... -v

proto:
	protoc \
	  --proto_path=api/proto --proto_path=third_party --proto_path=/usr/local/include \
	  --go_out=. --go_opt=module=github.com/Sushiiis/T-Wallet \
	  --go-grpc_out=. --go-grpc_opt=module=github.com/Sushiiis/T-Wallet \
	  --grpc-gateway_out=api/proto --grpc-gateway_opt=paths=source_relative \
	  --openapiv2_out=api/proto \
	  api/proto/wallet/v1/wallet.proto
	  
test:
	go test ./...

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

run:
	go run ./cmd/wallet

migrate-up:
	migrate -path migrations -database "$(MIGRATE_DSN)" up

migrate-down:
	migrate -path migrations -database "$(MIGRATE_DSN)" down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

tidy:
	go mod tidy