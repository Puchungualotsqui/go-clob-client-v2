package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultReadLimit is the default maximum inbound WebSocket message size (16 MiB).
const DefaultReadLimit int64 = 16 << 20

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
	// ReadLimit limits each inbound WebSocket message in bytes, both on the wire
	// and after decompression. Values <= 0 select DefaultReadLimit.
	ReadLimit int64
	// StrictMarket rejects invalid, unknown, or unroutable market messages as a
	// whole and reconnects the shared market group. User streams are unaffected.
	StrictMarket bool
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
	if c.ReadLimit <= 0 {
		c.ReadLimit = DefaultReadLimit
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
// On overflow, State becomes PhaseClosed and StateChanges closes before unsubscribe
// I/O and possibly before the other channels close; buffered messages remain.
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
// retired connections while the group had subscribers. StrictMarketRejections
// counts active market connections retired after strict message validation failed.
// Counters start at zero and are scoped to the group. A replay-sent phase is not
// a server acknowledgement.
type StreamState struct {
	Phase                  StreamPhase
	Version                uint64
	Generation             uint64
	Interruptions          uint64
	StrictMarketRejections uint64
}

// State returns the current lifecycle snapshot, including after stream closure.
// StateChanges is a coalescing wakeup: after receiving one, read State rather
// than inferring state from the notification. Only one consumer should drain it.
func (s *Stream[T]) State() StreamState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}

func (s *Stream[T]) setState(phase StreamPhase, generation, interruptions, strictMarketRejections uint64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.setStateLocked(phase, generation, interruptions, strictMarketRejections)
}

