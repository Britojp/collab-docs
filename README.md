# CollabDocs

Editor de documentos colaborativo em tempo real. Múltiplos usuários podem editar o mesmo documento simultaneamente e ver as alterações do outro em tempo real.

Projeto desenvolvido para a disciplina **Sistemas Concorrentes e Distribuídos — UFG 2026.1**.

---

## Tecnologias

| Camada | Stack |
|--------|-------|
| Frontend | React 18 + TypeScript + Vite + nginx |
| Serviço de tempo real | Go 1.22 + Gin + gorilla/websocket |
| Backend / API | Java 21 + Spring Boot 3.3 + Spring AMQP |
| Replicação em tempo real | Redis 7 Pub/Sub + lock SETNX por documento |
| Mensageria | RabbitMQ 3.13 |
| Banco de dados | PostgreSQL 16 |

---

## Arquitetura

A visão detalhada está em [docs/architecture.md](docs/architecture.md).

Diagrama PlantUML da aplicação:

- [docs/application.puml](docs/application.puml)

Para renderizar localmente:

```bash
plantuml docs/application.puml
```

---

## Rodando localmente

### Pré-requisitos

- [Docker](https://docs.docker.com/get-docker/) e Docker Compose instalados
- Portas **4000**, **8080**, **8081**, **8082**, **5432**, **6379** e **15672** disponíveis no host

### 1. Clone o repositório

```bash
git clone <url-do-repositório>
cd collab-docs
```

### 2. Suba todos os serviços

```bash
docker compose -f infra/docker-compose.yml up -d --build
```

O primeiro build leva alguns minutos (Maven baixa dependências Java, npm instala pacotes). Os próximos sobem em segundos.

### 3. Aguarde os serviços ficarem prontos

```bash
docker compose -f infra/docker-compose.yml ps
```

Todos devem estar com status `running`. O Java Backend pode levar ~20s para iniciar após o PostgreSQL ficar healthy.

### 4. Acesse a aplicação

| Serviço | URL |
|---------|-----|
| Aplicação (frontend) | http://localhost:4000 |
| RabbitMQ Management | http://localhost:15672 (user: `collabdocs` / senha: `collabdocs`) |
| Java Backend (direto) | http://localhost:8081 |
| Go Collab Service (instância 1) | http://localhost:8080 |
| Go Collab Service (instância 2) | http://localhost:8082 |
| Redis (replicação Pub/Sub) | localhost:6379 |

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

> Usar dois navegadores diferentes (ou um em modo incógnito) é necessário porque o `localStorage` é compartilhado entre abas do mesmo navegador.

### Teste automatizado de consistência

Com a stack rodando, execute:

```powershell
powershell -ExecutionPolicy Bypass -File .\infra\runtime-consistency-test.ps1
```

O teste cria dois usuários, valida criação/exclusão entre clientes, replica conteúdo via WebSocket nas duas instâncias Go, confere o snapshot persistido no PostgreSQL e força failover de liderança.

Também há wrappers para outros ambientes:

| Ambiente | Comando |
|----------|---------|
| Windows BAT | `.\infra\runtime-consistency-test.bat` |
| Linux / WSL | `bash ./infra/runtime-consistency-test.sh` |
| macOS | `./infra/runtime-consistency-test.command` |

Veja a documentação completa em [docs/tests.md](docs/tests.md).

---

## Parando os serviços

```bash
# Para os containers mas mantém os dados
docker compose -f infra/docker-compose.yml down

# Para e remove todos os dados (volumes)
docker compose -f infra/docker-compose.yml down -v
```

### Reset completo (banco zerado + imagens locais)

No PowerShell, na raiz do projeto:

```powershell
.\infra\reset.ps1
docker compose -f infra/docker-compose.yml up -d --build
```

Ou via Make:

```bash
make down
docker compose -f infra/docker-compose.yml up -d --build
```

> **Importante:** `down` sem `-v` **não apaga** usuários/documentos — eles ficam no volume `infra_postgres_data`.
>
> No navegador, limpe o **localStorage** (ou use janela anônima) para remover tokens de sessão antigos.

---

## Estrutura do projeto

```
collab-docs/
├── docs/
│   ├── application.puml      # Diagrama PlantUML da aplicação
│   ├── architecture.md       # Arquitetura detalhada do sistema
│   ├── requirements.md       # Requisitos funcionais e não funcionais do cenário
│   ├── development-status.md # Relatório de desenvolvimento
│   ├── failover-test.md      # Teste de failover de liderança (runtime)
│   └── tests.md              # Guia central de testes
├── frontend/                 # React SPA + nginx
├── go/
│   └── collab-service/       # Serviço Go (proxy + WebSocket hub)
├── java/
│   └── backend/              # Spring Boot (auth, docs, workers AMQP)
├── infra/
│   ├── docker-compose.yml    # Orquestração de todos os serviços
│   ├── reset.ps1             # Reset completo do ambiente local
│   ├── runtime-consistency-test.ps1 # Teste runtime multi-cliente/failover (base)
│   ├── runtime-consistency-test.bat # Wrapper Windows
│   ├── runtime-consistency-test.sh  # Wrapper Linux/WSL
│   ├── runtime-consistency-test.command # Wrapper macOS
│   └── postgres/
│       ├── init.sql          # Schema do banco
│       └── seed.sql          # Dados iniciais (usuário admin)
└── Makefile
```

---

## Documentação

- [Requisitos do cenário](docs/requirements.md)
- [Arquitetura do sistema](docs/architecture.md)
- [Relatório de desenvolvimento](docs/development-status.md)
- [Testes do projeto](docs/tests.md)
- [Teste de failover de liderança](docs/failover-test.md)
- [Diagrama PlantUML da aplicação](docs/application.puml)
