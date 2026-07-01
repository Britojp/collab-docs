# Arquitetura do Sistema — CollabDocs

## Visão Geral

CollabDocs é um editor de documentos colaborativo em tempo real construído como sistema distribuído concorrente. A arquitetura separa responsabilidades em serviços especializados que se comunicam via HTTP, WebSocket e mensageria assíncrona, e é pensada para rodar com **múltiplas instâncias do serviço Go em nós físicos distintos** — não apenas como containers no mesmo host, mas como o deploy real na AWS (`infra/aws/`) demonstra.

```
┌─────────────────────────────────────────────────────────────┐
│                        Browser                              │
│              React SPA  (localhost:4000)                    │
└────────────────────────┬────────────────────────────────────┘
                         │ HTTP / WebSocket
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Nginx (reverse proxy)                      │
│  /api/ws/:docId  → hash(docId) consistente                  │
│  /api/*          → round-robin                              │
│  /*              → index.html (SPA)                         │
└────────────────────────┬────────────────────────────────────┘
                         │
              ┌──────────┴──────────┐
              │                     │
              ▼                     ▼
   ┌──────────────────┐   ┌──────────────────────┐
   │  go-collab-1      │   │  go-collab-2         │
   │  (REST proxy +    │   │  (REST proxy +       │
   │   Hub Manager)    │   │   Hub Manager)        │
   └────────┬─────────┘   └──────────┬───────────┘
            │ HTTP proxy              │ AMQP publish
            │                         ├───────────────┐
            ▼                         ▼               ▼
   ┌──────────────────┐   ┌──────────────────────┐
   │  Java Backend    │   │      RabbitMQ         │
   │  Spring Boot     │   │  exchange: collab     │
   │  (auth, docs,    │   │  (topic)             │
   │   metrics, ORM)  │   └──────────┬───────────┘
   └────────┬─────────┘              │
            │ JPA                    │ consume
            ▼                        ▼
   ┌──────────────────┐   ┌──────────────────────┐
   │   PostgreSQL 16  │◀──│  Java Workers        │
   │   (partitioned)  │   │  OperationConsumer   │
   └──────────────────┘   │  MetricWorker        │
                          │  SpellWorker         │
                          └──────────────────────┘

   ┌──────────────────────────────────────────────────────────┐
   │ Redis — coordenação entre instâncias go-collab            │
   │ doc:{id}:proposals → operações recebidas por qualquer nó  │
   │ doc:{id}:commits   → operações ordenadas pelo líder       │
   │ doc:{id}:cursors   → posição de cursor, fan-out direto    │
   │ doc:{id}:presence  → roster de clientes por nó            │
   │ doc:{id}:leader    → lock de liderança (SETNX + TTL)      │
   │ doc:{id}:epoch     → contador monotônico (fencing token)  │
   └──────────────────────────────────────────────────────────┘
```

Na AWS (ver `infra/aws/`), cada um desses papéis é uma **instância EC2 separada** — `go-collab-1` e `go-collab-2` ficam em AZs diferentes (`us-east-1a`/`us-east-1b`), forçando o sistema a resolver de verdade os problemas de coordenação entre nós, em vez de mascará-los rodando tudo na mesma máquina.

---

## Serviços

### Frontend — React + Vite
- **Porta:** 4000 (via nginx)
- **Responsabilidade:** SPA que serve a interface de usuário. Comunica-se exclusivamente com o serviço Go via HTTP e WebSocket.
- **Tecnologias:** React 18, TypeScript, Vite, nginx
- **Roteamento:** React Router. nginx serve `index.html` para qualquer rota SPA; rotas `/api/*` são proxiadas para o Go.

### Go Collab Service
- **Porta:** 8080
- **Responsabilidade dual:**
  1. **Proxy autenticado:** valida JWT e encaminha requisições REST para o Java Backend, injetando cabeçalhos `X-User-ID` e `X-User-Name`.
  2. **Hub WebSocket:** gerencia edição colaborativa em tempo real usando o padrão Actor por documento.
- **Tecnologias:** Go 1.22, Gin, gorilla/websocket, golang-jwt, amqp091-go, redis/go-redis
- Roda em **múltiplas instâncias stateful** (`go-collab-1`, `go-collab-2`) — cada uma mantém em memória o estado (`content`, `version`, cursores, presença) apenas dos documentos que seus próprios clientes têm abertos, sincronizado com as demais via Redis quando necessário.

