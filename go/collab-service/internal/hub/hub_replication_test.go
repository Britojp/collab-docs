package hub

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/britojp/collabdocs/go/collab-service/internal/replication"
)

func TestLeaderProposalCommitsOperationAndSkipsOriginClient(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "", 0, nil, bus)
	h.isLeader = true

	origin := newTestClient("client-1", "user-1")
	peer := newTestClient("client-2", "user-2")
	h.clients[origin] = true
	h.clients[peer] = true

	h.handleProposal(replication.Proposal{
		DocID:          "doc-1",
		OriginNodeID:   bus.NodeID(),
		OriginClientID: origin.id,
		UserID:         origin.userID,
		ClientVersion:  0,
		Op:             replication.Operation{Type: "insert", Pos: 0, Char: "a"},
	})

	if h.content != "a" {
		t.Fatalf("content = %q, want %q", h.content, "a")
	}
	if h.version != 1 {
		t.Fatalf("version = %d, want 1", h.version)
	}

	if len(bus.commits) != 1 {
		t.Fatalf("commits = %d, want 1", len(bus.commits))
	}
	if bus.commits[0].ServerVersion != 1 {
		t.Fatalf("commit version = %d, want 1", bus.commits[0].ServerVersion)
	}

	// Origin client receives ack, not op.
	ack := receiveServerMessage(t, origin.send)
	if ack.Type != "ack" || ack.ServerVersion != 1 {
		t.Fatalf("expected ack{serverVersion:1} for origin, got %#v", ack)
	}

	msg := receiveServerMessage(t, peer.send)
	if msg.Type != "op" || msg.ServerVersion != 1 || msg.Op == nil || msg.Op.Char != "a" {
		t.Fatalf("unexpected peer message: %#v", msg)
	}
}

func TestFollowerAppliesCommitFromAnotherNode(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "", 0, nil, bus)

	peer := newTestClient("client-2", "user-2")
	h.clients[peer] = true

	// version 0 → commit version 1: sequential, no gap.
	h.handleCommit(replication.Commit{
		DocID:          "doc-1",
		OriginNodeID:   "node-a",
		OriginClientID: "client-1",
		UserID:         "user-1",
		ServerVersion:  1,
		Op:             replication.Operation{Type: "insert", Pos: 0, Char: "b"},
	})

	if h.content != "b" {
		t.Fatalf("content = %q, want %q", h.content, "b")
	}
	if h.version != 1 {
		t.Fatalf("version = %d, want 1", h.version)
	}

	msg := receiveServerMessage(t, peer.send)
	if msg.Type != "op" || msg.ServerVersion != 1 || msg.Op == nil || msg.Op.Char != "b" {
		t.Fatalf("unexpected peer message: %#v", msg)
	}
}

func TestFollowerDetectsVersionGapAndRequestsResync(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "hello", 0, nil, bus)
	h.version = 2

	peer := newTestClient("client-2", "user-2")
	h.clients[peer] = true

	// version 2 → incoming version 5: gap of 2 commits.
	h.handleCommit(replication.Commit{
		DocID:         "doc-1",
		OriginNodeID:  "node-a",
		ServerVersion: 5,
		Op:            replication.Operation{Type: "insert", Pos: 5, Char: "x"},
	})

	// State must remain unchanged — the commit was discarded.
	if h.content != "hello" {
		t.Fatalf("content changed to %q, want unchanged %q", h.content, "hello")
	}
	if h.version != 2 {
		t.Fatalf("version = %d, want unchanged 2", h.version)
	}

	// A resync request must have been published.
	bus.mu.Lock()
	reqs := bus.resyncRequests
	bus.mu.Unlock()
	if len(reqs) != 1 {
		t.Fatalf("resync requests = %d, want 1", len(reqs))
	}
	if reqs[0].KnownVersion != 2 {
		t.Fatalf("resync request known version = %d, want 2", reqs[0].KnownVersion)
	}

	// No broadcast to local clients should have happened.
	assertNoMessage(t, peer.send)
}

func TestLeaderHandlesResyncRequestAndPublishesResponse(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "world", 0, nil, bus)
	h.version = 7
	h.isLeader = true

	h.handleResyncRequest(replication.ResyncRequest{
		DocID:        "doc-1",
		FromNodeID:   "node-b",
		KnownVersion: 3,
	})

	bus.mu.Lock()
	resps := bus.resyncResponses
	bus.mu.Unlock()
	if len(resps) != 1 {
		t.Fatalf("resync responses = %d, want 1", len(resps))
	}
	if resps[0].Content != "world" || resps[0].Version != 7 {
		t.Fatalf("resync response = %+v, want content=world version=7", resps[0])
	}
}

