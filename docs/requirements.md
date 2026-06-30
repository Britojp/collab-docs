# Requisitos do Cenário — CollabDocs

**Disciplina:** Sistemas Concorrentes e Distribuídos (SCD 2026.1)  
**Instituição:** UFG  
**Cenário:** Editor de documentos colaborativo em tempo real

---

## 1. Contexto e problema

Organizações e equipes precisam editar textos compartilhados com feedback imediato — como notas de reunião, rascunhos ou documentação interna. Um editor tradicional (salvar e recarregar) não atende edição simultânea: conflitos de versão, perda de alterações e ausência de presença em tempo real.

O **CollabDocs** resolve esse problema com um sistema distribuído e concorrente: múltiplos usuários editam o mesmo documento ao mesmo tempo, veem alterações uns dos outros em tempo real e têm o histórico persistido de forma assíncrona no servidor.

---

## 2. Objetivos do sistema

| ID | Objetivo |
|----|----------|
| OBJ-01 | Permitir autenticação segura de usuários |
| OBJ-02 | Gerenciar documentos compartilhados (criar, listar, excluir) |
| OBJ-03 | Sincronizar edições em tempo real entre múltiplos clientes |
| OBJ-04 | Persistir operações e snapshot do conteúdo de forma durável |
| OBJ-05 | Processar tarefas auxiliares (métricas, ortografia) sem bloquear a edição |
| OBJ-06 | Manter disponibilidade da edição mesmo com falha de uma instância do serviço de tempo real |

---

## 3. Requisitos funcionais

| ID | Requisito | Prioridade | Implementação |
|----|-----------|------------|---------------|
| RF-01 | O usuário deve poder se registrar com e-mail, nome e senha | Alta | `AuthController` (Java) — `POST /auth/register` |
| RF-02 | O usuário deve poder fazer login e receber um token JWT | Alta | `AuthController` (Java) — `POST /auth/login`; validação no Go |
| RF-03 | O usuário autenticado deve poder criar documentos com título | Alta | `DocumentController` — `POST /documents` |
| RF-04 | O usuário autenticado deve poder listar todos os documentos do workspace | Alta | `DocumentController` — `GET /documents` |
| RF-05 | Somente o criador do documento pode excluí-lo | Média | `DocumentService.delete` — verificação de `ownerId` |
| RF-06 | Dois ou mais usuários devem editar o mesmo documento simultaneamente | Alta | WebSocket + Hub Actor (Go) + Redis replication |
| RF-07 | Alterações de um usuário devem aparecer nos demais clientes conectados ao documento | Alta | Broadcast via WebSocket após commit Redis |
| RF-08 | O sistema deve exibir quem está online no documento (presença) | Média | Mensagem `presence` no protocolo WebSocket |
| RF-09 | O conteúdo editado deve ser recuperável após fechar e reabrir o documento | Alta | `OperationConsumer` aplica ops em `documents.content` |
| RF-10 | O editor deve exibir métricas de uso do documento | Baixa | `MetricWorker` + painel com polling no frontend |
| RF-11 | Ao trocar de documento, o conteúdo exibido deve corresponder ao documento aberto | Alta | Reset de versão/conexão WS por `docId` no frontend |

---

## 4. Requisitos não funcionais

### 4.1 Distribuição e integração

