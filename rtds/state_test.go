package rtds

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

func awaitRTDSState[T any](t *testing.T, stream *Stream[T], phase StreamPhase, generation, interruptions uint64) StreamState {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		state := stream.State()
		if state.Phase == phase && state.Generation == generation && state.Interruptions == interruptions {
			return state
		}
		select {
		case _, ok := <-stream.StateChanges:
			if !ok {
				t.Fatalf("state channel closed at %+v", stream.State())
			}
		case <-timer.C:
			t.Fatalf("state = %+v, want %s generation %d interruptions %d", stream.State(), phase, generation, interruptions)
		}
	}
}

func TestStreamStateReconnectLateJoinAndCoalescing(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	var attempts atomic.Int32
	started := make(chan int, 2)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	var onceFirst, onceSecond sync.Once
	releaseFirst := func() { onceFirst.Do(func() { close(firstRelease) }) }
	releaseSecond := func() { onceSecond.Do(func() { close(secondRelease) }) }
	defer releaseFirst()
	defer releaseSecond()
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		attempt := int(attempts.Add(1))
		var release <-chan struct{}
		switch attempt {
		case 1:
			release = firstRelease
		case 2:
			release = secondRelease
		}
		if release != nil {
			started <- attempt
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	client := server.client(t, Config{Dialer: &dialer, InitialBackoff: time.Millisecond})
	raw, err := client.SubscribeCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if receive(t, started) != 1 {
		t.Fatal("wrong dial")
	}
	initial := raw.State()
	if initial != (StreamState{Phase: PhaseDisconnected, Version: 1}) {
		t.Fatalf("initial state: %+v", initial)
	}
	releaseFirst()
	first := server.event(t, "subscribe", "crypto_prices")
	ready := awaitRTDSState(t, raw, PhaseReady, 1, 0)
	if ready.Version <= initial.Version {
		t.Fatal("state version did not advance")
	}
	typed, err := client.SubscribeTypedCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer typed.Close()
	if got := typed.State(); got.Phase != PhaseReady || got.Generation != 1 || got.Interruptions != 0 || got.Version != 1 {
		t.Fatalf("late ready join: %+v", got)
	}
	if err := first.conn.Close(); err != nil {
		t.Fatal(err)
	}
	if receive(t, started) != 2 {
		t.Fatal("wrong reconnect dial")
	}
	interrupted := awaitRTDSState(t, raw, PhaseDisconnected, 1, 1)
	awaitRTDSState(t, typed, PhaseDisconnected, 1, 1)
	if interrupted.Version <= ready.Version {
		t.Fatal("interruption did not advance version")
	}
	late, err := client.SubscribeTypedComments(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	if got := late.State(); got != (StreamState{Phase: PhaseDisconnected, Version: 1, Generation: 1, Interruptions: 1}) {
		t.Fatalf("late disconnected join: %+v", got)
	}
	releaseSecond()
	second := server.event(t, "subscribe", "crypto_prices")
	server.event(t, "subscribe", "comments")
	awaitRTDSState(t, raw, PhaseReady, 2, 1)
	awaitRTDSState(t, typed, PhaseReady, 2, 1)
	if err := second.conn.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "subscribe", "crypto_prices")
	server.event(t, "subscribe", "comments")
	// The ordered writer publishes the comments state before writing this barrier.
	barrier, err := client.Subscribe(context.Background(), Subscription{Topic: "barrier", Type: "*"})
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Close()
	server.event(t, "subscribe", "barrier")
	latest := late.State()
	if latest.Phase != PhaseReady || latest.Generation != 3 || latest.Interruptions != 2 || latest.Version < 5 {
		t.Fatalf("coalesced snapshot: %+v", latest)
	}
	select {
	case _, ok := <-late.StateChanges:
		if !ok {
			t.Fatal("closed too early")
		}
	default:
		t.Fatal("missing coalesced wakeup")
	}
	select {
	case <-late.StateChanges:
		t.Fatal("state wakeups did not coalesce")
	default:
	}
	if err := late.Close(); err != nil {
		t.Fatal(err)
	}
	closed := late.State()
	if closed.Phase != PhaseClosed || closed.Version <= latest.Version || closed.Generation != 3 || closed.Interruptions != 2 {
		t.Fatalf("closed snapshot: %+v", closed)
	}
	select {
	case _, ok := <-late.StateChanges:
		if !ok {
			t.Fatal("missing final wakeup")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing final wakeup")
	}
	select {
	case _, ok := <-late.StateChanges:
		if ok {
			t.Fatal("state channel remains open")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("state channel did not close")
	}
}

func TestStreamStateWriteFailureRetiresWhileDecodeBlocked(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{InitialBackoff: time.Millisecond})
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	slow, err := subscribe(client, context.Background(), Subscription{Topic: "slow", Type: "*"}, func(msg Message) (Message, bool) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		return msg, true
	})
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	defer unblock()
	first := server.event(t, "subscribe", "slow")
	awaitRTDSState(t, slow, PhaseReady, 1, 0)
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"slow","type":"x","timestamp":1,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("decode did not block")
	}
	// Closing the client socket and then writing forces a locally observed writer
	// failure while the reader is still inside the decoder.
	client.mu.Lock()
	conn := slow.group.session.conn
	client.mu.Unlock()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	anchor, err := client.Subscribe(context.Background(), Subscription{Topic: "anchor", Type: "*"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = anchor.Close() }()
	awaitRTDSState(t, slow, PhaseDisconnected, 1, 1)
	late, err := client.Subscribe(context.Background(), Subscription{Topic: "slow", Type: "*"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = late.Close() }()
	if got := late.State(); got.Phase != PhaseDisconnected || got.Generation != 1 || got.Interruptions != 1 {
		t.Fatalf("late join to retired socket: %+v", got)
	}
	unblock()
	replay := server.event(t, "subscribe", "slow")
	server.event(t, "subscribe", "anchor")
	awaitRTDSState(t, late, PhaseReady, 2, 1)
	if err := replay.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"slow","type":"x","timestamp":2,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, slow.Messages); got.Timestamp != 2 {
		t.Fatalf("retired socket delivered stale message: %+v", got)
	}
	if got := receive(t, late.Messages); got.Timestamp != 2 {
		t.Fatalf("late stream received stale message: %+v", got)
	}
}

func TestStreamStateFailedWriteDoesNotBecomeReady(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	wrapped := make(chan *blockedWriteConn, 1)
	var dials atomic.Int32
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil || dials.Add(1) != 1 {
			return conn, err
		}
		blocked := &blockedWriteConn{Conn: conn, entered: make(chan struct{}), release: make(chan struct{})}
		wrapped <- blocked
		return blocked, nil
	}
	client := server.client(t, Config{Dialer: &dialer, InitialBackoff: time.Millisecond})
	anchor, err := client.SubscribeCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	first := server.event(t, "subscribe", "crypto_prices")
	awaitRTDSState(t, anchor, PhaseReady, 1, 0)
	blocked := receive(t, wrapped)
	var once sync.Once
	unblock := func() { once.Do(func() { close(blocked.release) }) }
	defer unblock()
	blocked.block.Store(true)
	pending, err := client.SubscribeComments(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = pending.Close() }()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("write did not block")
	}
	if got := pending.State(); got.Phase != PhaseReplaying || got.Generation != 1 {
		t.Fatalf("unexpected pending state: %+v", got)
	}
	if err := first.conn.Close(); err != nil {
		t.Fatal(err)
	}
	awaitRTDSState(t, pending, PhaseDisconnected, 1, 1)
	unblock()
	server.event(t, "subscribe", "crypto_prices")
	server.event(t, "subscribe", "comments")
	awaitRTDSState(t, pending, PhaseReady, 2, 1)
}

