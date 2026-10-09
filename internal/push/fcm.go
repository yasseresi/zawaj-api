package push

import (
	"context"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
)

// fcmBatchLimit is FCM's hard cap on tokens per multicast request.
const fcmBatchLimit = 500

// FCM is a Sender backed by Firebase Cloud Messaging (HTTP v1).
type FCM struct {
	client *messaging.Client
	log    *slog.Logger
}

// New builds a Sender. It initializes the Firebase Admin SDK using Application
// Default Credentials (set GOOGLE_APPLICATION_CREDENTIALS to a service-account
// JSON path). If credentials or the messaging client are unavailable, it logs a
// warning and returns a Noop so the app runs without push in dev/CI.
func New(ctx context.Context, log *slog.Logger) Sender {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		log.Warn("push: firebase init failed, using no-op sender", "error", err)
		return NewNoop(log)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		log.Warn("push: messaging client init failed, using no-op sender", "error", err)
		return NewNoop(log)
	}
	log.Info("push: FCM sender enabled")
	return &FCM{client: client, log: log}
}

// Send delivers msg to tokens in batches of 500, collecting tokens FCM reports
// as unregistered or invalid so the caller can prune them.
func (f *FCM) Send(ctx context.Context, tokens []string, msg Message) []string {
	var invalid []string
	for start := 0; start < len(tokens); start += fcmBatchLimit {
		end := start + fcmBatchLimit
		if end > len(tokens) {
			end = len(tokens)
		}
		batch := tokens[start:end]

		resp, err := f.client.SendEachForMulticast(ctx, &messaging.MulticastMessage{
			Tokens: batch, //nolint:staticcheck // SA1019: the app registers FCM registration tokens, not FIDs; migrate with the client.
			Notification: &messaging.Notification{
				Title: msg.Title,
				Body:  msg.Body,
			},
			Data: msg.Data,
		})
		if err != nil {
			// Whole-batch failure (network/auth). Log and move on — best-effort.
			f.log.Error("push: send batch failed", "error", err, "tokens", len(batch))
			continue
		}
		for i, r := range resp.Responses {
			if r.Success {
				continue
			}
			if messaging.IsUnregistered(r.Error) || messaging.IsInvalidArgument(r.Error) {
				invalid = append(invalid, batch[i])
			} else {
				f.log.Warn("push: token delivery failed", "error", r.Error)
			}
		}
	}
	return invalid
}
