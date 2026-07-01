package replication

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisBusPublishesProposalToDocumentSubscribers(t *testing.T) {
	server := miniredis.RunT(t)

	publisher := newTestRedisBus(t, server)
	defer publisher.Close()
	subscriber := newTestRedisBus(t, server)
	defer subscriber.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messages, closeSubscription, err := subscriber.Subscribe(ctx, "doc-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer closeSubscription()

	proposal := Proposal{
		DocID:          "doc-1",
		OriginNodeID:   publisher.NodeID(),
		OriginClientID: "client-1",
		UserID:         "user-1",
		ClientVersion:  7,
		Op:             Operation{Type: "insert", Pos: 2, Char: "x"},
	}

	if err := publisher.PublishProposal(ctx, proposal); err != nil {
		t.Fatalf("publish proposal: %v", err)
	}

	msg := receiveReplicationMessage(t, messages)
	if msg.Kind != MessageKindProposal {
		t.Fatalf("kind = %q, want %q", msg.Kind, MessageKindProposal)
	}
	if msg.Proposal == nil {
		t.Fatal("proposal is nil")
	}
	if got := *msg.Proposal; got != proposal {
		t.Fatalf("proposal = %#v, want %#v", got, proposal)
	}
}

func TestRedisBusPublishesCursorToDocumentSubscribers(t *testing.T) {
	server := miniredis.RunT(t)

	publisher := newTestRedisBus(t, server)
	defer publisher.Close()
	subscriber := newTestRedisBus(t, server)
	defer subscriber.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messages, closeSubscription, err := subscriber.Subscribe(ctx, "doc-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer closeSubscription()

	cursor := CursorUpdate{
		DocID:          "doc-1",
		OriginNodeID:   publisher.NodeID(),
		OriginClientID: "client-1",
		UserID:         "user-1",
		Name:           "Ana",
		Pos:            12,
	}

	if err := publisher.PublishCursor(ctx, cursor); err != nil {
		t.Fatalf("publish cursor: %v", err)
	}

	msg := receiveReplicationMessage(t, messages)
	if msg.Kind != MessageKindCursor {
		t.Fatalf("kind = %q, want %q", msg.Kind, MessageKindCursor)
	}
	if msg.Cursor == nil || *msg.Cursor != cursor {
		t.Fatalf("cursor = %#v, want %#v", msg.Cursor, cursor)
	}
}

func TestRedisBusPublishesPresenceToDocumentSubscribers(t *testing.T) {
	server := miniredis.RunT(t)

	publisher := newTestRedisBus(t, server)
	defer publisher.Close()
	subscriber := newTestRedisBus(t, server)
	defer subscriber.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messages, closeSubscription, err := subscriber.Subscribe(ctx, "doc-1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer closeSubscription()

	snapshot := PresenceSnapshot{
		DocID:        "doc-1",
		OriginNodeID: publisher.NodeID(),
		Users:        []PresenceUser{{ID: "user-1", Name: "Ana"}},
	}

	if err := publisher.PublishPresence(ctx, snapshot); err != nil {
		t.Fatalf("publish presence: %v", err)
	}

	msg := receiveReplicationMessage(t, messages)
	if msg.Kind != MessageKindPresence {
		t.Fatalf("kind = %q, want %q", msg.Kind, MessageKindPresence)
	}
	if msg.Presence == nil || msg.Presence.DocID != snapshot.DocID || msg.Presence.OriginNodeID != snapshot.OriginNodeID {
		t.Fatalf("presence = %#v, want %#v", msg.Presence, snapshot)
	}
	if len(msg.Presence.Users) != 1 || msg.Presence.Users[0] != snapshot.Users[0] {
		t.Fatalf("presence users = %#v, want %#v", msg.Presence.Users, snapshot.Users)
	}
}

func TestRedisBusLeadershipLockIsExclusiveAndRenewable(t *testing.T) {
	server := miniredis.RunT(t)

	leader := newTestRedisBus(t, server)
	defer leader.Close()
	follower := newTestRedisBus(t, server)
	defer follower.Close()

	ctx := context.Background()
	ok, epoch, err := leader.TryAcquireLeadership(ctx, "doc-1", time.Minute)
	if err != nil {
		t.Fatalf("leader acquire: %v", err)
	}
	if !ok {
		t.Fatal("leader did not acquire lock")
	}
	if epoch != 1 {
		t.Fatalf("epoch = %d, want 1 for first acquisition", epoch)
	}

	okFollower, _, err := follower.TryAcquireLeadership(ctx, "doc-1", time.Minute)
	if err != nil {
		t.Fatalf("follower acquire: %v", err)
	}
	if okFollower {
		t.Fatal("follower unexpectedly acquired lock")
	}

	ok, err = leader.RenewLeadership(ctx, "doc-1", time.Minute)
	if err != nil {
		t.Fatalf("leader renew: %v", err)
	}
	if !ok {
		t.Fatal("leader did not renew lock")
	}
}

func TestRedisBusEpochIncrementsAcrossLeadershipHandoffsAndSurvivesExpiry(t *testing.T) {
	server := miniredis.RunT(t)

	nodeA := newTestRedisBus(t, server)
	defer nodeA.Close()
	nodeB := newTestRedisBus(t, server)
	defer nodeB.Close()

	ctx := context.Background()

	ok, epoch, err := nodeA.TryAcquireLeadership(ctx, "doc-1", time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("node A acquire: ok=%v err=%v", ok, err)
	}
	if epoch != 1 {
		t.Fatalf("epoch = %d, want 1", epoch)
	}

	// Simulate node A being paused/partitioned past the lease TTL: its key
	// expires and node B takes over the same document.
	server.FastForward(10 * time.Millisecond)

	ok, epoch, err = nodeB.TryAcquireLeadership(ctx, "doc-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("node B acquire after expiry: ok=%v err=%v", ok, err)
	}
	if epoch != 2 {
		t.Fatalf("epoch = %d, want 2 after handoff — must never repeat a past epoch", epoch)
	}

	current, err := nodeA.CurrentEpoch(ctx, "doc-1")
	if err != nil {
		t.Fatalf("current epoch: %v", err)
	}
	if current != 2 {
		t.Fatalf("current epoch = %d, want 2 — node A must observe it has been superseded", current)
	}
}

func TestRedisBusGetLeaderReturnsEmptyWhenUnset(t *testing.T) {
	server := miniredis.RunT(t)
	bus := newTestRedisBus(t, server)
	defer bus.Close()

	leader, err := bus.GetLeader(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("get leader: %v", err)
	}
	if leader != "" {
		t.Fatalf("leader = %q, want empty", leader)
	}

	acquired, _, err := bus.TryAcquireLeadership(context.Background(), "doc-1", time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire leadership: ok=%v err=%v", acquired, err)
	}

	leader, err = bus.GetLeader(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("get leader after acquire: %v", err)
	}
	if leader != bus.NodeID() {
		t.Fatalf("leader = %q, want %q", leader, bus.NodeID())
	}
}

func newTestRedisBus(t *testing.T, server *miniredis.Miniredis) *RedisBus {
	t.Helper()
	bus, err := NewRedisBus("redis://" + server.Addr() + "/0")
	if err != nil {
		t.Fatalf("new redis bus: %v", err)
	}
	return bus
}

func receiveReplicationMessage(t *testing.T, messages <-chan Message) Message {
	t.Helper()
	select {
	case msg := <-messages:
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replication message")
		return Message{}
	}
}
