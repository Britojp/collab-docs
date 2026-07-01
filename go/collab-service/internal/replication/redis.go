package replication

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	leaderKeyPrefix = "collabdocs:doc:"
)

var renewLeadershipScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// RedisBus replicates real-time document operations across Go instances.
//
// Two Pub/Sub channels are used per document:
//   - proposals: client operations submitted by any node
//   - commits: operations ordered by the document leader
//
// The leader is selected with SETNX + TTL. Leadership is intentionally
// short-lived and renewed by the Hub, so a crashed Go instance releases the
// document after the TTL expires without manual cleanup.
type RedisBus struct {
	client *redis.Client
	nodeID string
}

// NewRedisBus creates a Redis-backed replication bus.
func NewRedisBus(redisURL string) (*RedisBus, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	bus := &RedisBus{
		client: redis.NewClient(opts),
		nodeID: newNodeID(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.client.Ping(ctx).Err(); err != nil {
		bus.client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return bus, nil
}

// NodeID returns this Go instance identifier. It is included in replicated
// messages so origin clients can be skipped when their committed operation is
// broadcast back to a node.
func (b *RedisBus) NodeID() string {
	return b.nodeID
}

// Subscribe listens to proposal and commit channels for a single document.
// The returned close function must be called when the Hub is shut down.
func (b *RedisBus) Subscribe(ctx context.Context, docID string) (<-chan Message, func() error, error) {
	pubsub := b.client.Subscribe(ctx, proposalChannel(docID), commitChannel(docID), cursorChannel(docID), presenceChannel(docID))
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return nil, nil, fmt.Errorf("subscribe doc %s: %w", docID, err)
	}

	out := make(chan Message, 128)
	go func() {
		defer close(out)
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				decoded, err := decodeMessage(msg.Payload)
				if err != nil {
					continue
				}
				out <- decoded
			}
		}
	}()

	return out, pubsub.Close, nil
}

// PublishProposal submits a client operation for leader ordering.
func (b *RedisBus) PublishProposal(ctx context.Context, proposal Proposal) error {
	return b.publish(ctx, proposalChannel(proposal.DocID), Message{
		Kind:     MessageKindProposal,
		Proposal: &proposal,
	})
}

// PublishCommit fan-outs a leader-confirmed operation to every Go instance.
func (b *RedisBus) PublishCommit(ctx context.Context, commit Commit) error {
	return b.publish(ctx, commitChannel(commit.DocID), Message{
		Kind:   MessageKindCommit,
		Commit: &commit,
	})
}

// PublishResyncRequest asks the leader to resend the full document state.
// The message travels via the proposals channel so only the leader processes it.
func (b *RedisBus) PublishResyncRequest(ctx context.Context, req ResyncRequest) error {
	return b.publish(ctx, proposalChannel(req.DocID), Message{
		Kind:          MessageKindResyncRequest,
		ResyncRequest: &req,
	})
}

// PublishResyncResponse broadcasts the authoritative document state to all nodes.
// The message travels via the commits channel so every subscribed Hub receives it.
func (b *RedisBus) PublishResyncResponse(ctx context.Context, resp ResyncResponse) error {
	return b.publish(ctx, commitChannel(resp.DocID), Message{
		Kind:           MessageKindResyncResponse,
		ResyncResponse: &resp,
	})
}

// PublishCursor fans out a caret position update to every other node.
func (b *RedisBus) PublishCursor(ctx context.Context, cursor CursorUpdate) error {
	return b.publish(ctx, cursorChannel(cursor.DocID), Message{
		Kind:   MessageKindCursor,
		Cursor: &cursor,
	})
}

// PublishPresence fans out this node's local roster for a document.
func (b *RedisBus) PublishPresence(ctx context.Context, snapshot PresenceSnapshot) error {
	return b.publish(ctx, presenceChannel(snapshot.DocID), Message{
		Kind:     MessageKindPresence,
		Presence: &snapshot,
	})
}

// TryAcquireLeadership attempts to become the single writer for a document.
func (b *RedisBus) TryAcquireLeadership(ctx context.Context, docID string, ttl time.Duration) (bool, error) {
	ok, err := b.client.SetNX(ctx, leaderKey(docID), b.nodeID, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("acquire leadership: %w", err)
	}
	return ok, nil
}

// RenewLeadership extends the lock only if this node still owns it.
// GetLeader reads the current leader node ID stored in Redis for a document.
func (b *RedisBus) GetLeader(ctx context.Context, docID string) (string, error) {
	value, err := b.client.Get(ctx, leaderKey(docID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get leader: %w", err)
	}
	return value, nil
}

func (b *RedisBus) RenewLeadership(ctx context.Context, docID string, ttl time.Duration) (bool, error) {
	result, err := renewLeadershipScript.Run(
		ctx,
		b.client,
		[]string{leaderKey(docID)},
		b.nodeID,
		ttl.Milliseconds(),
	).Int()
	if err != nil {
		return false, fmt.Errorf("renew leadership: %w", err)
	}
	return result == 1, nil
}

func (b *RedisBus) Close() error {
	return b.client.Close()
}

func (b *RedisBus) publish(ctx context.Context, channel string, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal replication message: %w", err)
	}
	if err := b.client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("publish %s: %w", channel, err)
	}
	return nil
}

func decodeMessage(payload string) (Message, error) {
	var msg Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func proposalChannel(docID string) string {
	return leaderKeyPrefix + docID + ":proposals"
}

func commitChannel(docID string) string {
	return leaderKeyPrefix + docID + ":commits"
}

func cursorChannel(docID string) string {
	return leaderKeyPrefix + docID + ":cursors"
}

func presenceChannel(docID string) string {
	return leaderKeyPrefix + docID + ":presence"
}

func leaderKey(docID string) string {
	return leaderKeyPrefix + docID + ":leader"
}

func newNodeID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	return "node-" + hex.EncodeToString(raw[:])
}
