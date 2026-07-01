package mq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const exchange = "collab"

// DocEvent is published to RabbitMQ for each confirmed document operation.
// Events are ordered by Version and are self-sufficient: replaying them in
// order (applying each Op to the previous Content) reproduces the exact
// document state at any point in time.
//
// Type values:
//   - "INSERT" — one character was inserted at Pos
//   - "DELETE" — one character was deleted at Pos
//
// JSON field names are intentionally kept compatible with the Java OpEvent record.
type DocEvent struct {
	EventID   string    `json:"eventId"` // UUID v4 for idempotency
	DocID     string    `json:"docId"`
	UserID    string    `json:"userId"`
	Version   int       `json:"version"` // monotonically increasing per doc
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "INSERT" | "DELETE"
	Pos       int       `json:"pos"`
	Character string    `json:"character,omitempty"` // present only for INSERT
	Content   string    `json:"content"`             // full document content after this op
}

// NewDocEvent builds a DocEvent with a generated EventID and current timestamp.
func NewDocEvent(docID, userID string, version int, eventType string, pos int, char, content string) DocEvent {
	return DocEvent{
		EventID:   newEventID(),
		DocID:     docID,
		UserID:    userID,
		Version:   version,
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Pos:       pos,
		Character: char,
		Content:   content,
	}
}

// Publisher wraps an AMQP channel for publishing collab events.
type Publisher struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(amqpURL string) (*Publisher, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("channel: %w", err)
	}

	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("exchange declare: %w", err)
	}

	log.Println("mq: publisher connected")
	return &Publisher{conn: conn, ch: ch}, nil
}

// PublishDocEvent publishes a document operation event to three routing keys:
//   - doc.persist  — consumed by Java to persist the event log
//   - doc.spell    — consumed by spell-check service
//   - doc.metric   — consumed by metrics aggregator
func (p *Publisher) PublishDocEvent(ev DocEvent) {
	body, err := json.Marshal(ev)
	if err != nil {
		log.Printf("mq: marshal error: %v", err)
		return
	}

	keys := []string{"op.persist", "op.spell", "op.metric"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, key := range keys {
		if err := p.ch.PublishWithContext(ctx, exchange, key, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Body:         body,
			}); err != nil {
			log.Printf("mq: publish to %s failed: %v", key, err)
		}
	}
}

func (p *Publisher) Close() {
	p.ch.Close()
	p.conn.Close()
}

// newEventID returns a random UUID v4 string.
func newEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:]),
	)
}
