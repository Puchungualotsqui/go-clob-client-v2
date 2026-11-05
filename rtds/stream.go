package rtds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
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
// a full buffer terminates the stream rather than silently losing data.
// StateChanges is a coalescing wakeup for the authoritative local State snapshot.
type Stream[T any] struct {
	Messages     <-chan T
	Errors       <-chan error
	StateChanges <-chan struct{}
	messages     chan T
	errors       chan error
	stateChanges chan struct{}
	state        StreamState
	done         chan struct{}
	client       *Client
	group        *connection
	entry        *subscriptionEntry
	member       *streamMember
	closed       bool // protected by client.mu
}

// StreamPhase describes the locally observed lifecycle of a subscription.
type StreamPhase string

const (
	PhaseDisconnected StreamPhase = "disconnected"
	PhaseReplaying    StreamPhase = "replaying"
	PhaseReady        StreamPhase = "ready"
	PhaseClosed       StreamPhase = "closed"
)

// StreamState is a versioned local snapshot. Generation counts established
// sockets in the shared group; Interruptions counts retired sockets that still
// had streams. Ready only means this subscription write succeeded locally.
type StreamState struct {
	Phase         StreamPhase
	Version       uint64
	Generation    uint64
	Interruptions uint64
}

// State returns the current snapshot, including after closure. StateChanges is a
// coalescing wakeup; read State after receiving it. Only one consumer should
// drain StateChanges. Neither state nor notifications acknowledge server acceptance.
func (s *Stream[T]) State() StreamState {
	s.client.mu.Lock()
	defer s.client.mu.Unlock()
	return s.state
}

// setState is called with client.mu held.
func (s *Stream[T]) setState(phase StreamPhase, generation, interruptions uint64) {
	if s.state.Version != 0 && s.state.Phase == phase && s.state.Generation == generation && s.state.Interruptions == interruptions {
		return
	}
	s.state = StreamState{Phase: phase, Version: s.state.Version + 1, Generation: generation, Interruptions: interruptions}
	select {
	case s.stateChanges <- struct{}{}:
	default:
	}
	if phase == PhaseClosed {
		close(s.stateChanges)
	}
}

type authIdentity struct {
	present     bool
	credentials Credentials
}

type streamMember struct {
	active      atomic.Bool
	prepare     func(Message) func() bool
	report      func(error)
	finish      func()
	setState    func(StreamPhase, uint64, uint64)
	failedDials int
}

type subscriptionEntry struct {
	topic, eventType  string
	wire, unsubscribe []byte
	members           map[*streamMember]struct{}
	readyGeneration   uint64
}

type connection struct {
	client        *Client
	cfg           Config
	auth          authIdentity
	ctx           context.Context
	cancel        context.CancelFunc
	session       *socketSession                // guarded by client.mu, as are entries and pending writes
	entries       map[string]*subscriptionEntry // one identity per topic
	retired       map[string]struct{}
	order         []*subscriptionEntry
	stopping      bool
	rotating      bool
	generation    uint64
	interruptions uint64
}

type socketSession struct {
	conn    *websocket.Conn
	pending []*socketWrite
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	retired bool
}

type socketWrite struct {
	data   []byte
	result chan error
	final  bool
	rotate bool
	entry  *subscriptionEntry
}

func newStream[T any](c *Client, ctx context.Context, topic, eventType string, auth authIdentity, wire, unsubscribe []byte, decode func(Message) (T, bool)) *Stream[T] {
	cfg := c.config.defaults()
	messages := make(chan T, cfg.BufferSize)
	errs := make(chan error, 32)
	stateChanges := make(chan struct{}, 1)
	s := &Stream[T]{Messages: messages, Errors: errs, StateChanges: stateChanges, messages: messages, errors: errs, stateChanges: stateChanges, done: make(chan struct{}), client: c}
	m := &streamMember{}
	m.active.Store(true)
	m.prepare = func(msg Message) func() bool {
		if eventType != "*" && msg.Type != eventType {
			return nil
		}
		// RawMessage is mutable; receivers must not share its backing storage.
		msg.Payload = append([]byte(nil), msg.Payload...)
		value, ok := decode(msg)
		if !ok {
			return nil
		}
		return func() bool {
			select {
			case messages <- value:
				return true
			default:
				return false
			}
		}
	}
	m.report = func(err error) {
		select {
		case errs <- err:
		default:
		}
	}
	m.setState = s.setState
	m.finish = func() {
		s.setState(PhaseClosed, s.state.Generation, s.state.Interruptions)
		m.active.Store(false)
		s.closed = true
		s.group, s.entry, s.member = nil, nil, nil
		close(messages)
		close(errs)
		close(s.done)
	}
	s.member = m

	c.mu.Lock()
	if c.groups == nil {
		c.groups = make(map[*connection]struct{})
	}
	var g *connection
	for candidate := range c.groups {
		if candidate.auth != auth || candidate.rotating {
			continue
		}
		if _, retired := candidate.retired[topic]; retired {
			continue
		}
		e := candidate.entries[topic]
		if e != nil && string(e.wire) == string(wire) {
			g = candidate
			break
		}
		if e == nil && g == nil {
			g = candidate
		}
	}
	if g == nil {
		groupCtx, cancel := context.WithCancel(context.Background())
		g = &connection{client: c, cfg: cfg, auth: auth, ctx: groupCtx, cancel: cancel, entries: make(map[string]*subscriptionEntry), retired: make(map[string]struct{})}
		c.groups[g] = struct{}{}
		go g.run()
	}
	e := g.entries[topic]
	if e == nil {
		e = &subscriptionEntry{topic: topic, eventType: eventType, wire: wire, unsubscribe: unsubscribe, members: make(map[*streamMember]struct{})}
		g.entries[topic] = e
		g.order = append(g.order, e)
		if g.session != nil && !g.session.retired {
			g.enqueue(&socketWrite{data: wire, entry: e})
		}
	}
	e.members[m] = struct{}{}
	s.group, s.entry = g, e
	phase := PhaseDisconnected
	if g.session != nil && !g.session.retired {
		phase = PhaseReplaying
		if e.readyGeneration == g.generation {
			phase = PhaseReady
		}
	}
	s.setState(phase, g.generation, g.interruptions)
	c.mu.Unlock()
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s
}

