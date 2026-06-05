.PHONY: up down build logs dev infra proto test-go test-java new-java-module new-go-module db-seed db-reset db-psql

# Desenvolvimento local 
# Sobe infra via Docker e todos os serviços Go/Java localmente (com saída colorida)

dev:
	@bash scripts/dev.sh

# Cria um novo módulo Java e atualiza pom.xml + docker-compose automaticamente
# Uso: make new-java-module NAME=spell-worker
#      make new-java-module NAME=notification-service PORT=8082
new-java-module:
	@bash scripts/new-java-module.sh $(NAME) $(PORT)

# Cria um novo módulo Go e atualiza go.work + docker-compose automaticamente
# Uso: make new-go-module NAME=metrics-worker
#      make new-go-module NAME=analytics-service PORT=8082
new-go-module:
	@bash scripts/new-go-module.sh $(NAME) $(PORT)

# Sobe apenas a infra (postgres, redis, kafka) sem os serviços da aplicação
infra:
	docker compose -f infra/docker-compose.yml up -d postgres redis zookeeper kafka kafka-init


up:
	docker compose -f infra/docker-compose.yml up -d

down:
	docker compose -f infra/docker-compose.yml down

# Banco de dados 
# Aplica apenas o seed (idempotente — pode rodar mais de uma vez)
db-seed:
	docker compose -f infra/docker-compose.yml exec -T postgres \
	  psql -U collabdocs -d collabdocs < infra/postgres/seed.sql

# Derruba o volume do postgres e sobe tudo do zero (schema + seed frescos)
# ATENÇÃO: apaga todos os dados
db-reset:
	docker compose -f infra/docker-compose.yml down -v
	docker compose -f infra/docker-compose.yml up -d

# Abre o psql interativo no container
db-psql:
	docker compose -f infra/docker-compose.yml exec postgres psql -U collabdocs -d collabdocs

build:
	docker compose -f infra/docker-compose.yml build

logs:
	docker compose -f infra/docker-compose.yml logs -f

# Proto 
# Instalar: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#           go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

proto:
	protoc -I proto \
	  --go_out=go/gateway/gen     --go_opt=paths=source_relative \
	  --go-grpc_out=go/gateway/gen --go-grpc_opt=paths=source_relative \
	  proto/doc.proto proto/collab.proto
	protoc -I proto \
	  --java_out=java/user-service/src/main/java \
	  proto/doc.proto proto/collab.proto


test-go:
	go test ./go/gateway/tests/... -v

test-java:
	mvn -f java/pom.xml test -pl user-service
