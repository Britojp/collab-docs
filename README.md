# CollabDocs

Sistema distribuído de edição colaborativa em tempo real.  
Go (Gateway) + Java/Spring (User Service).

## Setup

### 1. Clone o repositório

```bash
git clone https://github.com/britojp/collabdocs.git
cd collabdocs
```

### 2. Configure as variáveis de ambiente (opcional)

Por padrão os serviços usam valores seguros para desenvolvimento. Para sobrescrever, crie um arquivo `.env` na raiz:

```bash
# .env  (não commitar)
JWT_SECRET=meu-segredo-com-no-minimo-32-caracteres
```

> **Requisito:** `JWT_SECRET` precisa ter no mínimo 32 caracteres (256 bits) para o algoritmo HS256.  
> O default `collabdocs-dev-secret-key-32chars!!` já satisfaz esse requisito em desenvolvimento.

### 3. Suba a infraestrutura

```bash
make up
```

Isso sobe Postgres, Redis, Kafka, MinIO e todos os serviços via Docker Compose.  
Aguarde todos os healthchecks passarem antes de continuar.

### 4. Configure o banco de dados

O schema e o seed são aplicados **automaticamente** na primeira vez que o container do Postgres sobe.

**O que é executado automaticamente:**

| Arquivo | O que faz |
|---------|-----------|
| `infra/postgres/init.sql` | Cria todas as tabelas (`users`, `documents`, `operations`, `audit_log`, etc.) e índices |
| `infra/postgres/seed.sql` | Insere o usuário de teste inicial |

**Usuário de teste criado pelo seed:**

| Campo | Valor |
|-------|-------|
| E-mail | `admin@collabdocs.dev` |
| Senha | `admin123` |
| Nome | `Admin CollabDocs` |

**Verificar se o banco está pronto:**

```bash
make db-psql
# dentro do psql:
\dt          -- lista as tabelas
SELECT email, name FROM users;
\q
```

**Re-aplicar o seed em um banco já existente** (idempotente):

```bash
make db-seed
```

**Recriar o banco do zero** (apaga todos os dados):

```bash
make db-reset   # equivale a: docker compose down -v && docker compose up -d
```

**Testar o login com o usuário seed:**

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@collabdocs.dev","password":"admin123"}'
# → {"token":"eyJ...","userId":"...","name":"Admin CollabDocs","email":"admin@collabdocs.dev"}
```

### 6. Gere o código gRPC a partir dos `.proto`

```bash
# Instale os plugins (uma vez)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

make proto
```

### 7. Rode o Gateway (modo desenvolvimento)

```bash
go run ./go/gateway/cmd/main.go
```

### 8. Rode o User Service (modo desenvolvimento)

```bash
mvn -f java/pom.xml spring-boot:run -pl user-service
```

### 6. (Opcional) Build completo via Docker

```bash
make build   # constrói todas as imagens
make up      # sobe tudo junto
make logs    # acompanha os logs
make down    # derruba tudo
```

## Testes

### Gateway (Go)

```bash
go test ./go/gateway/tests/... -v
```

Os testes ficam em `go/gateway/tests/` e não precisam de infra rodando — usam servidores HTTP em memória (`httptest`).

### User Service (Java)

```bash
mvn -f java/pom.xml test -pl user-service

