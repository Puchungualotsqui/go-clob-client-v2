package ws

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func overflowTestDialer(c *Client) (<-chan struct{}, func(), *atomic.Bool) {
	entered, release := make(chan struct{}), make(chan struct{})
	armed := &atomic.Bool{}
	var count atomic.Int32
	var once sync.Once
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if count.Add(1) == 1 {
			return &gatedConn{Conn: conn, armed: armed, entered: entered, release: release}, nil
		}
		return conn, nil
	}
	c.config.Dialer = &dialer
	return entered, func() { once.Do(func() { close(release) }) }, armed
}

func waitOverflow(t *testing.T, s *Stream[Message]) {
	t.Helper()
	select {
	case <-s.done:
		if s.State().Phase != PhaseClosed {
			t.Fatalf("overflow did not close state: %+v", s.State())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("overflow cleanup did not complete")
	}
}

func TestOverflowQueuesAdditionalUnsubscribesWithoutBlockingDelivery(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release, armed := overflowTestDialer(c)
	defer release()
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	b := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	anchor := subscribeMarket(t, c, []string{"c"}, false)
	p.request(t, "subscribe", "c")
	p.send(t, `{"event_type":"book","asset_id":"a","id":"first"}`)
	p.send(t, `{"event_type":"book","asset_id":"b","id":"first"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(a.messages) != 1 || len(b.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("buffers did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `{"event_type":"book","asset_id":"a","id":"lost"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not start")
	}
	waitOverflow(t, a)
	p.send(t, `{"event_type":"book","asset_id":"b","id":"lost"}`)
	waitOverflow(t, b)
	p.send(t, `{"event_type":"book","asset_id":"c","id":"healthy"}`)
	receive(t, anchor, `"healthy"`)
	armed.Store(false)
	release()
	p.request(t, "unsubscribe", "a")
	p.request(t, "unsubscribe", "b")
	requireStreamError(t, a, errSlowConsumer)
	requireStreamError(t, b, errSlowConsumer)
}

type failingOverflowConn struct {
	net.Conn
	armed            *atomic.Bool
	entered, release chan struct{}
	err              error
}

func (c *failingOverflowConn) Write(data []byte) (int, error) {
	if c.armed.Load() {
		close(c.entered)
		<-c.release
		return 0, c.err
	}
	return c.Conn.Write(data)
}

func TestOverflowUnsubscribeFailureRetiresConnection(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var armed atomic.Bool
	var count atomic.Int32
	failure := errors.New("injected overflow unsubscribe failure")
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if count.Add(1) == 1 {
			return &failingOverflowConn{Conn: conn, armed: &armed, entered: entered, release: release, err: failure}, nil
		}
		return conn, nil
	}
	c.config.Dialer = &dialer
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	anchor := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(a.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("buffer did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not start")
	}
	p.send(t, `{"event_type":"book","asset_id":"b","id":"before-error"}`)
	receive(t, anchor, `"before-error"`)
	unblock()
	requireStreamError(t, anchor, failure)
	p = h.peer(t)
	p.request(t, "subscribe", "b")
	state := awaitState(t, anchor, PhaseReplaySent, 2, 1)
	if state.StrictMarketRejections != 0 {
		t.Fatalf("unexpected strict rejection: %+v", state)
	}
	p.send(t, `{"event_type":"book","asset_id":"b","id":"after-replay"}`)
	receive(t, anchor, `"after-replay"`)
}

func TestOverflowLastStreamUnpublishesBeforeUnsubscribeCompletes(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release, armed := overflowTestDialer(c)
	defer release()
	old := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `{"event_type":"book","asset_id":"a","id":"first"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(old.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("buffer did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `{"event_type":"book","asset_id":"a","id":"lost"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not start")
	}
	waitOverflow(t, old)
	fresh := subscribeMarket(t, c, []string{"a"}, false)
	newPeer := h.peer(t)
	newPeer.request(t, "subscribe", "a")
	newPeer.send(t, `{"event_type":"book","asset_id":"a","id":"new-group"}`)
	receive(t, fresh, `"new-group"`)
	if state := fresh.State(); state.Generation != 1 || state.Interruptions != 0 {
		t.Fatalf("new group inherited old state: %+v", state)
	}
	armed.Store(false)
	release()
	p.request(t, "unsubscribe", "a")
}

func TestOverflowNewSubscriptionReconnectsWhileUnsubscribeBlocked(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release, armed := overflowTestDialer(c)
	defer release()
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	anchor := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(a.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("buffer did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not start")
	}
	waitOverflow(t, a)
	newStream := subscribeMarket(t, c, []string{"c"}, false)
	requireStreamError(t, anchor, ErrReconnecting)
	requireStreamError(t, newStream, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "b", "c")
	p.send(t, `{"event_type":"book","asset_id":"c","id":"new"}`)
	receive(t, newStream, `"new"`)
	armed.Store(false)
	release()
	p.send(t, `{"event_type":"book","asset_id":"b","id":"healthy"}`)
	receive(t, anchor, `"healthy"`)
}

func TestOverflowReuseReconnectsWhileUnsubscribeBlocked(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release, armed := overflowTestDialer(c)
	defer release()
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	anchor := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	p.send(t, `{"event_type":"book","asset_id":"a","id":"first"}`)
	deadline := time.Now().Add(3 * time.Second)
	for len(a.messages) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("buffer did not fill")
		}
		time.Sleep(time.Millisecond)
	}
	armed.Store(true)
	p.send(t, `{"event_type":"book","asset_id":"a","id":"lost"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("unsubscribe did not start")
	}
	waitOverflow(t, a)
	replacement := subscribeMarket(t, c, []string{"a"}, false)
	requireStreamError(t, anchor, ErrReconnecting)
	requireStreamError(t, replacement, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "a", "b")
	p.send(t, `{"event_type":"book","asset_id":"a","id":"new"}`)
	receive(t, replacement, `"new"`)
	armed.Store(false)
	release() // The old blocked write must not disturb the replacement connection.
	p.send(t, `{"event_type":"book","asset_id":"b","id":"still-active"}`)
	receive(t, anchor, `"still-active"`)
	if state := anchor.State(); state.Phase != PhaseReplaySent || state.Generation != 2 || state.Interruptions != 1 {
		t.Fatalf("stale write affected replacement: %+v", state)
	}
}
