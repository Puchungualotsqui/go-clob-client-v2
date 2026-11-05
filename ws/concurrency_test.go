package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConcurrentSubscriptionsCancellationAndReconnect(t *testing.T) {
	connected := make(chan *websocket.Conn, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connected <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{InitialBackoff: time.Millisecond, HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := client.SubscribeMarket(context.Background(), []string{"a"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	var first *websocket.Conn
	select {
	case first = <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("no connection")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				ctx, cancel := context.WithCancel(context.Background())
				s, err := client.SubscribeMarket(ctx, []string{"a", "b", "b"}, false)
				if err != nil {
					t.Error(err)
					cancel()
					return
				}
				cancel()
				_ = s.Close()
			}
		}()
	}
	_ = first.Close()
	wg.Wait()
	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("did not reconnect")
	}
}

func TestExhaustedRetriesCloseSharedStreamsAndAllowNewGroup(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := subscribeMarket(t, client, []string{"a"}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no dial")
	}
	b := subscribeMarket(t, client, []string{"b"}, false)
	close(release)
	for _, stream := range []*Stream[Message]{a, b} {
		select {
		case err, ok := <-stream.Errors:
			if !ok || err == nil {
				t.Fatal("missing dial error")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no dial error")
		}
		select {
		case _, ok := <-stream.Messages:
			if ok {
				t.Fatal("unexpected message")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("stream did not close")
		}
		if state := stream.State(); state.Phase != PhaseClosed {
			t.Fatalf("terminal state: %+v", state)
		}
		for range stream.StateChanges {
		}
	}
	c := subscribeMarket(t, client, []string{"c"}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("new group did not dial")
	}
	select {
	case _, ok := <-c.Messages:
		if ok {
			t.Fatal("unexpected message")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("new stream did not close")
	}
}

func TestCancellationDoesNotStopOtherStream(t *testing.T) {
	h, c := newWSHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	a, err := c.SubscribeMarket(ctx, []string{"a", "b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	p := h.peer(t)
	p.request(t, "subscribe", "a", "b")
	b := subscribeMarket(t, c, []string{"b"}, false)
	cancel()
	p.request(t, "unsubscribe", "a")
	select {
	case _, ok := <-a.Messages:
		if ok {
			t.Fatal("canceled stream still open")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation timed out")
	}
	if state := a.State(); state.Phase != PhaseClosed {
		t.Fatalf("canceled state: %+v", state)
	}
	p.send(t, `{"event_type":"book","asset_id":"b"}`)
	receive(t, b, `"b"`)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "b")
}
