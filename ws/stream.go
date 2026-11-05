package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Config controls heartbeat, reconnection and buffering. Zero values use defaults.
// MaxAttempts is the maximum consecutive failed dials; zero retries indefinitely.
type Config struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	MaxAttempts       int
	BufferSize        int
	// Dialer may be set to configure proxies, TLS, or handshake timeouts.
	Dialer *websocket.Dialer
	Header http.Header
}

func (c Config) defaults() Config {
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 5 * time.Second
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = 15 * time.Second
	}
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = time.Minute
	}
	if c.MaxBackoff < c.InitialBackoff {
		c.MaxBackoff = c.InitialBackoff
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 1024
	}
	if c.Dialer == nil {
		c.Dialer = websocket.DefaultDialer
	}
	return c
}

// Stream delivers messages until Close, context cancellation, buffer overflow, or
// exhausted retries. Errors reports connection and parse failures on a best-effort
// basis. All channels close when the stream stops. Consumers must drain Messages;
// a full buffer terminates only that stream rather than silently losing data.
// Replays reports successful full subscription writes, coalescing to the latest
// generation if the consumer is slow; it does not acknowledge server acceptance.
// StateChanges is a coalescing wakeup for State, the authoritative lifecycle snapshot.
type Stream[T any] struct {
	Messages     <-chan T
	Errors       <-chan error
	Replays      <-chan SubscriptionReplay
	StateChanges <-chan struct{}
	messages     chan T
	errors       chan error
	replays      chan SubscriptionReplay
	stateChanges chan struct{}
	stateMu      sync.Mutex
	state        StreamState
	done         chan struct{}
	close        func() error
}

// Close releases this stream's subscriptions and waits for its channels to close.
// It is safe to call concurrently or more than once.
func (s *Stream[T]) Close() error { return s.close() }

// StreamPhase describes the current state of a stream's shared connection group.
type StreamPhase string

const (
	PhaseConnecting  StreamPhase = "connecting"
	PhaseReplaySent  StreamPhase = "replay_sent"
	PhaseInterrupted StreamPhase = "interrupted"
	PhaseClosed      StreamPhase = "closed"
)

// StreamState is an instantaneous, versioned lifecycle snapshot. Generation
// counts successful full subscription writes in the group; Interruptions counts
// retired connections while the group had subscribers. Both start at zero and
// are scoped to the group. A replay-sent phase is not a server acknowledgement.
type StreamState struct {
	Phase         StreamPhase
	Version       uint64
	Generation    uint64
	Interruptions uint64
}

// State returns the current lifecycle snapshot, including after stream closure.
// StateChanges is a coalescing wakeup: after receiving one, read State rather
// than inferring state from the notification. Only one consumer should drain it.
func (s *Stream[T]) State() StreamState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}

func (s *Stream[T]) setState(phase StreamPhase, generation, interruptions uint64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = StreamState{Phase: phase, Version: s.state.Version + 1, Generation: generation, Interruptions: interruptions}
	select {
	case s.stateChanges <- struct{}{}:
	default:
	}
	if phase == PhaseClosed {
		close(s.stateChanges)
	}
}

// SubscriptionReplay reports a successful full subscription write on a group
// connection, including the initial write. Generation starts at 1 for each
// connection group; Reconnected is true after its first successful write.
// It does not mean that the server accepted the request or that gaps were filled.
type SubscriptionReplay struct {
	Generation  uint64
	Reconnected bool
}

var errSlowConsumer = errors.New("websocket message buffer full; stream stopped")

// Bound per-connection retirement history even under unbounded ID churn.
const maxReleasedIDs = 4096

// ErrReconnecting signals an intentional connection replacement. Events may be
// missed before subscriptions are replayed; consumers should reconcile state.
var ErrReconnecting = errors.New("websocket subscription changed; reconnecting, events may be missed")

// ErrRetriesExhausted identifies the terminal error when dial retries are exhausted.
var ErrRetriesExhausted = errors.New("websocket dial retries exhausted")

type groupKey struct {
	channel     string
	credentials Credentials
}

type subscriber struct {
	stream *Stream[Message]
	ids    map[string]bool
	parse  func([]byte) ([]Message, error)
}