| ID | Requisito | Implementação |
|----|-----------|---------------|
| RNF-01 | O sistema deve ser composto por múltiplos componentes distribuídos implementados no trabalho | Frontend (React/nginx), 2× Go Collab, Java Backend, Redis, RabbitMQ, PostgreSQL |
| RNF-02 | Deve empregar mais de uma linguagem de programação | TypeScript (frontend), Go (tempo real/proxy), Java (persistência/workers) |
| RNF-03 | Deve usar paradigmas cliente-servidor, publish-subscribe e messaging | HTTP REST (cliente-servidor), Redis/RabbitMQ Pub/Sub, filas AMQP |
| RNF-04 | Deve ser acessível a múltiplos clientes simultâneos | nginx balanceia entre 2 instâncias Go; WebSocket por documento; ver [§6](#6-acesso-na-internet) |

### 4.2 Concorrência

| ID | Requisito | Implementação |
|----|-----------|---------------|
| RNF-05 | Deve suportar acessos concorrentes a recursos compartilhados | Hub Actor por documento (goroutine + channels); `@Transactional` em métricas |
| RNF-06 | Deve processar dados no servidor concorrentemente com os acessos dos clientes | Workers AMQP (`OperationConsumer`, `MetricWorker`, `SpellWorker`) em paralelo com edição WS |
| RNF-07 | Deve usar interação remota síncrona (bloqueante) e assíncrona | Síncrona: HTTP Go→Java; Assíncrona: AMQP + Redis Pub/Sub |

### 4.3 Replicação e particionamento

| ID | Requisito | Implementação |
|----|-----------|---------------|
| RNF-08 | Deve replicar funcionalidades entre instâncias | 2× `go-collab` com nginx upstream; liderança por documento via Redis `SETNX` |
| RNF-09 | Deve replicar estado de edição entre instâncias Go | Redis Pub/Sub: `proposals` (entrada) e `commits` (saída ordenada pelo líder) |
| RNF-10 | Deve particionar dados | Tabela `operations` particionada por `HASH(doc_id)` em 4 partições físicas |
| RNF-11 | Deve particionar funcionalidades | Go (tempo real), Java (auth/ORM), workers (persist/métricas/spell) |

### 4.4 Consistência e disponibilidade

| ID | Requisito | Implementação |
|----|-----------|---------------|
| RNF-12 | Deve garantir ordenação autoritativa das operações | Apenas o líder Redis transforma, incrementa versão e publica commits |
| RNF-13 | Deve persistir snapshot consistente com versão monotônica | `DocumentService.applyOperation` ignora ops com `version <= doc.version` |
| RNF-14 | Deve manter disponibilidade após falha do líder | TTL do lock Redis (~10s); outra instância Go assume liderança — ver [failover-test.md](failover-test.md) |
| RNF-15 | Deve reconectar clientes após queda de WebSocket | `useWebSocket` com retry a cada 1s; mensagem `resync` ao reconectar |
| RNF-16 | Deve isolar falhas de persistência sem perder mensagens | Re-queue AMQP em falha no `OperationConsumer` |

---

## 5. Mapeamento: requisitos da disciplina → CollabDocs

Requisitos obrigatórios do enunciado da disciplina e como o projeto os atende:

| Requisito da disciplina | Atendido | Evidência no CollabDocs |
|-------------------------|----------|-------------------------|
| Serviço acessível a múltiplos clientes na Internet | Parcial | Multi-cliente demonstrado localmente; exposição pública documentada em [§6](#6-acesso-na-internet) |
| Integração de vários componentes distribuídos | Sim | 7 containers Docker; ver [architecture.md](architecture.md) |
| Acessos concorrentes a recursos compartilhados | Sim | Hub Actor, documentos, métricas, filas AMQP |
| Processamento servidor concorrente com clientes | Sim | Workers Java + líder Go processam ops enquanto clientes editam |
| Interação remota síncrona e assíncrona | Sim | HTTP (sync), AMQP + Redis Pub/Sub (async) |
| Replicação de dados e funcionalidades | Sim | 2× Go + Redis; workers desacoplados; snapshot PostgreSQL |
| Particionamento de dados e funcionalidades | Sim | HASH partition em `operations`; separação Go/Java/workers |
| Consistência de dados | Parcial | Líder + versão monotônica + snapshot; OT simplificado (limitação conhecida) |
| Disponibilidade das funcionalidades | Sim | Failover Redis, nginx `max_fails`, reconexão WS, testes automatizados |
| Cenário com requisitos e arquitetura | Sim | Este documento + [architecture.md](architecture.md) + [application.puml](application.puml) |
| Múltiplas linguagens | Sim | TypeScript, Go, Java |
| Paradigmas cliente-servidor, pub-sub, messaging | Sim | HTTP/WS, Redis/RabbitMQ topic, filas duráveis |

---

## 6. Acesso na Internet

O enunciato exige serviço acessível a múltiplos clientes **na Internet**. O ambiente padrão roda em `localhost`, mas a arquitetura (containers + nginx na porta 4000) permite exposição pública.

### Opção A — Túnel (demonstração rápida)

Com a stack rodando, exponha a porta 4000:

```bash
# Exemplo com Cloudflare Tunnel (instalar cloudflared previamente)
cloudflared tunnel --url http://localhost:4000
```

Ou com ngrok:

```bash
ngrok http 4000
```

Compartilhe a URL gerada. Dois participantes em redes diferentes podem acessar o mesmo editor.

### Opção B — Deploy em VPS/cloud

1. Clone o repositório no servidor.
2. Abra a porta 4000 (ou 80/443 com proxy TLS).
3. Execute `docker compose -f infra/docker-compose.yml up -d --build`.
4. Acesse `http://<ip-do-servidor>:4000`.

> **Nota:** Para produção real seriam necessários TLS, secrets seguros (`JWT_SECRET`) e firewall. Para fins acadêmicos, túnel ou VPS com porta aberta são suficientes para demonstrar acesso na Internet.

---

## 7. Casos de uso principais

### UC-01 — Editar documento colaborativamente

**Atores:** Usuário A, Usuário B  
**Pré-condição:** Ambos autenticados; documento existente  
**Fluxo:**

1. A e B abrem o mesmo documento (podem cair em instâncias Go diferentes via nginx).
2. A digita texto → frontend envia op via WebSocket.
3. Instância Go de A publica proposta no Redis.
4. Líder do documento aplica op, publica no RabbitMQ e commit no Redis.
5. Todas as instâncias Go recebem commit e broadcast local.
6. B vê a alteração em tempo real.

**Pós-condição:** Op persistida em `operations` e snapshot atualizado em `documents`.

### UC-02 — Failover de liderança

**Atores:** Sistema (infraestrutura)  
**Pré-condição:** Documento aberto; líder identificado  
**Fluxo:**

1. Container do líder é encerrado (`docker stop`).
2. Lock Redis expira após TTL (~10s).
3. Instância sobrevivente adquire liderança via `SETNX`.
4. Clientes reconectam (WS retry) e recebem `resync` com conteúdo do PostgreSQL.

**Pós-condição:** Edição continua disponível. Procedimento detalhado em [failover-test.md](failover-test.md).

---

## 8. Restrições e premissas

| Tipo | Descrição |
|------|-----------|
| Premissa | Workspace compartilhado: todos os usuários autenticados veem e editam todos os documentos |
| Premissa | Operações limitadas a `insert` e `delete` de caractere único (modelo OT simplificado) |
| Restrição | PostgreSQL e Java rodam em instância única (sem read replica) |
| Restrição | Redis é volátil (AOF desabilitado); estado durável está no PostgreSQL via RabbitMQ |
| Restrição | Failover tem janela de indisponibilidade de até ~10s (TTL do lock) |

---

## 9. Limitações conhecidas

| Limitação | Impacto | Mitigação atual |
|-----------|---------|-----------------|
| OT simplificado (`transform()` stub) | Divergência se dois usuários editam a mesma posição ao mesmo tempo | Servidor é autoridade; mensagem `resync` ao conectar |
| SpellWorker incompleto | Ortografia não exibida ao usuário | Worker consome fila (demonstra messaging); implementação real é extensão futura |
| `doc_permissions` / `audit_log` não usados | Sem ACL granular nem auditoria | Fora do escopo mínimo; schema reservado |
| Acesso Internet não automatizado | Requer túnel ou deploy manual | Documentado em [§6](#6-acesso-na-internet) |

---

## 10. Critérios de aceite (testes)

| Critério | Como validar |
|----------|--------------|
| Colaboração multi-instância | `infra/runtime-consistency-test.ps1` (ou `.sh`/`.bat`) |
| Persistência de snapshot | Teste automatizado — etapa "Snapshot persistido" |
| Failover de liderança | Teste automatizado + [failover-test.md](failover-test.md) |
| Isolamento entre documentos | Teste automatizado — etapa "Troca de documento" |
| Colaboração manual (2 browsers) | [README.md](../README.md) — seção "Testando a colaboração" |

Documentação completa de testes: [tests.md](tests.md).

---

## 11. Documentos relacionados

| Documento | Conteúdo |
|-----------|----------|
| [architecture.md](architecture.md) | Arquitetura técnica detalhada |
| [application.puml](application.puml) | Diagrama de componentes (PlantUML) |
| [development-status.md](development-status.md) | Relatório de desenvolvimento e status |
| [tests.md](tests.md) | Guia de testes automatizados e manuais |
| [failover-test.md](failover-test.md) | Procedimento de teste de failover |
