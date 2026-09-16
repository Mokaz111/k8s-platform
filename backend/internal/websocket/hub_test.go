package websocket

import (
	"context"
	"testing"
	"time"
)

func TestChannelSubscriberCountEmpty(t *testing.T) {
	h := NewHub(nil, nil)
	if h.ChannelSubscriberCount("pod_logs:c:ns:p") != 0 {
		t.Fatal("expected 0 subscribers")
	}
}

func TestWaitForSubscribersTimeout(t *testing.T) {
	s := &PodLogService{hub: NewHub(nil, nil)}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	s.WaitForSubscribers(ctx, "pod_logs:c:ns:p")
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("should wait until context is done")
	}
}