// Group state and websocket writes are protected by mu, independently of other
// groups. The client mutex protects only its group map. When both are needed,
// mu is acquired first; lookups release the client mutex before waiting on mu.
type connectionGroup struct {
	mu            sync.Mutex
	retired       bool
	client        *Client
	key           groupKey
	cfg           Config
	ctx           context.Context
	cancel        context.CancelFunc
	conn          *websocket.Conn
	subs          map[*subscriber]bool
	refs          map[string]int
	released      map[string]struct{}
	generation    uint64
	interruptions uint64
	wildcards     int
	custom        bool
}

func (c *Client) subscribe(ctx context.Context, key groupKey, ids []string, custom bool) (*Stream[Message], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if !set[id] {
			set[id] = true
			unique = append(unique, id)
		}
	}
	var g *connectionGroup
	for {
		c.mu.Lock()
		if c.groups == nil {
			c.groups = make(map[groupKey]*connectionGroup)
		}
		g = c.groups[key]
		if g == nil {
			groupCtx, cancel := context.WithCancel(context.Background())
			g = &connectionGroup{client: c, key: key, cfg: c.config.defaults(), ctx: groupCtx, cancel: cancel, subs: make(map[*subscriber]bool), refs: make(map[string]int)}
			g.mu.Lock() // A new group cannot be contended before publication.
			c.groups[key] = g
			c.mu.Unlock()
			go g.run()
		} else {
			c.mu.Unlock()
			g.mu.Lock()
		}
		if !g.retired {
			break
		}
		g.mu.Unlock()
	}
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		if len(g.subs) == 0 {
			g.retireLocked()
		}
		return nil, err
	}
	messages := make(chan Message, g.cfg.BufferSize)
	errs := make(chan error, 32)
	replays := make(chan SubscriptionReplay, 1)
	stateChanges := make(chan struct{}, 1)
	s := &Stream[Message]{Messages: messages, Errors: errs, Replays: replays, StateChanges: stateChanges, messages: messages, errors: errs, replays: replays, stateChanges: stateChanges, done: make(chan struct{})}
	phase := PhaseConnecting
	if g.conn != nil {
		phase = PhaseReplaySent
	} else if g.interruptions > 0 {
		phase = PhaseInterrupted
	}
	s.setState(phase, g.generation, g.interruptions)
	sub := &subscriber{stream: s, ids: set}
	if key.channel == "market" {
		sub.parse = marketParser(unique)
	}
	s.close = func() error { return g.remove(sub, nil) }
	oldWildcard := g.wildcards > 0
	var added []string
	reused := false
	for _, id := range unique {
		if g.refs[id] == 0 {
			added = append(added, id)
			if _, ok := g.released[id]; ok {
				reused = true
			}
		}
		g.refs[id]++
	}
	if key.channel == "user" && len(unique) == 0 {
		g.wildcards++
	}
	upgrade := custom && !g.custom && len(g.subs) > 0
	g.custom = g.custom || custom
	g.subs[sub] = true
	if g.conn != nil {
		if reused || upgrade || (!oldWildcard && g.wildcards > 0 && len(g.refs) > 0) {
			g.reconnectLocked()
		} else if !oldWildcard && g.wildcards > 0 {
			g.sendLocked("subscribe", nil)
		} else if g.wildcards == 0 && len(added) > 0 {
			g.sendLocked("subscribe", added)
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = g.remove(sub, nil)
		case <-s.done:
		}
	}()
	return s, nil
}

// sendLocked closes a failed connection so its complete desired state is replayed.
func (g *connectionGroup) sendLocked(operation string, ids []string) error {
	if g.conn == nil {
		return nil
	}
	req := SubscriptionRequest{Type: g.key.channel, Operation: operation, Markets: []string{}, AssetIDs: []string{}}
	if g.key.channel == "market" {
		req.AssetIDs = append([]string{}, ids...)
	} else {
		req.Markets = append([]string{}, ids...)
		req.Auth = &g.key.credentials
	}
	if operation == "subscribe" {
		yes := true
		req.InitialDump = &yes
		if g.custom {
			req.CustomFeatureEnabled = &yes
		}
	}
	data, _ := json.Marshal(req)
	_ = g.conn.SetWriteDeadline(time.Now().Add(time.Second))
	err := g.conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		g.disconnectLocked()
		g.reportLocked(fmt.Errorf("websocket write: %w", err), true)
	}
	return err
}

func (g *connectionGroup) reconnectLocked() {
	g.disconnectLocked()
	g.reportLocked(ErrReconnecting, true)
}