func TestFollowerAppliesResyncResponseAndBroadcastsToClients(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "stale", 0, nil, bus)
	h.version = 2

	peer := newTestClient("client-2", "user-2")
	h.clients[peer] = true

	h.handleResyncResponse(replication.ResyncResponse{
		DocID:   "doc-1",
		Content: "authoritative",
		Version: 7,
	})

	if h.content != "authoritative" {
		t.Fatalf("content = %q, want %q", h.content, "authoritative")
	}
	if h.version != 7 {
		t.Fatalf("version = %d, want 7", h.version)
	}

	msg := receiveServerMessage(t, peer.send)
	if msg.Type != "resync" || msg.ServerVersion != 7 || msg.Content != "authoritative" {
		t.Fatalf("unexpected resync message: %#v", msg)
	}
}

func TestLeaderIgnoresOwnRedisCommitAfterLocalBroadcast(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "a", 0, nil, bus)
	h.version = 1
	h.isLeader = true

	h.handleCommit(replication.Commit{
		DocID:          "doc-1",
		OriginNodeID:   bus.NodeID(),
		OriginClientID: "client-1",
		UserID:         "user-1",
		ServerVersion:  2,
		Op:             replication.Operation{Type: "insert", Pos: 1, Char: "z"},
	})

	if h.content != "a" {
		t.Fatalf("content = %q, want unchanged %q", h.content, "a")
	}
	if h.version != 1 {
		t.Fatalf("version = %d, want unchanged 1", h.version)
	}
}

// Regression test: a client connected to a non-leader node must still have
// its own commit applied and acked when it comes back from Redis. Origin
// node and leader node are frequently different — matching OriginNodeID
// against this node's own ID (instead of checking h.isLeader) caused the
// origin node to silently drop its own client's commits forever, which
// stalls that client's pending-op queue on the frontend.
func TestOriginNonLeaderAppliesAndAcksOwnCommitFromLeader(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "a", 0, nil, bus)
	h.version = 1
	h.isLeader = false

	origin := newTestClient("client-1", "user-1")
	h.clients[origin] = true

	h.handleCommit(replication.Commit{
		DocID:          "doc-1",
		OriginNodeID:   bus.NodeID(), // this node originated the proposal, but is not the leader
		OriginClientID: origin.id,
		UserID:         origin.userID,
		ServerVersion:  2,
		Op:             replication.Operation{Type: "insert", Pos: 1, Char: "z"},
	})

	if h.content != "az" {
		t.Fatalf("content = %q, want %q", h.content, "az")
	}
	if h.version != 2 {
		t.Fatalf("version = %d, want 2", h.version)
	}

	msg := receiveServerMessage(t, origin.send)
	if msg.Type != "ack" || msg.ServerVersion != 2 {
		t.Fatalf("expected ack{serverVersion:2} for origin client, got %#v", msg)
	}
}

// Cursor and presence updates are node-local by default (plain WebSocket
// broadcast). These tests cover the Redis fan-out added so users connected
// to different go-collab nodes still see each other's cursor and presence.

func TestHandleCursorBroadcastsLocallyAndPublishesToBus(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "", 0, nil, bus)

	origin := newTestClient("client-1", "user-1")
	peer := newTestClient("client-2", "user-2")
	h.clients[origin] = true
	h.clients[peer] = true

	h.handleCursor(origin, ClientMessage{Pos: 5})

	// Origin does not receive its own cursor position back.
	assertNoMessage(t, origin.send)

	msg := receiveServerMessage(t, peer.send)
	if msg.Type != "cursor" || msg.UserID != "user-1" || msg.Pos != 5 {
		t.Fatalf("unexpected peer message: %#v", msg)
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.cursors) != 1 {
		t.Fatalf("published cursors = %d, want 1", len(bus.cursors))
	}
	if bus.cursors[0].UserID != "user-1" || bus.cursors[0].Pos != 5 || bus.cursors[0].OriginNodeID != "node-a" {
		t.Fatalf("published cursor = %#v", bus.cursors[0])
	}
}

func TestCursorReplicationFromAnotherNodeIsBroadcastLocally(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "", 0, nil, bus)

	peer := newTestClient("client-2", "user-2")
	h.clients[peer] = true

	h.handleCursorReplication(replication.CursorUpdate{
		DocID:        "doc-1",
		OriginNodeID: "node-a", // a different node than this one
		UserID:       "user-1",
		Name:         "user-1",
		Pos:          9,
	})

	msg := receiveServerMessage(t, peer.send)
	if msg.Type != "cursor" || msg.UserID != "user-1" || msg.Pos != 9 {
		t.Fatalf("unexpected peer message: %#v", msg)
	}
}

