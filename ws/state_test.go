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

func awaitState(t *testing.T, s *Stream[Message], phase StreamPhase, generation, interruptions uint64) StreamState {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		state := s.State()
		if state.Phase == phase && state.Generation == generation && state.Interruptions == interruptions {
			return state
		}
		select {
		case _, ok := <-s.StateChanges:
			if !ok {
				t.Fatalf("stream closed at %+v, want %s generation %d interruptions %d", s.State(), phase, generation, interruptions)
			}
		case <-timer.C:
			t.Fatalf("state = %+v, want %s generation %d interruptions %d", s.State(), phase, generation, interruptions)
		}
	}
}

// Pause the next dial before it can connect, allowing the interrupted state to
// be observed without depending on scheduler timing.
func gateSecondDial(c *Client) (<-chan struct{}, func()) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var dialCount atomic.Int32
	var once sync.Once
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dialCount.Add(1) == 2 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	c.config.Dialer = &dialer
	return entered, func() { once.Do(func() { close(release) }) }
}

func TestLifecycleInitialConnecting(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	c.config.Dialer = &dialer
	a := subscribeMarket(t, c, []string{"a"}, false)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not start")
	}
	initial := a.State()
	if initial.Phase != PhaseConnecting || initial.Version != 1 || initial.Generation != 0 || initial.Interruptions != 0 {
		t.Fatalf("initial state: %+v", initial)
	}
	unblock()
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	connected := awaitState(t, a, PhaseReplaySent, 1, 0)
	if connected.Version <= initial.Version {
		t.Fatal("successful replay did not advance state")
	}
}

func TestLifecycleCorrelatesUnreadReplayAndDelayedError(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release := gateSecondDial(c)
	defer release()
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	initial := awaitState(t, a, PhaseReplaySent, 1, 0)
	// Leave the first replay unread and trigger a deliberate reconnect.
	b := subscribeMarket(t, c, []string{"a"}, true)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect did not start")
	}
	interrupted := awaitState(t, a, PhaseInterrupted, 1, 1)
	if interrupted.Version <= initial.Version {
		t.Fatal("state version did not advance")
	}
	replay(t, a, 1, false)
	if got := a.State(); got.Phase != PhaseInterrupted || got.Interruptions != 1 {
		t.Fatalf("old replay obscured interruption: %+v", got)
	}
	awaitState(t, b, PhaseInterrupted, 1, 1)
	release()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	recovered := awaitState(t, a, PhaseReplaySent, 2, 1)
	awaitState(t, b, PhaseReplaySent, 2, 1)
	if recovered.Version <= interrupted.Version {
		t.Fatal("replay did not advance state version")
	}
	replay(t, a, 2, true)
	// An older error may be consumed after replay; State remains authoritative.
	requireStreamError(t, a, ErrReconnecting)
	if got := a.State(); got != recovered {
		t.Fatalf("delayed error changed recovered state: %+v", got)
	}
}

func TestLifecycleTracksNetworkReconnect(t *testing.T) {
	h, c := newWSHarness(t)
	entered, release := gateSecondDial(c)
	defer release()
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	awaitState(t, a, PhaseReplaySent, 1, 0)
	_ = p.conn.Close()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("network reconnect did not start")
	}
	awaitState(t, a, PhaseInterrupted, 1, 1)
	// Old Replays is historical even when consumed after a network interruption.
	replay(t, a, 1, false)
	if a.State().Phase != PhaseInterrupted {
		t.Fatal("state did not retain network interruption")
	}
	release()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	awaitState(t, a, PhaseReplaySent, 2, 1)
	replay(t, a, 2, true)
}

func TestLifecycleCoalescesLatestStateAndCloses(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	b := subscribeMarket(t, c, []string{"a"}, true)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	_ = p.conn.Close()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	// Never drain StateChanges while the group progresses through both reconnects.
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	state := a.State()
	g.mu.Unlock()
	if state.Phase != PhaseReplaySent || state.Generation != 3 || state.Interruptions != 2 || state.Version < 6 {
		t.Fatalf("unexpected latest state: %+v", state)
	}
	select {
	case _, ok := <-a.StateChanges:
		if !ok {
			t.Fatal("closed too early")
		}
	default:
		t.Fatal("missing wakeup")
	}
	if got := a.State(); got != state {
		t.Fatalf("wakeup exposed stale state: %+v", got)
	}
	select {
	case <-a.StateChanges:
		t.Fatal("notifications did not coalesce")
	default:
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	closed := a.State()
	if closed.Phase != PhaseClosed || closed.Generation != 3 || closed.Interruptions != 2 || closed.Version <= state.Version {
		t.Fatalf("unexpected closed state: %+v", closed)
	}
	select {
	case <-a.StateChanges:
	case <-time.After(3 * time.Second):
		t.Fatal("missing closed wakeup")
	}
	select {
	case _, ok := <-a.StateChanges:
		if ok {
			t.Fatal("state channel still open")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("state channel did not close")
	}
}

func TestJoinedStreamState(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	awaitState(t, a, PhaseReplaySent, 1, 0)
	b := subscribeMarket(t, c, []string{"a"}, false)
	if state := b.State(); state.Phase != PhaseReplaySent || state.Generation != 1 || state.Interruptions != 0 {
		t.Fatalf("joining state: %+v", state)
	}
	select {
	case <-b.Replays:
		t.Fatal("joining stream received historical replay")
	default:
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if got := b.State(); got.Phase != PhaseClosed {
		t.Fatalf("closed state: %+v", got)
	}

}