// Close releases this stream. The final reference sends an unsubscribe when
// connected; other streams and their shared connection continue unaffected.
// It is safe to call more than once.
func (s *Stream[T]) Close() error {
	s.client.mu.Lock()
	if s.closed {
		s.client.mu.Unlock()
		return nil
	}
	result := s.group.remove(s.entry, s.member)
	s.client.mu.Unlock()
	if result != nil {
		return <-result
	}
	return nil
}

// State and the write queue are protected by client.mu; socket I/O never holds it.
func (g *connection) enqueue(write *socketWrite) {
	g.session.pending = append(g.session.pending, write)
	select {
	case g.session.wake <- struct{}{}:
	default:
	}
}

func (g *connection) remove(e *subscriptionEntry, m *streamMember) <-chan error {
	delete(e.members, m)
	m.finish()
	if len(e.members) != 0 {
		return nil
	}
	delete(g.entries, e.topic)
	// In-flight messages on this socket cannot be attributed to a new subscription.
	rotate := false
	if g.session != nil && !g.rotating {
		g.retired[e.topic] = struct{}{}
		if len(g.retired) >= maxRetiredTopics {
			// Stop admitting streams and replace the socket before retirement grows without bound.
			g.rotating = true
			clear(g.retired)
			rotate = true
		}
	}
	for i, item := range g.order {
		if item == e {
			copy(g.order[i:], g.order[i+1:])
			g.order[len(g.order)-1] = nil
			g.order = g.order[:len(g.order)-1]
			break
		}
	}
	final := len(g.entries) == 0
	var result chan error
	if g.session != nil && !g.session.retired {
		result = make(chan error, 1)
		g.enqueue(&socketWrite{data: e.unsubscribe, result: result, final: final, rotate: rotate})
	}
	if final {
		delete(g.client.groups, g)
		g.stopping = true
		if g.session == nil || g.session.retired {
			g.cancel()
		}
	}
	return result
}

const maxRetiredTopics = 128

func (g *connection) report(err error) {
	for _, e := range g.entries {
		for m := range e.members {
			m.report(err)
		}
	}
}

var (
	errSlowConsumer = errors.New("websocket message buffer full; stream stopped")
	errWriteNotSent = errors.New("websocket connection closed before write")
)

// retire records a socket interruption and resolves queued writes exactly once,
// while client.mu is held. The writer exclusively owns any write it has dequeued.
func (g *connection) retire(session *socketSession) {
	if g.session != session || session.retired {
		return
	}
	session.retired = true
	for _, pending := range session.pending {
		if pending.result != nil {
			pending.result <- errWriteNotSent
		}
	}
	session.pending = nil
	if g.stopping {
		g.cancel()
	}
	if len(g.entries) == 0 {
		return
	}
	g.interruptions++
	for _, e := range g.entries {
		for m := range e.members {
			m.setState(PhaseDisconnected, g.generation, g.interruptions)
		}
	}
}

func (g *connection) dispatch(session *socketSession, data []byte) {
	messages, err := ParseMessages(data)
	g.client.mu.Lock()
	if g.session != session || session.retired {
		g.client.mu.Unlock()
		return
	}
	if err != nil {
		g.report(fmt.Errorf("websocket message: %w", err))
		g.client.mu.Unlock()
		return
	}
	// Capture membership once per frame: a late joiner must not receive an
	// already-read frame, even when decoding another member takes time.
	type targets struct {
		entry   *subscriptionEntry
		members []*streamMember
	}
	byTopic := make(map[string]targets)
	for _, msg := range messages {
		if _, seen := byTopic[msg.Topic]; seen {
			continue
		}
		e := g.entries[msg.Topic]
		var members []*streamMember
		if e != nil {
			members = make([]*streamMember, 0, len(e.members))
			for m := range e.members {
				members = append(members, m)
			}
		}
		byTopic[msg.Topic] = targets{entry: e, members: members}
	}
	g.client.mu.Unlock()
	for _, msg := range messages {
		target := byTopic[msg.Topic]
		for _, m := range target.members {
			if !m.active.Load() {
				continue
			}
			// Copy and decode without holding the lock used by other connections.
			deliver := m.prepare(msg)
			if deliver == nil {
				continue
			}
			g.client.mu.Lock()
			if _, active := target.entry.members[m]; active && g.session == session && !session.retired && !deliver() {
				m.report(errSlowConsumer)
				g.remove(target.entry, m)
			}
			g.client.mu.Unlock()
		}
	}
}

