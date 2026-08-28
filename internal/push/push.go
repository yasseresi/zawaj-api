// Package push delivers notifications to user devices via FCM. The Sender
// interface decouples producers (the notification service) from the transport,
// so tests and credential-less environments use a no-op while production uses
// Firebase Cloud Messaging.
package push

import (
	"context"
	"log/slog"
)

// Message is a transport-agnostic push payload. Data carries small string k/v
// pairs the client uses to route/deep-link (e.g. type, wedding_id).
type Message struct {
	Title string
	Body  string
	Data  map[string]string
}

// Sender delivers a message to a set of device tokens. It is best-effort: it
// never fails the caller. It returns the subset of tokens FCM reported as
// permanently invalid (unregistered / bad), which the caller should delete.
type Sender interface {
	Send(ctx context.Context, tokens []string, msg Message) (invalid []string)
}

// Noop is a Sender that logs and delivers nothing. Used in tests and when no
// Firebase credentials are configured.
type Noop struct{ log *slog.Logger }

// NewNoop builds a no-op sender.
func NewNoop(log *slog.Logger) *Noop { return &Noop{log: log} }

// Send logs the intent and returns no invalid tokens.
func (n *Noop) Send(_ context.Context, tokens []string, msg Message) []string {
	if n.log != nil && len(tokens) > 0 {
		n.log.Debug("push noop", "tokens", len(tokens), "title", msg.Title)
	}
	return nil
}
