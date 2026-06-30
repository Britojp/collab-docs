package hub

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/britojp/collabdocs/go/collab-service/internal/mq"
	"github.com/britojp/collabdocs/go/collab-service/internal/replication"
)

const (
	leadershipTTL      = 10 * time.Second
	leadershipRenewal  = 3 * time.Second
	replicationTimeout = 2 * time.Second
)

type incomingMsg struct {
	client  *Client
	payload []byte
}

type statusRequest struct {
	resp chan hubSnapshot
}

// Hub is an actor goroutine that serialises all state mutations for one document.
// It is the only writer of content, version and ops — no mutex needed for these fields.
type Hub struct {
	docID      string
	clients    map[*Client]bool
	content    string
	version    int
	ops        []Op // ops[i] was applied to produce version i+1; needed for OT transforms
	register   chan *Client
	unregister chan *Client
	incoming   chan incomingMsg
	pub        *mq.Publisher

	bus               replication.Bus
	repMessages       <-chan replication.Message
	closeReplication  func() error
	cancelReplication context.CancelFunc
	isLeader          bool
	status            chan statusRequest
}

func newHub(docID, content string, version int, pub *mq.Publisher, bus replication.Bus) *Hub {
	h := &Hub{
		docID:      docID,
		clients:    make(map[*Client]bool),
		content:    content,
		version:    version,
		ops:        make([]Op, 0, 64),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		incoming:   make(chan incomingMsg, 256),
		pub:        pub,
		bus:        bus,
		status:     make(chan statusRequest, 4),
	}

	if bus != nil {
		ctx, cancel := context.WithCancel(context.Background())
		messages, closeReplication, err := bus.Subscribe(ctx, docID)
		if err != nil {
			cancel()
			log.Printf("hub[%s]: redis replication disabled: %v", docID, err)
		} else {
			h.repMessages = messages
			h.closeReplication = closeReplication
			h.cancelReplication = cancel
		}
	}

	return h
}

func (h *Hub) run() {
	ticker := time.NewTicker(leadershipRenewal)
	defer ticker.Stop()

	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
			h.sendTo(c, ServerMessage{
				Type:          "resync",
				ServerVersion: h.version,
				Content:       h.content,
			})
			h.broadcastPresence()

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.broadcastPresence()
			}

		case m := <-h.incoming:
			h.dispatch(m)

		case msg, ok := <-h.repMessages:
			if ok {
				h.handleReplication(msg)
			}

		case <-ticker.C:
			h.ensureLeadership()

		case req := <-h.status:
			req.resp <- h.localSnapshot()
		}
	}
}

func (h *Hub) dispatch(m incomingMsg) {
	var env ClientMessage
	if err := json.Unmarshal(m.payload, &env); err != nil {
		return
	}
	switch env.Type {
	case "op":
		h.handleOp(m.client, env)
	case "cursor":
		h.handleCursor(m.client, env)
	}
}

func (h *Hub) handleOp(c *Client, env ClientMessage) {
	if env.Op == nil {
		return
	}
	if env.ClientVersion < 0 || env.ClientVersion > h.version {
		h.sendTo(c, ServerMessage{Type: "resync", ServerVersion: h.version, Content: h.content})
		return
	}

	if h.bus == nil || h.repMessages == nil {
		h.commitLocal(c, env.Op, env.ClientVersion)
		return
	}

	// Any node can receive a WebSocket operation. The operation is published as
	// a proposal and only the Redis leader for this document will order it.
	ctx, cancel := context.WithTimeout(context.Background(), replicationTimeout)
	defer cancel()

	if err := h.bus.PublishProposal(ctx, replication.Proposal{
		DocID:          h.docID,
		UserID:         c.userID,
		OriginNodeID:   h.bus.NodeID(),
		OriginClientID: c.id,
		ClientVersion:  env.ClientVersion,
		Op:             toReplicationOp(env.Op),
	}); err != nil {
		log.Printf("hub[%s]: publish proposal failed: %v", h.docID, err)
	}
}