#### Padrão Hub Actor
Cada documento aberto cria um goroutine exclusivo (`Hub.run()`) que é o único responsável por ler e escrever o estado do documento. Clientes interagem via canais Go, eliminando a necessidade de mutex no estado compartilhado.

```
Client A ──send chan──▶ Hub goroutine ──send chan──▶ Client B
                            │
                        (content, version)
                            │
                        AMQP publish
```

O `Hub` de um documento é criado sob demanda (`Manager.GetOrCreate`) e nunca é destruído enquanto o processo Go estiver de pé — mesmo sem clientes conectados, ele continua vivo, renovando/tentando liderança e recebendo replicação via Redis, para que reconexões e novos clientes encontrem estado consistente.

#### Operational Transformation (OT)
Cada operação (`insert` / `delete`) carrega `pos` e `char`. O servidor é a autoridade: operações recebidas com `clientVersion` defasado passam por `transformSince()` contra todas as operações do servidor desde aquela versão antes de serem aplicadas. O **cliente** também faz sua parte da transformação (ver [Fluxo de Edição em Tempo Real](#fluxo-de-edição-em-tempo-real)), mantendo uma fila de operações locais ainda não confirmadas (`pendingRef`) e transformando operações remotas recebidas contra essa fila antes de aplicá-las ao buffer local — e vice-versa, para manter a fila consistente com o novo estado do servidor.

### Java Backend — Spring Boot 3.3
- **Porta:** 8081 (interna; não exposta diretamente ao frontend), 9090 (gRPC, para analytics)
- **Responsabilidade:** autenticação, persistência de documentos, persistência de operações, métricas e verificação ortográfica — tudo via consumers AMQP assíncronos.
- **Tecnologias:** Java 21, Spring Boot, Spring Security, Spring Data JPA, Spring AMQP, jjwt 0.12, gRPC

**Endpoints internos relevantes:**
| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/auth/register` | Cadastro de usuário |
| POST | `/auth/login` | Login; retorna JWT |
| GET | `/documents` | Lista todos os documentos |
| POST | `/documents` | Cria documento |
| DELETE | `/documents/:id` | Remove documento (somente dono) |
| GET | `/metrics/:docId` | Métricas de uso do documento |
| GET | `/internal/documents/:id/content` | Conteúdo atual (chamado pelo Go Hub) |
| gRPC | `AnalyticsService.GetDocumentAnalytics` | Estatísticas do documento (via Envoy/gRPC-Web) |

### RabbitMQ
- **Portas:** 5672 (AMQP), 15672 (Management UI)
- **Exchange:** `collab` (topic, durable)
- **Filas e routing keys:**

| Fila | Routing Key | Consumer |
|------|-------------|----------|
| `q.ops.persist` | `op.persist` | `OperationConsumer` — persiste op no PostgreSQL |
| `q.ops.metric` | `op.metric` | `MetricWorker` — atualiza contadores de métricas |
| `q.ops.spell` | `op.spell` | `SpellWorker` — verifica ortografia (stub) |

Cada operação feita no editor é publicada simultaneamente nas três filas, permitindo processamento paralelo e desacoplado.

### Redis — coordenação entre instâncias Go

Redis é o que permite `go-collab-1` e `go-collab-2` se comportarem como **um único serviço lógico** apesar de rodarem em processos (e, na AWS, máquinas) diferentes. Cinco mecanismos, todos por documento (`docId`):

| Canal/Chave | Conteúdo | Padrão |
|---|---|---|
| `collabdocs:doc:{id}:proposals` | Operações recebidas por qualquer nó, ainda não ordenadas | Pub/Sub |
| `collabdocs:doc:{id}:commits` | Operações já ordenadas e aplicadas pelo líder | Pub/Sub |
| `collabdocs:doc:{id}:cursors` | Posição de cursor de um cliente | Pub/Sub, fan-out direto (sem líder — último valor prevalece) |
| `collabdocs:doc:{id}:presence` | Roster de clientes conectados *naquele nó* | Pub/Sub, heartbeat a cada 3s + em toda entrada/saída |
| `collabdocs:doc:{id}:leader` | Node ID do líder atual | `SETNX` + TTL de 10s, renovado a cada 3s |
| `collabdocs:doc:{id}:epoch` | Contador monotônico da liderança (fencing token) | `INCR`, nunca expira |

Apenas o **líder** de um documento transforma, incrementa versão e confirma (`commit`) operações — os demais nós apenas retransmitem propostas e aplicam commits já ordenados. Cursor e presença não precisam de líder: são replicados por fan-out direto, já que não exigem ordenação total, apenas "o valor mais recente vence".

Redis não substitui RabbitMQ: Redis mantém a experiência em tempo real entre Hubs (baixa latência, sem durabilidade); RabbitMQ/PostgreSQL continuam sendo o caminho durável para persistência, métricas, workers e snapshot de conteúdo.

### PostgreSQL 16
- **Porta:** 5432
- **Schema principal:**

| Tabela | Descrição |
|--------|-----------|
| `users` | Usuários (UUID, email, bcrypt hash) |
| `documents` | Documentos (título, conteúdo atual, versão) |
| `doc_permissions` | Controle de acesso por documento |
| `operations` | Histórico de todas as operações — **particionada por HASH(doc_id)** em 4 partições |
| `metrics` | Contadores agregados por documento (total_ops, chars_inserted, chars_deleted) |
| `spell_issues` | Problemas ortográficos detectados |
| `audit_log` | Log de auditoria de eventos |

A tabela `operations` usa particionamento por hash para distribuir o volume de escrita entre partições físicas independentes.

---

## Deploy Distribuído (AWS)

O diretório `infra/aws/` (Terraform) provisiona o sistema em **5 instâncias EC2 separadas**, espelhando o desenho lógico acima em nós físicos distintos:

| Instância | Serviços | AZ |
|---|---|---|
| `data` | postgres, rabbitmq, redis | `us-east-1a` |
| `java-backend` | Spring Boot | `us-east-1a` |
| `go-collab-1` | Go Collab Service | `us-east-1a` |
| `go-collab-2` | Go Collab Service | `us-east-1b` |
| `edge` | nginx (frontend) + Envoy (grpc-web) | `us-east-1a` |

As duas instâncias `go-collab` ficam **deliberadamente em AZs diferentes** — o objetivo do deploy é forçar cenários reais de coordenação entre nós (líder eleito num nó, cliente conectado no outro; latência de rede real entre AZs) em vez de permitir que tudo se resolva "por sorte" rodando na mesma máquina, como aconteceria num único host Docker.

Descoberta de serviço entre instâncias usa uma **zona privada do Route53** (`collabdocs.internal`), eliminando a necessidade de IPs fixos: `postgres.collabdocs.internal`, `go-collab.collabdocs.internal`, `go-collab-2.collabdocs.internal`, etc.

---

## Roteamento de Documentos entre Nós

O nginx do `edge` usa **duas estratégias de balanceamento diferentes**, dependendo se a rota carrega estado ou não:

```nginx
# REST — stateless, qualquer instância serve
upstream go_collab_rest {
    server go-collab:8080;
    server go-collab-2:8080;
}

# WebSocket — hash consistente por docId
upstream go_collab_ws {
    hash $docid consistent;
    server go-collab:8080;
    server go-collab-2:8080;
}
```

- **Chamadas REST** (`GET /api/documents`, `POST /api/documents`, etc.) são um proxy stateless para o Java — não importa qual instância Go atende, então usam round-robin simples.
- **Conexões WebSocket** (`/api/ws/:docId`) carregam o estado do `Hub` daquele documento em memória, na instância que a atende. Rotear por **hash consistente do `docId`** garante que, no caso comum, todos os clientes de um mesmo documento caiam sempre no mesmo nó — eliminando a necessidade de replicação cross-node de operações, cursor e presença no caminho feliz, e reduzindo a exposição aos bugs de coordenação entre nós a apenas cenários de failover (queda de instância, rebalanceamento).

Note que essa é uma otimização do caminho comum, não uma eliminação da complexidade: a replicação via Redis (líder, commits, cursor, presença) continua necessária para os casos em que documentos diferentes acabam no mesmo nó por acaso, ou quando uma instância cai e o hash consistente redistribui suas chaves para a outra.

---

## Fluxo de Edição em Tempo Real

```
Usuário A digita 'a'
       │
       ▼
EditorPage.handleChange()
  diffToOps(old, new) → [{insert, pos:5, char:'a'}]
  sendOp(op) via WebSocket        (clientVersion anexado; op empilhado em pendingRef)
  ajusta cursors[] e highlights[] locais (o próprio insert também desloca
  o que já sabemos sobre o cursor de outros usuários)
       │
       ▼ ws://host/api/ws/:docId?token=JWT
       │
    nginx (hash($docId) consistente → mesma instância go-collab de sempre)
       │
       ▼ ws://go-collab-N:8080/ws/:docId
       │
    Hub.ReadPump() → incoming chan
       │
    Hub.run() → PublishProposal(Redis)
       │
       ▼ doc:{id}:proposals
    Hub líder do documento
       ├── verifica epoch atual no Redis == leaderEpoch (fencing) — senão, se retira
       ├── transformSince(op, servidor.ops, clientVersion)
       ├── apply(content, op)
       ├── version++
       ├── PublishDocEvent(...)      → RabbitMQ (persist + snapshot + metric + spell)
       └── broadcastCommit + PublishCommit(Redis, epoch atual)
       │
       ▼ doc:{id}:commits
    Hubs em todas as instâncias Go (exceto o líder, que já aplicou direto)
       ├── descarta se commit.epoch < highestSeenEpoch (fencing do lado do seguidor)
       ├── apply(content, op)
       └── broadcast local           → clientes conectados naquela instância
       │
       ▼ WebSocket frame para Usuário B
       │
    useWebSocket.onmessage()
       ├── transforma o op recebido contra a fila local de pendingRef (nossas
       │   edições ainda não confirmadas), e vice-versa
       │
    handleMessage({ type:'op', op:{...}, userId })
       │
    setContent(prev => applyOp(prev, op))     → textarea atualiza
    ajusta myCursorRef, cursors[] e highlights[]  → cursor local e remoto
       │                                          continuam apontando pro
       │                                          caractere certo
    useLayoutEffect(() => ta.selectionStart = myCursorRef.current)
       │                                          (síncrono, antes do paint —
       │                                          nunca lê o DOM como fonte)
    flash de highlight colorido (cor do autor)  → atribuição visual da edição
```

---

## Autenticação

O JWT é gerado pelo Java com chave HMAC-SHA256 (≥ 256 bits). O Go valida o token em todas as rotas protegidas — incluindo WebSocket via query param `?token=`. Após validação, o Go injeta `X-User-ID` (subject do JWT) e `X-User-Name` nos cabeçalhos antes de fazer proxy para o Java.

```
Frontend → Authorization: Bearer <JWT>
                │
            Go JWT middleware
                │ valid
                ├── c.Set("userID", claims.Subject)
                └── proxy → X-User-ID: <uuid>
                                 │
                             Java Controller
                             @RequestHeader("X-User-ID")
```

---

## Concorrência e Distribuição

| Aspecto | Mecanismo |
|---------|-----------|
| Estado do documento | Goroutine Actor exclusivo por documento (sem mutex) |
| Múltiplos clientes no mesmo doc | Canais Go (`register`, `unregister`, `incoming`) |
| Múltiplos documentos simultâneos | `Manager` com `sync.RWMutex` + double-checked locking |
| Roteamento de conexões WS | Hash consistente por `docId` no nginx — afinidade nó↔documento no caminho comum |
| Ordenação de operações entre nós | Eleição de líder por documento via Redis `SETNX` + TTL; só o líder ordena/versiona |
| Proteção contra líder obsoleto (split-brain) | **Fencing token** — epoch monotônico (`INCR` no Redis); líder confere o epoch atual antes de aplicar cada proposta; seguidores rejeitam commits de epoch mais antigo que o já visto |
| OT no servidor | `transformSince()` contra o histórico de ops desde a versão do cliente |
| OT no cliente | Fila de operações pendentes (`pendingRef`); toda operação remota é transformada contra ela antes de aplicar, e ela é transformada contra a remota antes de reenviar |
| Replicação de conteúdo entre nós | Redis Pub/Sub (`proposals`/`commits`), líder aplica direto, seguidores via `handleCommit` |
| Replicação de cursor entre nós | Redis Pub/Sub (`cursors`) — fan-out direto, sem ordenação (posição é *last-write-wins*) |
| Replicação de presença entre nós | Redis Pub/Sub (`presence`) — snapshot do roster local, heartbeat de 3s, mesclado por nó de origem |
| Sincronia do cursor local (cliente) | Ref síncrono (`myCursorRef`) ajustado por toda edição — local ou remota — e aplicado ao DOM via `useLayoutEffect`, nunca lido de volta do DOM como fonte de verdade |
| Processamento assíncrono de ops | RabbitMQ topic exchange; workers Java independentes |
| Workers em paralelo (spell) | `@RabbitListener(concurrency = "2")` |
| Particionamento de dados | PostgreSQL HASH partition em `operations` |

---

## Falhas de Concorrência Identificadas e Corrigidas

Registro dos bugs de coordenação distribuída encontrados ao testar o sistema com duas instâncias `go-collab` reais (não apenas no mesmo host) — cada um só se manifestava sob condições específicas de distribuição, exatamente o tipo de bug que testes locais de nó único não pegam.

### 1. Nó não-líder nunca confirmava a própria operação do cliente
`handleCommit` verificava `commit.OriginNodeID == h.bus.NodeID()` para evitar que o líder reprocessasse o eco do próprio commit publicado no Redis — mas essa checagem usa a identidade errada. Se o cliente está conectado a um nó que **não** é o líder (comum sem afinidade de roteamento), o commit de volta via Redis também tem `OriginNodeID` igual ao desse nó, fazendo-o descartar silenciosamente a confirmação da própria operação do seu cliente. O `ack` nunca chegava, a fila de pendências do frontend nunca esvaziava, e toda edição remota subsequente era transformada contra uma fila cada vez mais desatualizada.
**Fix:** a checagem correta é `if h.isLeader { return }` — só o líder já aplicou o commit diretamente; qualquer outro nó, seja ou não a origem do cliente, precisa processá-lo.

### 2. Cursor e presença nunca cruzavam nós
`handleCursor` e `broadcastPresence` faziam apenas broadcast local (`h.clients`, restrito ao processo daquele nó) — nunca passavam pelo Redis. Clientes em nós diferentes nunca viam o cursor ou a presença um do outro.
**Fix:** dois canais Redis novos (`cursors`, `presence`) espelhando o padrão já usado para operações — fan-out direto para cursor (sem necessidade de ordenação), snapshot com heartbeat para presença (mesclado por nó de origem).

### 3. Ajuste de cursor local sujeito a race condition
No frontend, ao receber uma operação remota, o código lia `ta.selectionStart` do DOM, calculava a correção e a aplicava via `requestAnimationFrame` — mas se várias operações remotas chegassem antes do próximo repaint (comum quando o outro usuário digita rápido), cada leitura via DOM ficava desatualizada em relação à correção anterior, ainda não aplicada. O cursor local acumulava um erro que "puxava" visualmente na direção de onde a edição remota acontecia.
**Fix:** posição do cursor local passou a ser mantida num ref (`myCursorRef`) atualizado de forma síncrona e encadeada a cada operação, nunca lido de volta do DOM; a aplicação ao `<textarea>` acontece via `useLayoutEffect`, antes do próximo paint.

### 4. Cursor remoto nunca era ajustado pela própria digitação
O ajuste de posição de cursores remotos (mapa `cursors`, renderizado como indicador visual do outro usuário) só acontecia ao receber uma operação **remota** — nunca ao processar a própria digitação local. Resultado: ao digitar antes da posição registrada do cursor de outro usuário, o indicador dele ficava desatualizado até a próxima atualização de cursor vinda dele, aparentando "ficar atrás" do texto.
**Fix:** `handleChange` agora aplica o mesmo ajuste (`adjustCursor`) em `cursors` e `highlights` para cada operação gerada localmente, simetricamente ao que já acontecia para operações remotas.

### 5. Eleição de líder sem fencing token (split-brain)
A eleição de líder via `SETNX` + TTL detecta corretamente quando *outro* nó já é líder, mas não protege contra o cenário em que o próprio nó líder pausa (GC longo, CPU throttling) por mais tempo que o TTL, um novo líder assume, e o nó original **retoma** ainda acreditando ser líder (seu `isLeader` só seria corrigido no próximo tick, até 3s depois). Nesse intervalo, ele poderia aplicar propostas e publicar commits divergentes da linha do tempo do novo líder.
**Fix:** *fencing token* — um contador de época (`epoch`) monotônico no Redis, incrementado a cada nova aquisição de liderança (nunca em renovação). Cada commit carrega o epoch do líder que o produziu; antes de aplicar qualquer proposta, o líder confere se seu epoch ainda é o vigente no Redis (senão, se retira sem aplicar nada); e todo nó descarta qualquer commit cujo epoch seja mais antigo que o mais recente já visto, mesmo que a versão pareça plausível.

Todos os cinco fixes têm testes de regressão em `go/collab-service/internal/hub/hub_replication_test.go` e `internal/replication/redis_test.go` (este último com Redis real via `miniredis`), reproduzindo cada cenário antes e depois da correção.