func (s *Stream[T]) setStateLocked(phase StreamPhase, generation, interruptions, strictMarketRejections uint64) {
	if s.state.Phase == PhaseClosed {
		return
	}
	s.state = StreamState{Phase: phase, Version: s.state.Version + 1, Generation: generation, Interruptions: interruptions, StrictMarketRejections: strictMarketRejections}
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

// IDs are shared across all clients and groups in this process.
var nextConnectionID atomic.Uint64

// Bound per-connection retirement history even under unbounded ID churn.
const maxReleasedIDs = 4096

// ErrReconnecting signals an intentional connection replacement. Events may be
// missed before subscriptions are replayed; consumers should reconcile state.
var ErrReconnecting = errors.New("websocket subscription changed; reconnecting, events may be missed")

// ErrStrictMarketMessage identifies a rejected message in strict market mode.
// Errors is best-effort; use State().StrictMarketRejections for authoritative counts.
var ErrStrictMarketMessage = errors.New("strict market message rejected")

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

// Group state and synchronous writes are protected by mu, independently of other
// groups. asyncConn reserves the connection for the overflow writer outside mu.
// The client mutex protects only its group map. When both are needed, mu is
// acquired first; lookups release the client mutex before waiting on mu.
type connectionGroup struct {
	mu                     sync.Mutex
	retired                bool
	client                 *Client
	key                    groupKey
	cfg                    Config
	ctx                    context.Context
	cancel                 context.CancelFunc
	conn                   *websocket.Conn
	asyncConn              *websocket.Conn
	pendingUnsubs          map[string]struct{}
	pendingWildcardUnsub   bool
	subs                   map[*subscriber]bool
	refs                   map[string]int
	released               map[string]struct{}
	generation             uint64
	interruptions          uint64
	strictMarketRejections uint64
	wildcards              int
	custom                 bool
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
	s.setState(phase, g.generation, g.interruptions, g.strictMarketRejections)
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
	if g.asyncConn == g.conn {
		// Do not wait for an overflow unsubscribe while holding the group lock.
		// Replay the desired state on a fresh connection instead.
		g.reconnectLocked()
		return ErrReconnecting
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
		g.asyncConn = nil
		g.pendingUnsubs = nil
		g.pendingWildcardUnsub = false
		g.released = nil
		if len(g.subs) > 0 && !g.retired {
			g.interruptions++
			for sub := range g.subs {
				sub.stream.setState(PhaseInterrupted, g.generation, g.interruptions, g.strictMarketRejections)
			}
		}
		_ = conn.Close()
	}
}

func (g *connectionGroup) remove(sub *subscriber, reason error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.removeLocked(sub, reason)
}

func (g *connectionGroup) removeLocked(sub *subscriber, reason error) error {
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
	sub.stream.setState(PhaseClosed, g.generation, g.interruptions, g.strictMarketRejections)
	close(sub.stream.messages)
	close(sub.stream.errors)
	close(sub.stream.replays)
	close(sub.stream.done)
	if len(g.subs) == 0 {
		g.retireLocked()
	}
	return err
}

// removeSlowLocked detaches every overflowed subscriber before starting any I/O.
// The read loop must remain free to deliver subsequent messages to healthy streams.
func (g *connectionGroup) removeSlowLocked(slow []*subscriber) {
	oldWildcard := g.wildcards > 0
	var removed []string
	for _, sub := range slow {
		if !g.subs[sub] {
			continue
		}
		delete(g.subs, sub)
		if g.key.channel == "user" && len(sub.ids) == 0 {
			g.wildcards--
		}
		for id := range sub.ids {
			g.refs[id]--
			if g.refs[id] == 0 {
				delete(g.refs, id)
				removed = append(removed, id)
			}
		}
		reportStream(sub.stream, errSlowConsumer, true)
		close(sub.stream.messages)
		close(sub.stream.errors)
		close(sub.stream.replays)
		close(sub.stream.done)
	}
	if g.conn != nil && oldWildcard && g.wildcards == 0 && len(g.subs) > 0 {
		g.reconnectLocked()
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
	if g.conn != nil {
		if oldWildcard && len(g.subs) == 0 {
			g.queueOverflowUnsubscribeLocked(nil, true)
		} else if !oldWildcard && len(removed) > 0 {
			g.queueOverflowUnsubscribeLocked(removed, false)
		}
	}
	if len(g.subs) == 0 {
		g.unpublishLocked()
		if g.asyncConn == nil {
			g.disconnectLocked()
		}
	}
}

// Only this worker writes on its connection while asyncConn is set. A concurrent
// subscription change replaces the connection instead of waiting under g.mu.
func (g *connectionGroup) queueOverflowUnsubscribeLocked(ids []string, wildcard bool) {
	if g.asyncConn == nil {
		g.asyncConn = g.conn
		g.pendingUnsubs = make(map[string]struct{})
		go g.writeOverflowUnsubscribes(g.conn)
	}
	for _, id := range ids {
		g.pendingUnsubs[id] = struct{}{}
	}
	g.pendingWildcardUnsub = g.pendingWildcardUnsub || wildcard
}

func (g *connectionGroup) writeOverflowUnsubscribes(conn *websocket.Conn) {
	for {
		g.mu.Lock()
		if g.conn != conn || g.asyncConn != conn {
			g.mu.Unlock()
			return
		}
		ids := make([]string, 0, len(g.pendingUnsubs))
		for id := range g.pendingUnsubs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		wildcard := g.pendingWildcardUnsub
		g.pendingUnsubs = make(map[string]struct{})
		g.pendingWildcardUnsub = false
		g.mu.Unlock()

		req := SubscriptionRequest{Type: g.key.channel, Operation: "unsubscribe", Markets: []string{}, AssetIDs: []string{}}
		if g.key.channel == "market" {
			req.AssetIDs = ids
		} else {
			req.Auth = &g.key.credentials
			if !wildcard {
				req.Markets = ids
			}
		}
		data, _ := json.Marshal(req)
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		err := conn.WriteMessage(websocket.TextMessage, data)

		g.mu.Lock()
		if g.conn != conn || g.asyncConn != conn {
			g.mu.Unlock()
			return
		}
		if err != nil {
			g.disconnectLocked()
			g.reportLocked(fmt.Errorf("websocket write: %w", err), true)
			g.mu.Unlock()
			return
		}
		if len(g.pendingUnsubs) == 0 && !g.pendingWildcardUnsub {
			g.asyncConn = nil
			g.pendingUnsubs = nil
			if g.retired {
				g.disconnectLocked()
			}
			g.mu.Unlock()
			return
		}
		g.mu.Unlock()
	}
}

func (g *connectionGroup) retireLocked() {
	g.unpublishLocked()
	g.disconnectLocked()
}

func (g *connectionGroup) unpublishLocked() {
	g.retired = true
	g.client.mu.Lock()
	if g.client.groups[g.key] == g {
		delete(g.client.groups, g.key)
	}
	g.client.mu.Unlock()
	g.cancel()
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

// A stale connection may have been retired while its frame was being parsed.
func (g *connectionGroup) rejectStrictMarket(conn *websocket.Conn, cause error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != conn {
		return
	}
	g.strictMarketRejections++
	g.disconnectLocked()
	g.reportLocked(fmt.Errorf("%w: %v", ErrStrictMarketMessage, cause), true)
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
						sub.stream.setState(PhaseClosed, g.generation, g.interruptions, g.strictMarketRejections)
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
			connectionID := nextConnectionID.Add(1)
			conn.SetReadLimit(g.cfg.ReadLimit)
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
					sub.stream.setState(PhaseReplaySent, g.generation, g.interruptions, g.strictMarketRejections)
					select {
					case <-sub.stream.replays:
					default:
					}
					sub.stream.replays <- replay
				}
			}
			generation := g.generation
			g.mu.Unlock()
			if err == nil {
				err = g.read(conn, connectionID, generation)
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

func (g *connectionGroup) read(conn *websocket.Conn, connectionID, generation uint64) error {
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
			if g.conn == conn && g.asyncConn == conn {
				g.mu.Unlock()
				continue
			}
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
		kind, reader, err := conn.NextReader()
		if err != nil {
			return err
		}
		// Gorilla's limit applies before decompression. Also bound decompressed data.
		limit := g.cfg.ReadLimit
		if limit < math.MaxInt64 {
			limit++
		}
		data, err := io.ReadAll(io.LimitReader(reader, limit))
		receivedAt := time.Now()
		if err != nil {
			return err
		}
		if int64(len(data)) > g.cfg.ReadLimit {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseMessageTooBig, ""), time.Now().Add(time.Second))
			return websocket.ErrReadLimit
		}
		if kind != websocket.TextMessage {
			if g.key.channel == "market" && g.cfg.StrictMarket {
				err := errors.New("unexpected binary market message")
				g.rejectStrictMarket(conn, err)
				return err
			}
			continue
		}
		if string(data) == "PONG" {
			select {
			case pong <- struct{}{}:
			default:
			}
			continue
		}
		var messages []Message
		if g.key.channel == "market" && g.cfg.StrictMarket {
			messages, err = strictMarketMessages(data)
			if err != nil {
				g.rejectStrictMarket(conn, err)
				return err
			}
		} else {
			messages, err = ParseMessages(data)
			if err != nil {
				g.reportConn(conn, fmt.Errorf("websocket message: %w", err))
			}
		}
		for _, msg := range messages {
			msg.ConnectionID = connectionID
			msg.Generation = generation
			msg.ReceivedAt = receivedAt
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
			copyMsg := msg
			copyMsg.Raw = append(json.RawMessage(nil), msg.Raw...)
			// State readers must not observe an active stream after a failed send.
			sub.stream.stateMu.Lock()
			select {
			case sub.stream.messages <- copyMsg:
			default:
				sub.stream.setStateLocked(PhaseClosed, g.generation, g.interruptions, g.strictMarketRejections)
				slow = append(slow, sub)
			}
			sub.stream.stateMu.Unlock()
		}
	}
	if len(slow) > 0 {
		g.removeSlowLocked(slow)
	}
	g.mu.Unlock()
}
