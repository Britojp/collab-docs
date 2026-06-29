# CollabDocs

Editor de documentos colaborativo em tempo real. Múltiplos usuários podem editar o mesmo documento simultaneamente e ver as alterações do outro em tempo real.

Projeto desenvolvido para a disciplina **Sistemas Concorrentes e Distribuídos — UFG 2026.1**.

---

## Tecnologias

| Camada | Stack |
|--------|-------|
| Frontend | React 18 + TypeScript + Vite + nginx |
| Serviço de tempo real | Go 1.22 + Gin + gorilla/websocket |
| Backend / API / Analytics | Java 21 + Spring Boot 3.3 + Spring AMQP + gRPC |
| Gateway gRPC-Web | Envoy |
| Mensageria | RabbitMQ 3.13 |
| Banco de dados | PostgreSQL 16 |

---

## Rodando localmente

### Pré-requisitos

- [Docker](https://docs.docker.com/get-docker/) e Docker Compose instalados
- Portas **4000**, **8080**, **8081**, **8082**, **9090**, **5432**, **5672** e **15672** disponíveis no host

### 1. Clone o repositório

```bash
git clone <url-do-repositório>
cd collab-docs
```

### 2. Suba todos os serviços

```bash
docker compose -f infra/docker-compose.yml up -d --build
```

O primeiro build leva alguns minutos: Maven baixa dependências Java e gera classes gRPC a partir de `src/main/proto`, npm instala pacotes do frontend e Docker puxa a imagem do Envoy. Os próximos builds usam cache e sobem bem mais rápido.

Se quiser apenas validar o build sem iniciar containers:

```bash
docker compose -f infra/docker-compose.yml build
```

### 3. Aguarde os serviços ficarem prontos

```bash
docker compose -f infra/docker-compose.yml ps
```

Todos devem estar com status `running`. O Java Backend pode levar ~20s para iniciar após o PostgreSQL ficar healthy. A stack atual sobe 6 serviços: `postgres`, `rabbitmq`, `java-backend`, `grpc-web`, `go-collab` e `frontend`.

### 4. Acesse a aplicação

| Serviço | URL |
|---------|-----|
| Aplicação (frontend) | http://localhost:4000 |
| RabbitMQ Management | http://localhost:15672 (user: `collabdocs` / senha: `collabdocs`) |
| Java Backend (direto) | http://localhost:8081 |
| Java gRPC (direto) | localhost:9090 |
| Envoy gRPC-Web | http://localhost:8082 |
| Go Collab Service (direto) | http://localhost:8080 |

No navegador, o frontend acessa analytics por `/grpc/*`; o nginx encaminha para o Envoy, e o Envoy traduz gRPC-Web para o gRPC nativo do Java.

### 5. Login

Um usuário admin é criado automaticamente:

| Campo | Valor |
|-------|-------|
| E-mail | `admin@collabdocs.dev` |
| Senha | `admin123` |

Para testar a colaboração com dois usuários, use a opção **Cadastre-se** na tela de login para criar uma segunda conta.

---

## Testando a colaboração em tempo real

1. Abra http://localhost:4000 em **dois navegadores diferentes** (ou um normal + um anônimo/incógnito)
2. Faça login com contas diferentes em cada janela
3. Crie um documento em uma das janelas — ele aparecerá na sidebar de ambas
4. Abra o mesmo documento nos dois navegadores
5. Digite em um e observe as alterações aparecendo no outro em tempo real
6. Observe na barra superior o grupo **Analytics** com `chars`, palavras, linhas, parágrafos e versão, atualizados via gRPC-Web

> Usar dois navegadores diferentes (ou um em modo incógnito) é necessário porque o `localStorage` é compartilhado entre abas do mesmo navegador.

As métricas de analytics têm consistência eventual: o Go aplica a edição em tempo real, publica a operação no RabbitMQ, o Java persiste a operação em `documents.content` e o frontend consulta o Java via gRPC-Web.

---

## Parando os serviços

```bash
# Para os containers mas mantém os dados
docker compose -f infra/docker-compose.yml down

# Para e remove todos os dados (volumes)
docker compose -f infra/docker-compose.yml down -v
```

Use `down -v` quando quiser recriar o PostgreSQL e RabbitMQ do zero. Isso remove documentos, usuários criados manualmente, histórico de operações, métricas e filas persistidas.

---

## Estrutura do projeto

```
collab-docs/
├── docs/
│   ├── architecture.md       # Arquitetura detalhada do sistema
│   └── development-status.md # Relatório de desenvolvimento
├── frontend/                 # React SPA + nginx
├── go/
│   └── collab-service/       # Serviço Go (proxy + WebSocket hub)
├── java/
│   └── backend/              # Spring Boot (auth, docs, workers AMQP, analytics gRPC)
├── infra/
│   ├── docker-compose.yml    # Orquestração de todos os serviços
│   ├── envoy/
│   │   └── envoy.yaml        # Proxy gRPC-Web → gRPC Java
│   └── postgres/
│       ├── init.sql          # Schema do banco
│       └── seed.sql          # Dados iniciais (usuário admin)
└── Makefile
```

---

## Documentação

- [Arquitetura do sistema](docs/architecture.md)
- [Relatório de desenvolvimento](docs/development-status.md)
