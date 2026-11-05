package ws

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestOverflowClosesStateBeforeBlockedUnsubscribe(t *testing.T) {
	h, c := newWSHarness(t)
	var armed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &gatedConn{Conn: conn, armed: &armed, entered: entered, release: release}, nil
	}
	c.config.Dialer = &dialer
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	b := subscribeMarket(t, c, []string{"a"}, false)
	anchor := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	streams := []*Stream[Message]{a, b}
	versions := make([]uint64, len(streams))
	for i, s := range streams {
		versions[i] = awaitState(t, s, PhaseReplaySent, 1, 0).Version
		select {
		case <-s.StateChanges:
		default:
		}
	}
	p.send(t, `{"event_type":"book","asset_id":"a","id":"first"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(a.messages) != 1 || len(b.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("first message did not fill both buffers")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `[{"event_type":"book","asset_id":"a","id":"lost"},{"event_type":"book","asset_id":"b","id":"same-batch"}]`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not block")
	}
	// Both slow streams must be invalidated even if removal of one is blocked
	// on I/O. State reads do not acquire the group's lock.
	for i, s := range streams {
		state := s.State()
		if state.Phase != PhaseClosed || state.Version <= versions[i] || state.Generation != 1 {
			t.Fatalf("state while unsubscribe blocked: %+v", state)
		}
		select {
		case <-s.StateChanges:
		case <-time.After(3 * time.Second):
			t.Fatal("missing closed wakeup")
		}
		select {
		case _, ok := <-s.StateChanges:
			if ok {
				t.Fatal("state notifications did not close")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("state notifications still open")
		}
	}
	if state := anchor.State(); state.Phase != PhaseReplaySent {
		t.Fatalf("healthy stream changed state: %+v", state)
	}
	// The physical connection is still blocked in Write; both subsequent batch
	// elements and a later frame must reach the healthy stream.
	receive(t, anchor, `"same-batch"`)
	p.send(t, `{"event_type":"book","asset_id":"b","id":"while-blocked"}`)
	receive(t, anchor, `"while-blocked"`)
	closed := make(chan error, 1)
	go func() { closed <- a.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent Close blocked on unsubscribe")
	}
	armed.Store(false)
	unblock()
	p.request(t, "unsubscribe", "a")
	for _, s := range streams {
		requireStreamError(t, s, errSlowConsumer)
		msg := nextMessage(t, s)
		if string(msg.Raw) != `{"event_type":"book","asset_id":"a","id":"first"}` {
			t.Fatalf("unexpected buffered message: %s", msg.Raw)
		}
		select {
		case _, ok := <-s.Messages:
			if ok {
				t.Fatal("overflowed stream received another message")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("overflowed stream did not close")
		}
	}
	p.send(t, `{"event_type":"book","asset_id":"b"}`)
	receive(t, anchor, `"b"`)
}
