# Relatório de Desenvolvimento — CollabDocs

**Disciplina:** Sistemas Concorrentes e Distribuídos (SCD 2026.1)  
**Professor:** Fábio Moreira Costa — UFG  
**Prazo:** 28/06/2026  
**Data do relatório:** 27/06/2026

Documento complementar: [Requisitos do cenário](requirements.md)

---

## Status Geral

| Camada | Status | Observações |
|--------|--------|-------------|
| Infraestrutura (Docker) | Completo | 7 containers: frontend, 2× Go, Java, Redis, RabbitMQ, PostgreSQL |
| Banco de dados (PostgreSQL) | Completo | Schema + seed; particionamento HASH em `operations` |
| Mensageria (RabbitMQ) | Completo | Topic exchange + 3 filas duráveis |
| Replicação (Redis) | Completo | Pub/Sub proposals/commits + lock SETNX por documento |
| Java Backend (auth + docs + workers) | Completo | REST, JWT, JPA, consumers AMQP |
| Go Collab Service (2 instâncias) | Completo | Proxy + Hub Actor + replicação Redis + failover |
| Frontend (React) | Completo | SPA, login, editor, presença, reconexão WS |
| Edição colaborativa em tempo real | Completo | Multi-instância validada; OT simplificado (limitação conhecida) |
| Persistência de conteúdo | Completo | Snapshot via `OperationConsumer.applyOperation` |
| Failover de liderança | Completo | Teste automatizado + guia manual |
| Workers assíncronos | Parcial | Persist e Metric funcionais; Spell é stub |
| Acesso na Internet | Parcial | Arquitetura pronta; exposição via túnel/VPS documentada em `requirements.md` |

---

## O que está implementado

### Infraestrutura
- Docker Compose orquestrando 7 serviços com healthchecks
- Rede interna Docker; portas expostas conforme necessidade de demo e testes
- PostgreSQL com volume persistente e schema versionado via `init.sql`
- Redis 7 para coordenação entre instâncias Go
- Seed automático com usuário admin de desenvolvimento
- Scripts de teste runtime multiplataforma (`runtime-consistency-test.*`)
- Script de reset completo (`infra/reset.ps1`)

### Autenticação
- Registro e login via Java (`/api/auth/register`, `/api/auth/login`)
- JWT HMAC-SHA256 com secret de 280 bits (acima do mínimo de 256 bits)
- Validade de 24h; validado no Go antes de qualquer rota protegida
- WebSocket autenticado via query param `?token=JWT`

### Documentos
- Criação, listagem e remoção de documentos
- Todos os usuários autenticados visualizam todos os documentos (modelo de workspace compartilhado)
- Restrição: somente o criador pode deletar seu próprio documento (403 para outros)
- Carregamento do conteúdo atual no Hub Go via `GET /internal/documents/:id/content`

### Replicação entre instâncias Go
- Duas instâncias `go-collab` e `go-collab-2` balanceadas pelo nginx (`frontend/nginx.conf`)
- Por documento: lock Redis `collabdocs:doc:{id}:leader` (SETNX + TTL 10s)
- Propostas: `collabdocs:doc:{id}:proposals` — qualquer instância publica ops recebidas
- Commits: `collabdocs:doc:{id}:commits` — líder publica ops ordenadas para todas as instâncias
- Endpoint `/health` retorna `nodeId` único por instância
- Failover: ao cair o líder, lock expira e outra instância assume — ver [failover-test.md](failover-test.md)

### Edição Colaborativa (WebSocket)
- Padrão Hub Actor: um goroutine Go por documento aberto
- Presença em tempo real: avatares coloridos de quem está no documento
- `diffToOps`: calcula operações mínimas (insert/delete) entre estado anterior e novo do textarea
- Operational Transformation simplificado: servidor (líder) é autoridade
- Cursor preservado após aplicação de ops remotas via `adjustCursor` + `requestAnimationFrame`
- Reconexão automática WebSocket (retry 1s); reset de versão por documento