func (g *connection) run() {
	backoff := g.cfg.InitialBackoff
	for g.ctx.Err() == nil {
		conn, response, err := g.cfg.Dialer.DialContext(g.ctx, g.client.endpoint, g.cfg.Header)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			g.client.mu.Lock()
			if g.ctx.Err() != nil {
				g.client.mu.Unlock()
				return
			}
			for _, e := range g.entries {
				for m := range e.members {
					m.report(fmt.Errorf("websocket dial: %w", err))
					m.failedDials++
					if g.cfg.MaxAttempts > 0 && m.failedDials >= g.cfg.MaxAttempts {
						g.remove(e, m)
					}
				}
			}
			stopped := g.ctx.Err() != nil
			g.client.mu.Unlock()
			if stopped {
				return
			}
		} else {
			backoff = g.cfg.InitialBackoff
			g.client.mu.Lock()
			if g.ctx.Err() != nil {
				g.client.mu.Unlock()
				_ = conn.Close()
				return
			}
			session := &socketSession{conn: conn, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
			g.session = session
			g.generation++
			for _, e := range g.order {
				for m := range e.members {
					m.failedDials = 0
					m.setState(PhaseReplaying, g.generation, g.interruptions)
				}
				g.enqueue(&socketWrite{data: e.wire, entry: e})
			}
			go g.writer(session)
			g.client.mu.Unlock()
			err = g.read(session)
			g.client.mu.Lock()
			g.retire(session)
			g.session = nil
			// read has returned, so no more old-socket messages can be dispatched.
			clear(g.retired)
			g.rotating = false
			close(session.stop)
			if g.stopping {
				g.cancel()
			}
			stopped := g.ctx.Err() != nil
			if !stopped && err != nil {
				g.report(fmt.Errorf("websocket connection: %w", err))
			}
			g.client.mu.Unlock()
			_ = conn.Close()
			<-session.done
			if stopped {
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

func (g *connection) writer(session *socketSession) {
	defer close(session.done)
	for {
		g.client.mu.Lock()
		if g.session != session || session.retired {
			g.client.mu.Unlock()
			return
		}
		if len(session.pending) == 0 {
			g.client.mu.Unlock()
			select {
			case <-session.wake:
			case <-session.stop:
				return
			}
			continue
		}
		write := session.pending[0]
		session.pending[0] = nil
		session.pending = session.pending[1:]
		g.client.mu.Unlock()
		_ = session.conn.SetWriteDeadline(time.Now().Add(time.Second))
		err := session.conn.WriteMessage(websocket.TextMessage, write.data)
		g.client.mu.Lock()
		if err == nil && write.entry != nil && g.session == session && !session.retired && g.entries[write.entry.topic] == write.entry {
			write.entry.readyGeneration = g.generation
			for m := range write.entry.members {
				m.setState(PhaseReady, g.generation, g.interruptions)
			}
		}
		if err != nil || write.final || write.rotate {
			g.retire(session)
		}
		g.client.mu.Unlock()
		if write.final {
			g.cancel()
		}
		if err != nil || write.final || write.rotate {
			_ = session.conn.Close()
		}
		if write.result != nil {
			write.result <- err
		}
		if err != nil || write.final || write.rotate {
			return
		}
	}
}

func (g *connection) read(session *socketSession) error {
	conn := session.conn
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
				_ = conn.Close()
				return
			case <-stop:
				return
			case <-ticker.C:
			}
			select {
			case <-pong:
			default:
			}
			g.client.mu.Lock()
			if g.session != session || session.retired || g.stopping {
				g.client.mu.Unlock()
				return
			}
			result := make(chan error, 1)
			g.enqueue(&socketWrite{data: []byte("PING"), result: result})
			g.client.mu.Unlock()
			select {
			case err := <-result:
				if err != nil {
					_ = conn.Close()
					return
				}
			case <-g.ctx.Done():
				return
			case <-stop:
				return
			}
			timer := time.NewTimer(g.cfg.HeartbeatTimeout)
			select {
			case <-pong:
				timer.Stop()
			case <-timer.C:
				g.client.mu.Lock()
				g.retire(session)
				g.report(errors.New("websocket heartbeat timeout"))
				g.client.mu.Unlock()
				_ = conn.Close()
				return
			case <-g.ctx.Done():
				timer.Stop()
				_ = conn.Close()
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
		g.dispatch(session, data)
	}
}