# Com output detalhado (Surefire)
mvn -f java/pom.xml test -pl user-service -Dsurefire.failIfNoSpecifiedTests=false
```

Os testes ficam em `java/user-service/src/test/java/` e usam Mockito + MockMvc — sem banco, sem Kafka, sem Docker.

### Via Makefile

```bash
make test-go    # go test ./go/gateway/tests/... -v
make test-java  # mvn -f java/pom.xml test -pl user-service
```

---

## Guia de decisão: Go ou Java? REST ou gRPC?

### Quando usar Go

Use Go para serviços que precisam de **alta concorrência ou conexões persistentes**.

| Situação                                                        | Por quê Go                                                                 |
| --------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Serviço que mantém conexões WebSocket abertas                   | Goroutines são baratas (~2 KB cada); manter milhares em paralelo é trivial |
| Serviço que roteia ou faz proxy de requisições                  | Go compila para binário único, latência de startup em milissegundos        |
| Serviço que processa operações com alta taxa (CRDT)             | `sync.RWMutex` e channels tornam paralelismo explícito e eficiente         |
| CLI ou ferramenta de linha de comando                           | Binário estático, sem JVM, sem instalação                                  |
| Novo serviço de infra/plataforma sem lógica de negócio complexa | Simplicidade e performance por padrão                                      |

**Módulos Go atuais:** Gateway

---

### Quando usar Java

Use Java para serviços com **lógica de negócio rica, integrações de ecossistema ou processamento batch**.

| Situação                                                          | Por quê Java                                          |
| ----------------------------------------------------------------- | ----------------------------------------------------- |
| CRUD com autenticação, validação e ORM                            | Spring Boot + JPA + Security eliminam boilerplate     |
| Worker que consome Kafka e processa em batch                      | Spring Batch, Spring Kafka com retry/DLQ prontos      |
| Integração com bibliotecas de terceiros                           | Ecossistema Maven maduro; essas libs existem para JVM |
| Serviço com regras de negócio complexas e muitos testes unitários | Mockito + JUnit5 + MockMvc facilitam testes isolados  |
| Envio de notificações, auditoria, relatórios                      | Spring tem abstrações prontas para cada um            |

**Módulos Java atuais:** User Service

---

### Quando usar REST

Use REST para comunicação **entre cliente externo e gateway**, ou quando o consumidor não é Go.

| Situação                                                      | Use REST                                                   |
| ------------------------------------------------------------- | ---------------------------------------------------------- |
| Cliente browser ou mobile chamando a API                      | Navegadores entendem HTTP nativo; não precisam de gRPC-web |
| Gateway → User Service                                        | User Service é Java/Spring; REST é o padrão natural        |
| Endpoints públicos que precisam ser testados com curl/Postman | REST é legível e sem ferramentas especiais                 |
| Webhooks ou callbacks de terceiros                            | Terceiros enviam HTTP, não gRPC                            |

**Exemplo no projeto:** `POST /auth/login`, `GET /users/:id` (cliente → Gateway → User Service)

---

### Quando usar gRPC

Use gRPC para comunicação **interna entre serviços Go**, especialmente quando há streaming ou contratos tipados.

| Situação                                               | Use gRPC                                                               |
| ------------------------------------------------------ | ---------------------------------------------------------------------- |
| Gateway → Doc Service (operação de edição crítica)     | Tipado via Protobuf, streaming bidirecional, baixa latência            |
| Gateway → Collab Service (presença e cursores)         | Volume alto de mensagens pequenas; gRPC é mais eficiente que HTTP/JSON |
| Qualquer chamada servidor-para-servidor em Go          | Código gerado pelo `protoc` elimina erros de contrato                  |
| Precisar de streaming (histórico de operações, replay) | `stream` no Protobuf é nativo; em REST precisaria de SSE/WS separado   |

**Não use gRPC quando:** o chamador é um browser diretamente (use WebSocket ou REST), ou quando a simplicidade importa mais que performance.

**Exemplo no projeto:** `ApplyOperation` (Gateway → Doc Service), `UpdatePresence` (Gateway → Collab Service)

---

### Resumo visual (estado atual)

```
Cliente externo (browser, curl, Postman)
    └── REST ──► Gateway [Go :8080]
                    └── REST ──► User Service [Java :8081]  ← auth, CRUD
```

À medida que os demais serviços forem implementados, o diagrama será expandido.

---

## Adicionando um novo módulo Go

Use o script `new-go-module` sempre que precisar criar um novo serviço ou worker Go. Ele gera toda a estrutura do módulo e atualiza automaticamente o `go.work` e o `docker-compose.yml`.

### Quando usar

- Ao implementar um worker Go que ainda não existe
- Ao criar um novo microsserviço Go com porta HTTP ou gRPC própria

### Como usar

```bash
# Worker — processo de background, sem porta exposta
make new-go-module NAME=metrics-worker

# Serviço HTTP — expõe porta e gera main.go com servidor HTTP
make new-go-module NAME=analytics-service PORT=8082
```

`go.work` e `docker-compose.yml` são atualizados automaticamente.

### Após criar o módulo

1. Implemente a lógica em `go/<módulo>/cmd/main.go` e `internal/`
2. Adicione dependências: `cd go/<módulo> && go get <pacote>`
3. Suba só o novo módulo: `docker compose -f infra/docker-compose.yml up -d <módulo>`

---

## Adicionando um novo módulo Java

Use o script `new-java-module` sempre que precisar criar um novo serviço ou worker Java. Ele gera toda a estrutura do módulo e atualiza automaticamente o `pom.xml` e o `docker-compose.yml`.

### Quando usar

- Ao implementar um worker que ainda não existe (`spell-worker`, `notif-worker`, `audit-worker`)
- Ao criar um novo microsserviço Java com porta HTTP própria

### Como usar

```bash
# Worker — consome Kafka, sem porta HTTP exposta
make new-java-module NAME=spell-worker

# Serviço — expõe porta HTTP além do Kafka
make new-java-module NAME=notification-service PORT=8082
```

### Após criar o módulo

1. Adicione dependências específicas em `java/<módulo>/pom.xml`
2. Implemente a lógica em `src/main/java/`
3. Suba só o novo módulo: `docker compose -f infra/docker-compose.yml up -d <módulo>`

---

## Portas locais

| Serviço      | Porta |
| ------------ | ----- |
| Gateway      | 8080  |
| User Service | 8081  |
| Postgres     | 5432  |
| Redis        | 6379  |
| Kafka        | 9092  |
| MinIO        | 9000  |
