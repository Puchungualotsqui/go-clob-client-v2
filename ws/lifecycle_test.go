package ws

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRetiredConnectionCannotDeliverToNewSubscribers(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	old := g.conn
	for i := 0; i < cap(a.errors); i++ {
		reportStream(a, errors.New("older diagnostic"), false)
	}
	g.mu.Unlock()
	if old == nil {
		t.Fatal("connection missing")
	}
	b := subscribeMarket(t, c, []string{"a"}, true)
	requireStreamError(t, a, ErrReconnecting)
	requireStreamError(t, b, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	// Simulate a frame and parse error read before retirement but processed after it.
	g.deliver(old, Message{EventType: "book", Raw: []byte(`{"event_type":"book","asset_id":"a","id":"stale"}`)})
	g.reportConn(old, errors.New("stale parse error"))
	p.send(t, `{"event_type":"book","asset_id":"a","id":"fresh"}`)
	receive(t, a, `"fresh"`)
	receive(t, b, `"fresh"`)
	for _, s := range []*Stream[Message]{a, b} {
		select {
		case err := <-s.Errors:
			t.Fatalf("retired connection reported error: %v", err)
		default:
		}
	}
}

func TestSubscriptionWriteFailureReportsBeforeRetirement(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	// Keep the read loop from retiring the connection first, as if a write fails
	// while a subscription is being changed.
	_ = g.conn.UnderlyingConn().Close()
	err := g.sendLocked("subscribe", []string{"b"})
	g.mu.Unlock()
	if err == nil {
		t.Fatal("expected write failure")
	}
	select {
	case got := <-a.Errors:
		if got == nil || !strings.Contains(got.Error(), "websocket write") {
			t.Fatalf("missing write error: %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("write failure was silent")
	}
}

type gatedConn struct {
	net.Conn
	armed   *atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *gatedConn) Write(p []byte) (int, error) {
	if c.armed.Load() {
		c.once.Do(func() { close(c.entered) })
		<-c.release
	}
	return c.Conn.Write(p)
}

func TestBlockedWriteDoesNotBlockOtherGroups(t *testing.T) {
	h, c := newWSHarness(t)
	var armed atomic.Bool
	var dialCount atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if dialCount.Add(1) == 1 {
			return &gatedConn{Conn: conn, armed: &armed, entered: entered, release: release}, nil
		}
		return conn, nil
	}
	c.config.Dialer = &dialer
	a := subscribeMarket(t, c, []string{"a"}, false)
	_ = a
	marketPeer := h.peer(t)
	marketPeer.request(t, "subscribe", "a")
	armed.Store(true)
	type result struct {
		stream *Stream[Message]
		err    error
	}
	marketResult := make(chan result, 1)
	go func() {
		s, err := c.SubscribeMarket(context.Background(), []string{"b"}, false)
		marketResult <- result{s, err}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("write did not block")
	}
	userResult := make(chan result, 1)
	go func() {
		s, err := c.SubscribeUser(context.Background(), []string{"m"}, Credentials{Key: "k", Secret: "s", Passphrase: "p"})
		userResult <- result{s, err}
	}()
	var user *Stream[Message]
	select {
	case r := <-userResult:
		if r.err != nil {
			t.Fatal(r.err)
		}
		user = r.stream
	case <-time.After(3 * time.Second):
		t.Fatal("unrelated user subscription blocked by market write")
	}
	defer user.Close()
	userPeer := h.peer(t)
	userPeer.request(t, "subscribe", "m")
	userPeer.send(t, `{"event_type":"order","market":"m"}`)
	receive(t, user, `"m"`)
	armed.Store(false)
	unblock()
	select {
	case r := <-marketResult:
		if r.err != nil {
			t.Fatal(r.err)
		}
		defer r.stream.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("market subscription did not resume")
	}
	marketPeer.request(t, "subscribe", "b")
}

func TestRetryExhaustionWithConcurrentSubscriberAndFullErrors(t *testing.T) {
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	responded := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		responded <- struct{}{}
	}))
	defer server.Close()
	c, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := subscribeMarket(t, c, []string{"a"}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no dial")
	}
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	for i := 0; i < cap(a.errors); i++ {
		reportStream(a, errors.New("older diagnostic"), false)
	}
	started := make(chan struct{})
	result := make(chan *Stream[Message], 1)
	go func() {
		close(started)
		s, _ := c.SubscribeMarket(context.Background(), []string{"b"}, false)
		result <- s
	}()
	<-started
	close(release)
	select {
	case <-responded:
	case <-time.After(3 * time.Second):
		t.Fatal("no response")
	}
	g.mu.Unlock()
	var b *Stream[Message]
	select {
	case b = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber blocked")
	}
	if b == nil {
		t.Fatal("subscription failed")
	}
	defer b.Close()
	requireStreamError(t, a, ErrRetriesExhausted)
	requireStreamError(t, b, ErrRetriesExhausted)
}
