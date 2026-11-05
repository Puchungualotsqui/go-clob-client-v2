package rtds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type socketEvent struct {
	id      int32
	conn    *websocket.Conn
	request SubscriptionRequest
}

type socketServer struct {
	server *httptest.Server
	events chan socketEvent
	next   atomic.Int32
}

func newSocketServer(t *testing.T) *socketServer {
	t.Helper()
	s := &socketServer{events: make(chan socketEvent, 2048)}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		id := s.next.Add(1)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(data) == "PING" {
				continue
			}
			var req SubscriptionRequest
			if json.Unmarshal(data, &req) == nil {
				s.events <- socketEvent{id, conn, req}
			}
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *socketServer) client(t *testing.T, cfg Config) *Client {
	t.Helper()
	c, err := NewClient("ws"+strings.TrimPrefix(s.server.URL, "http"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (s *socketServer) event(t *testing.T, action, topic string) socketEvent {
	t.Helper()
	select {
	case ev := <-s.events:
		if ev.request.Action != action || len(ev.request.Subscriptions) != 1 || ev.request.Subscriptions[0].Topic != topic {
			t.Fatalf("unexpected request on connection %d: %+v; want %s %s", ev.id, ev.request, action, topic)
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatalf("missing %s %s", action, topic)
		return socketEvent{}
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("missing message")
		var zero T
		return zero
	}
}

func TestMultiplexFanoutRefcountsAndReplay(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{InitialBackoff: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	raw, _ := client.SubscribeCryptoPrices(ctx, []string{"btc"})
	defer raw.Close()
	first := server.event(t, "subscribe", "crypto_prices")
	typed, _ := client.SubscribeTypedCryptoPrices(context.Background(), []string{"btc"})
	defer typed.Close()
	other, _ := client.SubscribeComments(context.Background(), CommentCreated, nil)
	defer other.Close()
	event := server.event(t, "subscribe", "comments")
	if first.id != event.id {
		t.Fatal("compatible subscriptions did not share a connection")
	}
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`[{"topic":"crypto_prices","type":"update","timestamp":1,"payload":{"symbol":"btc","timestamp":2,"value":42}},{"topic":"comments","type":"comment_created","timestamp":1,"payload":{"id":"one"}}]`)); err != nil {
		t.Fatal(err)
	}
	price := receive(t, typed.Messages)
	if price.Symbol != "btc" || price.Value.String() != "42" {
		t.Fatalf("bad typed price: %+v", price)
	}
	if receive(t, raw.Messages).Topic != "crypto_prices" || receive(t, other.Messages).Topic != "comments" {
		t.Fatal("misrouted message")
	}
	cancel()
	select {
	case <-raw.done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop stream")
	}
	// The next request is a barrier proving cancellation did not unsubscribe the shared topic.
	barrier, _ := client.Subscribe(context.Background(), Subscription{Topic: "barrier", Type: "update"})
	defer barrier.Close()
	if ev := server.event(t, "subscribe", "barrier"); ev.id != first.id {
		t.Fatal("barrier not shared")
	}
	if err := first.conn.Close(); err != nil {
		t.Fatal(err)
	}
	for _, topic := range []string{"crypto_prices", "comments", "barrier"} {
		ev := server.event(t, "subscribe", topic)
		if ev.id == first.id {
			t.Fatal("replay did not reconnect")
		}
	}
	if err := typed.Close(); err != nil {
		t.Fatal(err)
	}
	ev := server.event(t, "unsubscribe", "crypto_prices")
	if ev.request.Subscriptions[0].Filters != nil || ev.request.Subscriptions[0].ClobAuth != nil {
		t.Fatal("unsubscribe included filter or credentials")
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "comments")
}

func TestMultiplexIsolationAndSnapshot(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{InitialBackoff: time.Millisecond})
	filter := `["btc"]`
	auth := &Credentials{Key: "key", Secret: "secret", Passphrase: "pass"}
	sub := Subscription{Topic: "prices", Type: "update", Filters: &filter, ClobAuth: auth}
	one, _ := client.Subscribe(context.Background(), sub)
	defer one.Close()
	first := server.event(t, "subscribe", "prices")
	filter = `["eth"]`
	two, _ := client.Subscribe(context.Background(), sub)
	defer two.Close()
	second := server.event(t, "subscribe", "prices")
	if first.id == second.id {
		t.Fatal("different filters shared a connection")
	}
	btc := `["btc"]`
	duplicate, _ := client.Subscribe(context.Background(), Subscription{Topic: "prices", Type: "update", Filters: &btc, ClobAuth: auth})
	defer duplicate.Close()
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"prices","type":"update","timestamp":1,"payload":{"symbol":"btc"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := second.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"prices","type":"update","timestamp":2,"payload":{"symbol":"eth"}}`)); err != nil {
		t.Fatal(err)
	}
	firstMsg := receive(t, one.Messages)
	duplicateMsg := receive(t, duplicate.Messages)
	secondMsg := receive(t, two.Messages)
	if firstMsg.Timestamp != 1 || duplicateMsg.Timestamp != 1 || secondMsg.Timestamp != 2 {
		t.Fatal("filter-isolated streams received the wrong message")
	}
	firstMsg.Payload[0] = 'x'
	if duplicateMsg.Payload[0] != '{' {
		t.Fatal("raw streams share mutable payload storage")
	}
	// Credentials are snapshotted; later caller mutations cannot affect replay.
	auth.Secret = "changed"
	third, _ := client.Subscribe(context.Background(), Subscription{Topic: "comments", Type: "*", ClobAuth: auth})
	defer third.Close()
	thirdEvent := server.event(t, "subscribe", "comments")
	if thirdEvent.id == first.id || thirdEvent.id == second.id {
		t.Fatal("different credentials shared a connection")
	}
	plain, _ := client.Subscribe(context.Background(), Subscription{Topic: "comments", Type: "*"})
	defer plain.Close()
	plainEvent := server.event(t, "subscribe", "comments")
	if plainEvent.id == thirdEvent.id {
		t.Fatal("authenticated and unauthenticated streams shared a connection")
	}
	if err := one.Close(); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	unsub := server.event(t, "unsubscribe", "prices")
	if unsub.id != first.id {
		t.Fatal("unsubscribe affected the wrong connection")
	}
	if err := second.conn.Close(); err != nil {
		t.Fatal(err)
	}
	replay := server.event(t, "subscribe", "prices")
	got := replay.request.Subscriptions[0]
	if got.Filters == nil || *got.Filters != `["eth"]` || got.ClobAuth == nil || got.ClobAuth.Secret != "secret" {
		t.Fatal("replay did not preserve subscription snapshot")
	}
}

func TestMultiplexTopicReuseAndTypeIsolation(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{})
	all, _ := client.Subscribe(context.Background(), Subscription{Topic: "events", Type: "*"})
	defer all.Close()
	first := server.event(t, "subscribe", "events")
	anchor, _ := client.Subscribe(context.Background(), Subscription{Topic: "anchor", Type: "*"})
	defer anchor.Close()
	if server.event(t, "subscribe", "anchor").id != first.id {
		t.Fatal("anchor did not share the first connection")
	}
	specific, _ := client.Subscribe(context.Background(), Subscription{Topic: "events", Type: "created"})
	defer specific.Close()
	second := server.event(t, "subscribe", "events")
	if first.id == second.id {
		t.Fatal("wildcard and specific event types shared a connection")
	}
	if err := all.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "events")

	replacement, _ := client.Subscribe(context.Background(), Subscription{Topic: "events", Type: "removed"})
	defer replacement.Close()
	ev := server.event(t, "subscribe", "events")
	if ev.id == first.id || ev.id == second.id {
		t.Fatal("replacement reused an unsafe connection")
	}
}

func TestMultiplexNoReplayAfterCancellation(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	var attempts atomic.Int32
	reconnecting := make(chan struct{})
	release := make(chan struct{})
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if attempts.Add(1) == 2 {
			close(reconnecting)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	client := server.client(t, Config{InitialBackoff: time.Millisecond, Dialer: &dialer})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	removed, _ := client.Subscribe(ctx, Subscription{Topic: "removed", Type: "*"})
	defer removed.Close()
	first := server.event(t, "subscribe", "removed")
	kept, _ := client.Subscribe(context.Background(), Subscription{Topic: "kept", Type: "*"})
	defer kept.Close()
	server.event(t, "subscribe", "kept")
	if err := first.conn.Close(); err != nil {
		t.Fatal(err)
	}
	// Hold the reconnect dial until the removed subscription is gone.
	select {
	case <-reconnecting:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect did not start")
	}
	cancel()
	select {
	case <-removed.done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not complete")
	}
	close(release)
	replay := server.event(t, "subscribe", "kept")
	if replay.id == first.id {
		t.Fatal("expected a new connection")
	}
	barrier, _ := client.Subscribe(context.Background(), Subscription{Topic: "barrier", Type: "*"})
	defer barrier.Close()
	server.event(t, "subscribe", "barrier")
}

type blockedWriteConn struct {
	net.Conn
	block   atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockedWriteConn) Write(data []byte) (int, error) {
	if c.block.Load() {
		c.once.Do(func() { close(c.entered) })
		<-c.release
	}
	return c.Conn.Write(data)
}

func TestBlockedDecodeDoesNotBlockOtherConnectionsOrClose(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{})
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
	anchor, err := client.Subscribe(context.Background(), Subscription{Topic: "anchor", Type: "*"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = anchor.Close() }()
	server.event(t, "subscribe", "anchor")
	batch := `[{"topic":"slow","type":"x","timestamp":1,"payload":{}},{"topic":"slow","type":"x","timestamp":2,"payload":{}}]`
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(batch)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("decode did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	otherResult := make(chan *Stream[Message], 1)
	go func() {
		s, _ := client.Subscribe(ctx, Subscription{Topic: "other", Type: "*", ClobAuth: &Credentials{Key: "other"}})
		otherResult <- s
	}()
	other := receive(t, otherResult)
	defer func() { unblock(); _ = other.Close() }()
	otherEvent := server.event(t, "subscribe", "other")
	if err := otherEvent.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"other","type":"x","timestamp":3,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, other.Messages); got.Timestamp != 3 {
		t.Fatal("wrong independent message")
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "other")

	late, err := client.Subscribe(context.Background(), Subscription{Topic: "slow", Type: "*"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { unblock(); _ = late.Close() }()
	closed := make(chan error, 1)
	go func() { closed <- slow.Close() }()
	if err := receive(t, closed); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-slow.Messages:
		if ok {
			t.Fatal("closed stream received blocked decode")
		}
	default:
		t.Fatal("stream did not close")
	}
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"anchor","type":"x","timestamp":4,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	unblock()
	if got := receive(t, anchor.Messages); got.Timestamp != 4 {
		t.Fatal("wrong barrier message")
	}
	select {
	case msg := <-late.Messages:
		t.Fatalf("late stream received old frame: %+v", msg)
	default:
	}
	if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"slow","type":"x","timestamp":5,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, late.Messages); got.Timestamp != 5 {
		t.Fatal("late stream missed new message")
	}
}

func TestBlockedSocketDoesNotBlockOtherConnections(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	var count atomic.Int32
	wrapped := make(chan *blockedWriteConn, 1)
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil || count.Add(1) != 1 {
			return conn, err
		}
		blocked := &blockedWriteConn{Conn: conn, entered: make(chan struct{}), release: make(chan struct{})}
		wrapped <- blocked
		return blocked, nil
	}
	client := server.client(t, Config{Dialer: &dialer})
	first, _ := client.Subscribe(context.Background(), Subscription{Topic: "first", Type: "*"})
	defer first.Close()
	server.event(t, "subscribe", "first")
	blocked := receive(t, wrapped)
	var release sync.Once
	unblock := func() { release.Do(func() { close(blocked.release) }) }
	defer unblock()
	blocked.block.Store(true)
	slowResult := make(chan *Stream[Message], 1)
	go func() {
		s, _ := client.Subscribe(context.Background(), Subscription{Topic: "slow", Type: "*"})
		slowResult <- s
	}()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("socket write did not block")
	}
	independent := make(chan *Stream[Message], 1)
	go func() {
		s, _ := client.Subscribe(context.Background(), Subscription{Topic: "other", Type: "*", ClobAuth: &Credentials{Key: "other"}})
		independent <- s
	}()
	other := receive(t, independent)
	defer other.Close()
	ev := server.event(t, "subscribe", "other")
	if err := ev.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"other","type":"x","timestamp":1,"payload":{}}`)); err != nil {
		t.Fatal(err)
	}
	receive(t, other.Messages)
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "other")
	unblock()
	slow := receive(t, slowResult)
	defer slow.Close()
	server.event(t, "subscribe", "slow")
}