func (h *Hub) handleReplication(msg replication.Message) {
	switch msg.Kind {
	case replication.MessageKindProposal:
		if msg.Proposal != nil {
			h.handleProposal(*msg.Proposal)
		}
	case replication.MessageKindCommit:
		if msg.Commit != nil {
			h.handleCommit(*msg.Commit)
		}
	case replication.MessageKindResyncRequest:
		if msg.ResyncRequest != nil {
			h.handleResyncRequest(*msg.ResyncRequest)
		}
	case replication.MessageKindResyncResponse:
		if msg.ResyncResponse != nil {
			h.handleResyncResponse(*msg.ResyncResponse)
		}
	}
}

func (h *Hub) handleProposal(proposal replication.Proposal) {
	if !h.isLeader {
		return
	}

	op := transformSince(fromReplicationOp(proposal.Op), h.ops, proposal.ClientVersion)
	if op == nil {
		return
	}
	h.content = apply(h.content, op)
	h.version++
	h.ops = append(h.ops, *op)

	commit := replication.Commit{
		DocID:          h.docID,
		OriginNodeID:   proposal.OriginNodeID,
		OriginClientID: proposal.OriginClientID,
		UserID:         proposal.UserID,
		ServerVersion:  h.version,
		Op:             toReplicationOp(op),
	}

	h.publishDurableOp(commit)
	h.broadcastCommit(commit)

	ctx, cancel := context.WithTimeout(context.Background(), replicationTimeout)
	defer cancel()
	if err := h.bus.PublishCommit(ctx, commit); err != nil {
		log.Printf("hub[%s]: publish commit failed: %v", h.docID, err)
	}
}

func (h *Hub) handleCommit(commit replication.Commit) {
	if h.bus != nil && commit.OriginNodeID == h.bus.NodeID() {
		return
	}
	if commit.ServerVersion <= h.version {
		return
	}
	if commit.ServerVersion > h.version+1 {
		// One or more commits were dropped by Redis Pub/Sub. Request the leader
		// to publish the full document state so we can recover.
		log.Printf("hub[%s]: version gap detected (local=%d incoming=%d), requesting resync",
			h.docID, h.version, commit.ServerVersion)
		h.requestResync()
		return
	}

	op := fromReplicationOp(commit.Op)
	h.content = apply(h.content, op)
	h.version = commit.ServerVersion
	h.ops = append(h.ops, *op)
	h.broadcastCommit(commit)
}

func (h *Hub) requestResync() {
	ctx, cancel := context.WithTimeout(context.Background(), replicationTimeout)
	defer cancel()
	if err := h.bus.PublishResyncRequest(ctx, replication.ResyncRequest{
		DocID:        h.docID,
		FromNodeID:   h.bus.NodeID(),
		KnownVersion: h.version,
	}); err != nil {
		log.Printf("hub[%s]: publish resync request failed: %v", h.docID, err)
	}
}

func (h *Hub) handleResyncRequest(req replication.ResyncRequest) {
	if !h.isLeader {
		return
	}
	log.Printf("hub[%s]: resync requested by %s (known version %d), publishing state at version %d",
		h.docID, req.FromNodeID, req.KnownVersion, h.version)
	ctx, cancel := context.WithTimeout(context.Background(), replicationTimeout)
	defer cancel()
	if err := h.bus.PublishResyncResponse(ctx, replication.ResyncResponse{
		DocID:   h.docID,
		Content: h.content,
		Version: h.version,
	}); err != nil {
		log.Printf("hub[%s]: publish resync response failed: %v", h.docID, err)
	}
}

func (h *Hub) handleResyncResponse(resp replication.ResyncResponse) {
	if resp.Version <= h.version {
		return
	}
	log.Printf("hub[%s]: applying resync (local version %d → %d)", h.docID, h.version, resp.Version)
	h.content = resp.Content
	h.version = resp.Version
	h.broadcast(ServerMessage{
		Type:          "resync",
		ServerVersion: h.version,
		Content:       h.content,
	})
}

func (h *Hub) commitLocal(origin *Client, rawOp *Op, clientVersion int) {
	op := transformSince(rawOp, h.ops, clientVersion)
	if op != nil {
		h.content = apply(h.content, op)
		h.version++
		h.ops = append(h.ops, *op)

		h.broadcastExcept(origin, ServerMessage{
			Type:          "op",
			ServerVersion: h.version,
			UserID:        origin.userID,
			Op:            op,
		})

		h.publishDurableOp(replication.Commit{
			DocID:         h.docID,
			UserID:        origin.userID,
			ServerVersion: h.version,
			Op:            toReplicationOp(op),
		})
	}

	h.sendTo(origin, ServerMessage{Type: "ack", ServerVersion: h.version})
}

