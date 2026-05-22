.PHONY: all build run dev test lint migrate-up migrate-down migrate-create docker-up docker-down docker-logs clean

BINARY_NAME=server
MAIN_PATH=./cmd/server
MIGRATIONS_PATH=./migrations
DB_URL?=postgres://dp_user:dp_secret@localhost:5432/digital_personality?sslmode=disable

all: build

build:
	go build -ldflags="-s -w" -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: build
	./bin/$(BINARY_NAME)

dev:
	go run $(MAIN_PATH)

test:
	go test -race -count=1 ./...

test-integration:
	go test -race -count=1 -tags integration ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
	goimports -w .

vet:
	go vet ./...

# Migrations (requires golang-migrate CLI)
migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" down 1

migrate-down-all:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" down

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $$name

migrate-status:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" version

# Docker
docker-up:
	docker compose up -d

docker-up-build:
	docker compose up -d --build

docker-down:
	docker compose down

docker-down-volumes:
	docker compose down -v

docker-logs:
	docker compose logs -f

docker-ps:
	docker compose ps

# Database helpers
db-shell:
	docker compose exec postgres psql -U dp_user -d digital_personality

# Cleanup
clean:
	rm -rf bin/
	go clean -cache

deps:
	go mod tidy
	go mod download

.env:
	cp .env.example .env
	@echo "Created .env from .env.example — fill in your secrets"