func (g *connectionGroup) disconnectLocked() {
	if g.conn != nil {
		conn := g.conn
		g.conn = nil
		g.released = nil
		if len(g.subs) > 0 && !g.retired {
			g.interruptions++
			for sub := range g.subs {
				sub.stream.setState(PhaseInterrupted, g.generation, g.interruptions)
			}
		}
		_ = conn.Close()
	}
}

func (g *connectionGroup) remove(sub *subscriber, reason error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.subs[sub] {
		return nil
	}
	delete(g.subs, sub)
	oldWildcard := g.wildcards > 0
	if g.key.channel == "user" && len(sub.ids) == 0 {
		g.wildcards--
	}
	var removed []string
	for id := range sub.ids {
		g.refs[id]--
		if g.refs[id] == 0 {
			delete(g.refs, id)
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	var err error
	if g.conn != nil {
		if oldWildcard && g.wildcards == 0 && len(g.subs) > 0 {
			g.reconnectLocked()
		} else if oldWildcard && len(g.subs) == 0 {
			err = g.sendLocked("unsubscribe", nil)
		} else if !oldWildcard && len(removed) > 0 {
			err = g.sendLocked("unsubscribe", removed)
		}
	}
	if g.conn != nil && len(g.subs) > 0 && len(removed) > 0 {
		if g.released == nil {
			g.released = make(map[string]struct{})
		}
		for _, id := range removed {
			g.released[id] = struct{}{}
			if len(g.released) >= maxReleasedIDs {
				g.reconnectLocked()
				break
			}
		}
	}
	if reason != nil {
		reportStream(sub.stream, reason, true)
	}
	sub.stream.setState(PhaseClosed, g.generation, g.interruptions)
	close(sub.stream.messages)
	close(sub.stream.errors)
	close(sub.stream.replays)
	close(sub.stream.done)
	if len(g.subs) == 0 {
		g.retireLocked()
	}
	return err
}

func (g *connectionGroup) retireLocked() {
	g.retired = true
	g.client.mu.Lock()
	if g.client.groups[g.key] == g {
		delete(g.client.groups, g.key)
	}
	g.client.mu.Unlock()
	g.cancel()
	g.disconnectLocked()
}

// Critical lifecycle errors take precedence over older diagnostic errors when
// the bounded channel is full. Callers must still drain Errors continuously.
func reportStream(s *Stream[Message], err error, critical bool) {
	select {
	case s.errors <- err:
		return
	default:
	}
	if critical {
		select {
		case <-s.errors:
		default:
		}
		select {
		case s.errors <- err:
		default:
		}
	}
}

func (g *connectionGroup) reportLocked(err error, critical bool) {
	for sub := range g.subs {
		reportStream(sub.stream, err, critical)
	}
}

func (g *connectionGroup) report(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reportLocked(err, false)
}

func (g *connectionGroup) reportConn(conn *websocket.Conn, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn == conn {
		g.reportLocked(err, false)
	}
}

func (g *connectionGroup) run() {
	attempts := 0
	backoff := g.cfg.InitialBackoff
	for g.ctx.Err() == nil {
		conn, response, err := g.cfg.Dialer.DialContext(g.ctx, g.client.endpoint+"/ws/"+g.key.channel, g.cfg.Header)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			if g.ctx.Err() != nil {
				return
			}
			attempts++
			if g.cfg.MaxAttempts > 0 && attempts >= g.cfg.MaxAttempts {
				g.mu.Lock()
				if !g.retired {
					g.retireLocked()
					terminal := fmt.Errorf("%w: %w", ErrRetriesExhausted, err)
					for sub := range g.subs {
						reportStream(sub.stream, terminal, true)
						sub.stream.setState(PhaseClosed, g.generation, g.interruptions)
						close(sub.stream.messages)
						close(sub.stream.errors)
						close(sub.stream.replays)
						close(sub.stream.done)
						delete(g.subs, sub)
					}
				}
				g.mu.Unlock()
				return
			}
			g.report(fmt.Errorf("websocket dial: %w", err))
		} else {
			attempts = 0
			backoff = g.cfg.InitialBackoff
			g.mu.Lock()
			if g.ctx.Err() != nil {
				g.mu.Unlock()
				_ = conn.Close()
				return
			}
			g.conn = conn
			g.released = nil
			ids := make([]string, 0, len(g.refs))
			if g.wildcards == 0 {
				for id := range g.refs {
					ids = append(ids, id)
				}
				sort.Strings(ids)
			}
			err = g.sendLocked("subscribe", ids)
			if err == nil {
				g.generation++
				replay := SubscriptionReplay{Generation: g.generation, Reconnected: g.generation > 1}
				for sub := range g.subs {
					sub.stream.setState(PhaseReplaySent, g.generation, g.interruptions)
					select {
					case <-sub.stream.replays:
					default:
					}
					sub.stream.replays <- replay
				}
			}
			g.mu.Unlock()
			if err == nil {
				err = g.read(conn)
			}
			g.mu.Lock()
			if g.conn == conn {
				g.disconnectLocked()
				if err != nil && g.ctx.Err() == nil {
					g.reportLocked(fmt.Errorf("websocket connection: %w", err), true)
				}
			}
			g.mu.Unlock()
			_ = conn.Close()
			if g.ctx.Err() != nil {
				return
			}
		}
		timer := time.NewTimer(backoff)
		select {
		case <-g.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < g.cfg.MaxBackoff/2 {
			backoff *= 2
		} else {
			backoff = g.cfg.MaxBackoff
		}
	}
}

func (g *connectionGroup) read(conn *websocket.Conn) error {
	pong := make(chan struct{}, 1)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(g.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-g.ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
			}
			select {
			case <-pong:
			default:
			}
			g.mu.Lock()
			var err error
			if g.conn == conn {
				_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
				err = conn.WriteMessage(websocket.TextMessage, []byte("PING"))
				if err != nil {
					g.disconnectLocked()
					g.reportLocked(fmt.Errorf("websocket heartbeat write: %w", err), true)
				}
			} else {
				err = errors.New("connection replaced")
			}
			g.mu.Unlock()
			if err != nil {
				_ = conn.Close()
				return
			}
			timer := time.NewTimer(g.cfg.HeartbeatTimeout)
			select {
			case <-pong:
				timer.Stop()
			case <-timer.C:
				g.mu.Lock()
				if g.conn == conn {
					g.disconnectLocked()
					g.reportLocked(errors.New("websocket heartbeat timeout"), true)
				}
				g.mu.Unlock()
				return
			case <-g.ctx.Done():
				timer.Stop()
				return
			case <-stop:
				timer.Stop()
				return
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if kind != websocket.TextMessage {
			continue
		}
		if string(data) == "PONG" {
			select {
			case pong <- struct{}{}:
			default:
			}
			continue
		}
		messages, err := ParseMessages(data)
		if err != nil {
			g.reportConn(conn, fmt.Errorf("websocket message: %w", err))
		}
		for _, msg := range messages {
			g.deliver(conn, msg)
		}
	}
}

func (g *connectionGroup) deliver(conn *websocket.Conn, msg Message) {
	g.mu.Lock()
	if g.conn != conn {
		g.mu.Unlock()
		return
	}
	var slow []*subscriber
	for sub := range g.subs {
		matched := false
		if g.key.channel == "market" {
			messages, err := sub.parse(msg.Raw)
			if err != nil {
				select {
				case sub.stream.errors <- fmt.Errorf("websocket message: %w", err):
				default:
				}
				continue
			}
			matched = len(messages) > 0
		} else if msg.EventType == "order" || msg.EventType == "trade" {
			var event map[string]json.RawMessage
			if err := json.Unmarshal(msg.Raw, &event); err != nil {
				select {
				case sub.stream.errors <- fmt.Errorf("websocket message: %w", err):
				default:
				}
				continue
			}
			market := ""
			if raw := event["market"]; len(raw) != 0 && !isNull(raw) {
				var err error
				market, err = routingMarketID(raw)
				if err != nil {
					select {
					case sub.stream.errors <- fmt.Errorf("websocket message: market: %w", err):
					default:
					}
					continue
				}
			}
			matched = len(sub.ids) == 0 || sub.ids[market]
		}
		if matched {
			// Each subscriber owns its Raw bytes; callers may modify them.
			copyMsg := Message{EventType: msg.EventType, Raw: append(json.RawMessage(nil), msg.Raw...)}
			select {
			case sub.stream.messages <- copyMsg:
			default:
				slow = append(slow, sub)
			}
		}
	}
	g.mu.Unlock()
	for _, sub := range slow {
		_ = g.remove(sub, errSlowConsumer)
	}
}
