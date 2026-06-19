.PHONY: up down build logs go-dev java-dev fe-dev proto-clean

up:
	docker compose -f infra/docker-compose.yml up --build -d

down:
	docker compose -f infra/docker-compose.yml down -v

build:
	docker compose -f infra/docker-compose.yml build

logs:
	docker compose -f infra/docker-compose.yml logs -f

go-dev:
	cd go/collab-service && go run ./cmd

java-dev:
	cd java/backend && ./mvnw spring-boot:run

fe-dev:
	cd frontend && npm run dev

go-tidy:
	cd go/collab-service && go mod tidy

db-reset:
	docker compose -f infra/docker-compose.yml down -v postgres
	docker compose -f infra/docker-compose.yml up -d postgres