func (h *Hub) publishDurableOp(commit replication.Commit) {
	if h.pub == nil {
		return
	}
	eventType := "INSERT"
	if commit.Op.Type == "delete" {
		eventType = "DELETE"
	}
	h.pub.PublishDocEvent(mq.NewDocEvent(
		commit.DocID,
		commit.UserID,
		commit.ServerVersion,
		eventType,
		commit.Op.Pos,
		commit.Op.Char,
		h.content,
	))
}

func (h *Hub) broadcastCommit(commit replication.Commit) {
	opRaw, _ := json.Marshal(ServerMessage{
		Type:          "op",
		ServerVersion: commit.ServerVersion,
		UserID:        commit.UserID,
		Op:            fromReplicationOp(commit.Op),
	})
	ackRaw, _ := json.Marshal(ServerMessage{Type: "ack", ServerVersion: commit.ServerVersion})

	for c := range h.clients {
		if h.bus != nil && commit.OriginNodeID == h.bus.NodeID() && c.id == commit.OriginClientID {
			h.sendRaw(c, ackRaw)
			continue
		}
		h.sendRaw(c, opRaw)
	}
}

func (h *Hub) ensureLeadership() {
	if h.bus == nil || h.repMessages == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), replicationTimeout)
	defer cancel()

	var (
		ok  bool
		err error
	)
	if h.isLeader {
		ok, err = h.bus.RenewLeadership(ctx, h.docID, leadershipTTL)
	} else {
		ok, err = h.bus.TryAcquireLeadership(ctx, h.docID, leadershipTTL)
	}
	if err != nil {
		log.Printf("hub[%s]: leadership error: %v", h.docID, err)
		return
	}
	if h.isLeader && !ok {
		log.Printf("hub[%s]: node %s lost leadership", h.docID, h.bus.NodeID())
	}
	if ok && !h.isLeader {
		log.Printf("hub[%s]: node %s became leader (failover)", h.docID, h.bus.NodeID())
	}
	h.isLeader = ok
}

// snapshot returns hub state via the actor goroutine to avoid data races.
func (h *Hub) snapshot() hubSnapshot {
	req := statusRequest{resp: make(chan hubSnapshot, 1)}
	select {
	case h.status <- req:
		return <-req.resp
	case <-time.After(replicationTimeout):
		return hubSnapshot{}
	}
}

func (h *Hub) localSnapshot() hubSnapshot {
	return hubSnapshot{
		isLeader: h.isLeader,
		version:  h.version,
		clients:  len(h.clients),
		content:  h.content,
	}
}

type hubSnapshot struct {
	isLeader bool
	version  int
	clients  int
	content  string
}

func (h *Hub) handleCursor(c *Client, env ClientMessage) {
	h.broadcastExcept(c, ServerMessage{
		Type:   "cursor",
		UserID: c.userID,
		Name:   c.name,
		Pos:    env.Pos,
	})
}

func (h *Hub) broadcastPresence() {
	users := make([]PresenceUser, 0, len(h.clients))
	for c := range h.clients {
		users = append(users, PresenceUser{ID: c.userID, Name: c.name})
	}
	h.broadcast(ServerMessage{Type: "presence", Users: users})
}

func (h *Hub) broadcast(msg ServerMessage) {
	raw, _ := json.Marshal(msg)
	for c := range h.clients {
		h.sendRaw(c, raw)
	}
}

func (h *Hub) broadcastExcept(except *Client, msg ServerMessage) {
	raw, _ := json.Marshal(msg)
	for c := range h.clients {
		if c != except {
			h.sendRaw(c, raw)
		}
	}
}

func (h *Hub) sendTo(c *Client, msg ServerMessage) {
	raw, _ := json.Marshal(msg)
	h.sendRaw(c, raw)
}

func (h *Hub) sendRaw(c *Client, raw []byte) {
	select {
	case c.send <- raw:
	default:
		log.Printf("hub[%s]: client %s send buffer full, dropping message", h.docID, c.userID)
	}
}