func TestRetiredTopicsResetOnReconnect(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{InitialBackoff: time.Millisecond})
	retired, _ := client.Subscribe(context.Background(), Subscription{Topic: "reused", Type: "old"})
	defer retired.Close()
	first := server.event(t, "subscribe", "reused")
	anchor, _ := client.Subscribe(context.Background(), Subscription{Topic: "anchor", Type: "*"})
	defer anchor.Close()
	server.event(t, "subscribe", "anchor")
	if err := retired.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "reused")
	if err := first.conn.Close(); err != nil {
		t.Fatal(err)
	}
	replay := server.event(t, "subscribe", "anchor")
	replacement, _ := client.Subscribe(context.Background(), Subscription{Topic: "reused", Type: "new"})
	defer replacement.Close()
	if ev := server.event(t, "subscribe", "reused"); ev.id != replay.id || ev.id == first.id {
		t.Fatal("retired topic was not safely reused on the new socket")
	}
}

func TestRetirementIsBoundedBySocketRotation(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{InitialBackoff: time.Millisecond})
	anchor, _ := client.Subscribe(context.Background(), Subscription{Topic: "anchor", Type: "*"})
	defer anchor.Close()
	first := server.event(t, "subscribe", "anchor")
	for i := 0; i < maxRetiredTopics; i++ {
		topic := fmt.Sprintf("retired-%d", i)
		s, _ := client.Subscribe(context.Background(), Subscription{Topic: topic, Type: "*"})
		if ev := server.event(t, "subscribe", topic); ev.id != first.id {
			t.Fatal("unexpected early rotation")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		server.event(t, "unsubscribe", topic)
	}
	replay := server.event(t, "subscribe", "anchor")
	if replay.id == first.id {
		t.Fatal("socket was not rotated")
	}
	s, _ := client.Subscribe(context.Background(), Subscription{Topic: "retired-0", Type: "*"})
	defer s.Close()
	if ev := server.event(t, "subscribe", "retired-0"); ev.id != replay.id {
		t.Fatal("retirement persisted after rotation")
	}
}