func TestCursorReplicationEchoFromOwnNodeIsIgnored(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "", 0, nil, bus)

	peer := newTestClient("client-2", "user-2")
	h.clients[peer] = true

	// This node already broadcast the cursor locally in handleCursor;
	// receiving its own publish echoed back via Redis must be a no-op.
	h.handleCursorReplication(replication.CursorUpdate{
		DocID:        "doc-1",
		OriginNodeID: "node-a",
		UserID:       "user-1",
		Pos:          9,
	})

	assertNoMessage(t, peer.send)
}

func TestPresenceSnapshotFromAnotherNodeMergesIntoBroadcast(t *testing.T) {
	bus := newFakeBus("node-b")
	h := newHub("doc-1", "", 0, nil, bus)

	local := newTestClient("client-2", "user-2")
	h.clients[local] = true

	h.handlePresenceSnapshot(replication.PresenceSnapshot{
		DocID:        "doc-1",
		OriginNodeID: "node-a",
		Users:        []replication.PresenceUser{{ID: "user-1", Name: "user-1"}},
	})

	msg := receiveServerMessage(t, local.send)
	if msg.Type != "presence" {
		t.Fatalf("unexpected message type: %#v", msg)
	}
	ids := make(map[string]bool)
	for _, u := range msg.Users {
		ids[u.ID] = true
	}
	if !ids["user-1"] || !ids["user-2"] {
		t.Fatalf("expected merged roster with user-1 and user-2, got %#v", msg.Users)
	}
}

func TestBroadcastPresencePublishesLocalRosterToBus(t *testing.T) {
	bus := newFakeBus("node-a")
	h := newHub("doc-1", "", 0, nil, bus)

	local := newTestClient("client-1", "user-1")
	h.clients[local] = true

	h.broadcastPresence()

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.presenceSnaps) != 1 {
		t.Fatalf("published presence snapshots = %d, want 1", len(bus.presenceSnaps))
	}
	got := bus.presenceSnaps[0]
	if got.OriginNodeID != "node-a" || len(got.Users) != 1 || got.Users[0].ID != "user-1" {
		t.Fatalf("published snapshot = %#v", got)
	}
}

func newTestClient(id, userID string) *Client {
	return &Client{
		id:     id,
		userID: userID,
		name:   userID,
		send:   make(chan []byte, 8),
	}
}

func receiveServerMessage(t *testing.T, messages <-chan []byte) ServerMessage {
	t.Helper()
	select {
	case raw := <-messages:
		var msg ServerMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal server message: %v", err)
		}
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server message")
		return ServerMessage{}
	}
}

func assertNoMessage(t *testing.T, messages <-chan []byte) {
	t.Helper()
	select {
	case raw := <-messages:
		t.Fatalf("unexpected message: %s", raw)
	default:
	}
}

type fakeBus struct {
	nodeID          string
	mu              sync.Mutex
	commits         []replication.Commit
	proposals       []replication.Proposal
	resyncRequests  []replication.ResyncRequest
	resyncResponses []replication.ResyncResponse
	cursors         []replication.CursorUpdate
	presenceSnaps   []replication.PresenceSnapshot
}

func newFakeBus(nodeID string) *fakeBus {
	return &fakeBus{nodeID: nodeID}
}

func (b *fakeBus) NodeID() string {
	return b.nodeID
}

func (b *fakeBus) Subscribe(context.Context, string) (<-chan replication.Message, func() error, error) {
	ch := make(chan replication.Message)
	return ch, func() error { close(ch); return nil }, nil
}

func (b *fakeBus) PublishProposal(_ context.Context, proposal replication.Proposal) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.proposals = append(b.proposals, proposal)
	return nil
}

func (b *fakeBus) PublishCommit(_ context.Context, commit replication.Commit) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commits = append(b.commits, commit)
	return nil
}

func (b *fakeBus) PublishResyncRequest(_ context.Context, req replication.ResyncRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resyncRequests = append(b.resyncRequests, req)
	return nil
}

func (b *fakeBus) PublishResyncResponse(_ context.Context, resp replication.ResyncResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resyncResponses = append(b.resyncResponses, resp)
	return nil
}

func (b *fakeBus) PublishCursor(_ context.Context, cursor replication.CursorUpdate) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursors = append(b.cursors, cursor)
	return nil
}

func (b *fakeBus) PublishPresence(_ context.Context, snapshot replication.PresenceSnapshot) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.presenceSnaps = append(b.presenceSnaps, snapshot)
	return nil
}

func (b *fakeBus) TryAcquireLeadership(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (b *fakeBus) RenewLeadership(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (b *fakeBus) GetLeader(context.Context, string) (string, error) {
	return b.nodeID, nil
}

func (b *fakeBus) Close() error {
	return nil
}