### Pipeline de Mensagens (RabbitMQ)
- Cada operação confirmada pelo líder publicada em 3 filas (`op.persist`, `op.metric`, `op.spell`)
- `OperationConsumer`: persiste op em `operations` (particionada) **e** atualiza snapshot `documents.content/version`
- `MetricWorker`: upsert em `metrics` — total_ops, chars_inserted, chars_deleted
- `SpellWorker`: consome fila (stub — não popula `spell_issues` de forma útil)
- Painel de métricas no editor com polling a cada 10s

### Frontend
- SPA React com roteamento client-side (recarregar página mantém a rota)
- Design inspirado no Notion (paleta neutra, sidebar, área de editor centralizada)
- Login e registro na mesma tela com alternância de modo
- Indicador visual de conexão WebSocket (ponto verde/vermelho "ao vivo")
- Sidebar com indicador "(meu)" nos documentos criados pelo usuário logado

### Testes
- Teste automatizado principal: `infra/runtime-consistency-test.ps1`
- Valida: health das 2 instâncias Go, registro, colaboração cross-instância, snapshot, troca de documento, exclusão, failover
- Testes unitários Go: `internal/replication/redis_test.go`, `internal/hub/hub_replication_test.go`
- Guia central: [tests.md](tests.md)

---

## Bugs resolvidos durante o desenvolvimento

| Bug | Causa raiz | Solução |
|-----|-----------|---------|
| Login retornava 401 | Hash bcrypt no seed não correspondia a "admin123" | Geração de hash verificado com Python `bcrypt.checkpw` |
| `column "content" does not exist` | Volume PostgreSQL antigo sem a coluna | `docker compose down -v` para recriar volumes |
| JWT `WeakKeyException` | Secret com 31 chars (248 bits < 256 mínimo) | Secret aumentado para 35 chars (280 bits) |
| Wildcard conflict no Gin | Rota `/documents/:id/history` conflitava com `/documents/:id` | Rota de histórico removida |
| nginx falha ao iniciar | DNS `go-collab` não resolvido no startup | Upstream com múltiplos servidores Go |
| `nil pointer` em `fetchContent` | `resp` nil testado junto com `resp.StatusCode` | Separação em dois `if` independentes |
| JSON field mismatch | Go publicava `"char"` mas Java esperava `"character"` | Ajuste do json tag no struct Go |
| `ddl-auto: validate` com tabela particionada | JPA não consegue validar partições | Alterado para `ddl-auto: none` |
| Recarregar `/documents` redirecionava para login | nginx proxiava `/documents` para o Go (sem token) | Prefixo `/api/` para rotas de backend; SPA serve o resto |
| FK violation ao criar documento (500) | Token stale no localStorage com UUID de sessão antiga | Usuário precisa relogar; seed corrigido |
| Erro `Cannot read properties of undefined` | `content` em `ServerMessage` com `omitempty` | Removido `omitempty` do campo `Content` no struct Go |
| `versionRef` desatualizado no remetente | Sender não recebia próprio op de volta (`broadcastExcept`) | Incremento local de `versionRef` após cada `sendOp` |
| Conteúdo perdido após fechar documento | Snapshot não era atualizado no PostgreSQL | `OperationConsumer` chama `DocumentService.applyOperation` |
| Vazamento de conteúdo entre documentos | Mensagens WS de conexão anterior | Ignorar mensagens de sockets stale; reset por `docId` |
| Edição indisponível após queda do Go líder | Estado preso na instância caída | Redis lock + failover para instância sobrevivente |

---

## Pendências e limitações conhecidas

### OT Simplificado
A função `transform()` no Go retorna a operação sem modificação quando há delta de versão. Funciona corretamente com edição sequencial ou em posições diferentes; pode divergir quando dois usuários editam a **mesma posição simultaneamente**. OT completo exigiria buffer de ops do servidor.

### SpellWorker
O `SpellWorker` consome mensagens da fila `q.ops.spell` mas não implementa verificação ortográfica end-to-end. A tabela `spell_issues` existe no banco mas não é populada na prática.