func TestStreamStateFailedDialAndConcurrentCancel(t *testing.T) {
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("test dial failure") }
	client, err := NewClient("ws://127.0.0.1:1", Config{Dialer: &dialer, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.SubscribeCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	select {
	case <-stream.done:
	case <-time.After(3 * time.Second):
		t.Fatal("failed stream did not close")
	}
	if got := stream.State(); got != (StreamState{Phase: PhaseClosed, Version: 2}) {
		t.Fatalf("failed dial state: %+v", got)
	}
	for range stream.StateChanges {
	}

	entered := make(chan struct{})
	dialer.NetDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	client, err = NewClient("ws://127.0.0.1:1", Config{Dialer: &dialer})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	typed, err := client.SubscribeTypedCryptoPrices(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer typed.Close()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("dial did not start")
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = typed.State()
				}
			}
		}()
	}
	cancel()
	select {
	case <-typed.done:
	case <-time.After(3 * time.Second):
		close(stop)
		wg.Wait()
		t.Fatal("canceled stream did not close")
	}
	close(stop)
	wg.Wait()
	if got := typed.State(); got.Phase != PhaseClosed || got.Generation != 0 || got.Interruptions != 0 {
		t.Fatalf("canceled state: %+v", got)
	}
}

func TestStreamStateWaitsForOwnWriteAndStaysClosed(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	wrapped := make(chan *blockedWriteConn, 1)
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		blocked := &blockedWriteConn{Conn: conn, entered: make(chan struct{}), release: make(chan struct{})}
		wrapped <- blocked
		return blocked, nil
	}
	client := server.client(t, Config{Dialer: &dialer})
	anchor, err := client.SubscribeCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	server.event(t, "subscribe", "crypto_prices")
	awaitRTDSState(t, anchor, PhaseReady, 1, 0)
	blocked := receive(t, wrapped)
	var once sync.Once
	unblock := func() { once.Do(func() { close(blocked.release) }) }
	defer unblock()
	blocked.block.Store(true)
	raw, err := client.SubscribeComments(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = raw.Close() }()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription write did not block")
	}
	if state := raw.State(); state.Phase != PhaseReplaying || state.Generation != 1 {
		t.Fatalf("premature readiness: %+v", state)
	}
	typed, err := client.SubscribeTypedComments(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = typed.Close() }()
	if state := typed.State(); state.Phase != PhaseReplaying || state.Generation != 1 {
		t.Fatalf("pending duplicate state: %+v", state)
	}
	if err := typed.Close(); err != nil {
		t.Fatal(err)
	}
	closed := typed.State()
	if closed.Phase != PhaseClosed {
		t.Fatalf("typed stream not closed: %+v", closed)
	}
	unblock()
	server.event(t, "subscribe", "comments")
	awaitRTDSState(t, raw, PhaseReady, 1, 0)
	if got := typed.State(); got != closed {
		t.Fatalf("closed state changed after write: %+v", got)
	}
}