func TestLateJoinerHasOwnDialBudget(t *testing.T) {
	server := newSocketServer(t)
	dialer := *websocket.DefaultDialer
	var attempts atomic.Int32
	secondStarted := make(chan struct{})
	failSecond := make(chan struct{})
	thirdStarted := make(chan struct{})
	allowThird := make(chan struct{})
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch attempts.Add(1) {
		case 1:
			return nil, errors.New("first dial failed")
		case 2:
			close(secondStarted)
			select {
			case <-failSecond:
				return nil, errors.New("second dial failed")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case 3:
			close(thirdStarted)
			select {
			case <-allowThird:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	client := server.client(t, Config{Dialer: &dialer, MaxAttempts: 2, InitialBackoff: time.Millisecond})
	early, _ := client.Subscribe(context.Background(), Subscription{Topic: "topic", Type: "*"})
	defer early.Close()
	receive(t, early.Errors)
	select {
	case <-secondStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("second dial did not start")
	}
	late, _ := client.Subscribe(context.Background(), Subscription{Topic: "topic", Type: "*"})
	defer late.Close()
	close(failSecond)
	select {
	case <-early.done:
	case <-time.After(3 * time.Second):
		t.Fatal("early stream did not exhaust its retry budget")
	}
	receive(t, late.Errors)
	select {
	case <-thirdStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("late stream did not get another attempt")
	}
	select {
	case <-late.done:
		t.Fatal("late stream inherited early failures")
	default:
	}
	close(allowThird)
	server.event(t, "subscribe", "topic")
}

func TestMultiplexConcurrentCloseAndSlowConsumer(t *testing.T) {
	server := newSocketServer(t)
	client := server.client(t, Config{BufferSize: 1})
	slow, _ := client.Subscribe(context.Background(), Subscription{Topic: "topic", Type: "*"})
	defer slow.Close()
	ev := server.event(t, "subscribe", "topic")
	fast, _ := client.Subscribe(context.Background(), Subscription{Topic: "topic", Type: "*"})
	defer fast.Close()
	for i := 0; i < 2; i++ {
		msg := fmt.Sprintf(`{"topic":"topic","type":"x","timestamp":%d,"payload":{}}`, i)
		if err := ev.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			t.Fatal(err)
		}
		if got := receive(t, fast.Messages); got.Timestamp != int64(i) {
			t.Fatal("bad message")
		}
	}
	if err := receive(t, slow.Errors); err != errSlowConsumer {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-slow.done:
	case <-time.After(3 * time.Second):
		t.Fatal("slow stream did not stop")
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := client.Subscribe(ctx, Subscription{Topic: "topic", Type: "*"})
			if err != nil {
				t.Error(err)
				return
			}
			cancel()
			_ = s.Close()
		}()
	}
	wg.Wait()
	if err := fast.Close(); err != nil {
		t.Fatal(err)
	}
	server.event(t, "unsubscribe", "topic")
}