### doc_permissions / audit_log
Tabelas existem no schema mas não são utilizadas. Modelo atual: workspace compartilhado sem ACL granular.

### Acesso na Internet
Multi-cliente está validado em localhost e via testes HTTP/WS contra containers. Exposição pública requer túnel (ngrok, Cloudflare) ou deploy em VPS — procedimento documentado em [requirements.md](requirements.md) §6.

### Failover — janela de indisponibilidade
O TTL do lock Redis (~10s) implica intervalo em que nenhuma instância é líder após queda abrupta do container líder. Clientes reconectam automaticamente após assumir novo líder.

---

## Requisitos da disciplina — matriz completa

Referência detalhada: [requirements.md](requirements.md)

| Requisito (enunciado) | Status | Mecanismo / evidência |
|-----------------------|--------|------------------------|
| Múltiplas linguagens de programação | **Sim** | TypeScript, Go, Java |
| Paradigmas: cliente-servidor, pub-sub, messaging | **Sim** | HTTP/WS; Redis + RabbitMQ topic; filas AMQP duráveis |
| Serviço acessível a múltiplos clientes na Internet | **Parcial** | Multi-cliente OK; exposição Internet documentada (túnel/VPS) |
| Vários componentes distribuídos (implementados) | **Sim** | 7 containers Docker independentes |
| Acessos concorrentes a recursos compartilhados | **Sim** | Hub Actor, documentos, métricas, filas |
| Processamento servidor concorrente com clientes | **Sim** | Workers AMQP + líder Go processam ops em paralelo à edição |
| Interação remota síncrona (bloqueante) | **Sim** | HTTP REST Go→Java (proxy autenticado) |
| Interação remota assíncrona | **Sim** | RabbitMQ publish/consume; Redis Pub/Sub |
| Replicação de funcionalidades | **Sim** | 2× Go Collab + nginx; workers desacoplados por fila |
| Replicação de dados (tempo real) | **Sim** | Redis commits replicam ops entre instâncias Go |
| Particionamento de dados | **Sim** | `operations` HASH(doc_id) — 4 partições |
| Particionamento de funcionalidades | **Sim** | Go (tempo real) / Java (auth+ORM) / workers (persist, métricas, spell) |
| Consistência de dados | **Parcial** | Líder autoritativo + versão monotônica + snapshot; OT simplificado |
| Disponibilidade das funcionalidades | **Sim** | Failover Redis, nginx `max_fails`, reconexão WS, testes runtime |
| Cenário + requisitos + arquitetura | **Sim** | `requirements.md`, `architecture.md`, `application.puml` |
| Containerização / orquestração | **Sim** | Docker Compose com healthchecks e dependências |

### Como demonstrar cada requisito

| Requisito | Demonstração |
|-----------|--------------|
| Multi-cliente / concorrência | Dois browsers em http://localhost:4000 editando o mesmo doc |
| Replicação Go | Teste runtime — cliente em `:8080` replica para cliente em `:8082` |
| Failover / disponibilidade | `runtime-consistency-test.ps1` ou [failover-test.md](failover-test.md) |
| Persistência / consistência | Teste runtime — snapshot em `GET /documents/{id}` após edição |
| Mensageria assíncrona | RabbitMQ Management UI (:15672) — filas `q.ops.*` recebendo mensagens |
| Particionamento | `\d+ operations_p*` no PostgreSQL — 4 partições físicas |
| Síncrono vs assíncrono | REST síncrono (login); ops via AMQP assíncrono (logs do `OperationConsumer`) |

---

## Próximos passos (opcional)

Itens que reforçam a entrega mas não bloqueiam o núcleo técnico:

1. Expor via Cloudflare Tunnel ou ngrok e registrar URL de demo no README
2. Completar `SpellWorker` com dicionário e endpoint para listar `spell_issues`
3. Implementar OT com buffer de ops do servidor
4. Ativar `doc_permissions` para controle de acesso por documento
