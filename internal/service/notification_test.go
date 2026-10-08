package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Shutdown waits for in-flight pushes, but never past its deadline.
func TestWaitContextIsBoundedAndReturnsWhenPushesFinish(t *testing.T) {
	s := &NotificationService{}
	s.inflight.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.WaitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("with a push still running: want DeadlineExceeded, got %v", err)
	}

	s.inflight.Done()
	if err := s.WaitContext(context.Background()); err != nil {
		t.Fatalf("after pushes finished: want nil, got %v", err)
	}
}
