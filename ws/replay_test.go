package ws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func replay(t *testing.T, s *Stream[Message], generation uint64, reconnected bool) {
	t.Helper()
	select {
	case got, ok := <-s.Replays:
		if !ok || got.Generation != generation || got.Reconnected != reconnected {
			t.Fatalf("replay = %+v, open=%v; want generation %d reconnected=%v", got, ok, generation, reconnected)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for replay %d", generation)
	}
}

func TestRetiredIDReconnectsBeforeReuse(t *testing.T) {
	for _, mode := range []string{"market", "user", "wildcard"} {
		t.Run(mode, func(t *testing.T) {
			h, c := newWSHarness(t)
			cred := Credentials{Key: "key", Secret: "secret", Passphrase: "pass"}
			key := groupKey{channel: "market"}
			var anchor, first *Stream[Message]
			if mode == "market" {
				anchor = subscribeMarket(t, c, []string{"anchor"}, false)
			} else {
				key = groupKey{channel: "user", credentials: cred}
				ids := []string{"anchor"}
				if mode == "wildcard" {
					ids = nil
				}
				anchor = subscribeUser(t, c, ids, cred)
			}
			p := h.peer(t)
			if mode == "wildcard" {
				p.request(t, "subscribe")
			} else {
				p.request(t, "subscribe", "anchor")
			}
			replay(t, anchor, 1, false)
			if mode == "market" {
				first = subscribeMarket(t, c, []string{"target"}, false)
			} else {
				first = subscribeUser(t, c, []string{"target"}, cred)
			}
			if mode != "wildcard" {
				p.request(t, "subscribe", "target")
			}
			select {
			case got := <-first.Replays:
				t.Fatalf("incremental subscription reported replay: %+v", got)
			default:
			}
			c.mu.Lock()
			g := c.groups[key]
			c.mu.Unlock()
			g.mu.Lock()
			old := g.conn
			g.mu.Unlock()
			if old == nil {
				t.Fatal("missing old connection")
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			if mode != "wildcard" {
				p.request(t, "unsubscribe", "target")
			}
			var replacement *Stream[Message]
			if mode == "market" {
				replacement = subscribeMarket(t, c, []string{"target"}, false)
			} else {
				replacement = subscribeUser(t, c, []string{"target"}, cred)
			}
			requireStreamError(t, anchor, ErrReconnecting)
			requireStreamError(t, replacement, ErrReconnecting)
			p = h.peer(t)
			if mode == "wildcard" {
				p.request(t, "subscribe")
			} else {
				p.request(t, "subscribe", "anchor", "target")
			}
			replay(t, anchor, 2, true)
			replay(t, replacement, 2, true)
			stale := Message{EventType: "book", Raw: []byte(`{"event_type":"book","asset_id":"target","id":"stale"}`)}
			fresh := `{"event_type":"book","asset_id":"target","id":"fresh"}`
			if mode != "market" {
				stale = Message{EventType: "order", Raw: []byte(`{"event_type":"order","market":"target","id":"stale"}`)}
				fresh = `{"event_type":"order","market":"target","id":"fresh"}`
			}
			// An old frame already read but waiting for the group lock must be rejected.
			g.deliver(old, stale)
			p.send(t, fresh)
			receive(t, replacement, `"fresh"`)
			if mode == "wildcard" {
				receive(t, anchor, `"fresh"`)
			}
		})
	}
}

func TestRetirementHistoryLimitReconnects(t *testing.T) {
	h, c := newWSHarness(t)
	anchor := subscribeMarket(t, c, []string{"anchor"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "anchor")
	replay(t, anchor, 1, false)
	target := subscribeMarket(t, c, []string{"target"}, false)
	p.request(t, "subscribe", "target")
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	g.released = make(map[string]struct{}, maxReleasedIDs)
	for i := 0; i < maxReleasedIDs-1; i++ {
		g.released[fmt.Sprint(i)] = struct{}{}
	}
	g.mu.Unlock()
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "target")
	requireStreamError(t, anchor, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "anchor")
	replay(t, anchor, 2, true)
	g.mu.Lock()
	remaining := len(g.released)
	g.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("retired ID history was not cleared: %d", remaining)
	}
}

func TestReplaySignalsAndCoalescing(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 1, false)
	b := subscribeMarket(t, c, []string{"a"}, true)
	requireStreamError(t, a, ErrReconnecting)
	requireStreamError(t, b, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, b, 2, true)
	// Leave a's generation 2 unread; a later replay replaces it.
	_ = p.conn.Close()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 3, true)
	replay(t, b, 3, true)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-b.Replays:
		if ok {
			t.Fatal("replay channel still open")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replay channel did not close")
	}
}

type failReplayConn struct {
	net.Conn
	writes  int
	entered chan struct{}
	release chan struct{}
}

func (c *failReplayConn) Write(p []byte) (int, error) {
	c.writes++
	if c.writes == 2 {
		close(c.entered)
		<-c.release
		return 0, errors.New("injected replay write failure")
	}
	return c.Conn.Write(p)
}

func TestReplaySignalOnlyAfterSuccessfulWrite(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	thirdDial, allowThird := make(chan struct{}), make(chan struct{})
	var once, thirdOnce sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	unblockThird := func() { thirdOnce.Do(func() { close(allowThird) }) }
	defer unblock()
	defer unblockThird()
	var dialCount atomic.Int32
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		count := dialCount.Add(1)
		if count == 3 {
			close(thirdDial)
			select {
			case <-allowThird:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if count == 2 {
			return &failReplayConn{Conn: conn, entered: entered, release: release}, nil
		}
		return conn, nil
	}
	c.config.Dialer = &dialer
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 1, false)
	_ = p.conn.Close()
	_ = h.peer(t)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("replay write did not start")
	}
	select {
	case r := <-a.Replays:
		t.Fatalf("premature replay signal: %+v", r)
	default:
	}
	unblock()
	select {
	case <-thirdDial:
	case <-time.After(3 * time.Second):
		t.Fatal("failed replay did not retry")
	}
	awaitState(t, a, PhaseInterrupted, 1, 2)
	select {
	case r := <-a.Replays:
		t.Fatalf("failed write reported replay: %+v", r)
	default:
	}
	unblockThird()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 2, true)
	awaitState(t, a, PhaseReplaySent, 2, 2)
}
