package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type testPeer struct {
	conn     *websocket.Conn
	requests chan SubscriptionRequest
}

type wsHarness struct {
	server *httptest.Server
	peers  chan *testPeer
}

func newWSHarness(t *testing.T) (*wsHarness, *Client) {
	t.Helper()
	h := &wsHarness{peers: make(chan *testPeer, 20)}
	h.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		p := &testPeer{conn: conn, requests: make(chan SubscriptionRequest, 40)}
		h.peers <- p
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req SubscriptionRequest
			if json.Unmarshal(data, &req) == nil {
				p.requests <- req
			}
		}
	}))
	t.Cleanup(h.server.Close)
	client, err := NewClient("ws"+strings.TrimPrefix(h.server.URL, "http"), Config{BufferSize: 1, InitialBackoff: time.Millisecond, HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return h, client
}

func (h *wsHarness) peer(t *testing.T) *testPeer {
	t.Helper()
	select {
	case p := <-h.peers:
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for connection")
		return nil
	}
}
func (p *testPeer) request(t *testing.T, op string, ids ...string) SubscriptionRequest {
	t.Helper()
	select {
	case req := <-p.requests:
		got := req.AssetIDs
		if req.Type == "user" {
			got = req.Markets
		}
		sort.Strings(got)
		sort.Strings(ids)
		if req.Operation != op || !reflect.DeepEqual(append([]string{}, got...), append([]string{}, ids...)) {
			t.Fatalf("unexpected request: %+v, want %s %v", req, op, ids)
		}
		return req
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s %v", op, ids)
		return SubscriptionRequest{}
	}
}
func (p *testPeer) send(t *testing.T, data string) {
	t.Helper()
	if err := p.conn.WriteMessage(websocket.TextMessage, []byte(data)); err != nil {
		t.Fatal(err)
	}
}
func receive(t *testing.T, s *Stream[Message], contains string) {
	t.Helper()
	select {
	case m, ok := <-s.Messages:
		if !ok || !strings.Contains(string(m.Raw), contains) {
			t.Fatalf("unexpected message: %s (open=%v), want %q", m.Raw, ok, contains)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %q", contains)
	}
}
func requireStreamError(t *testing.T, s *Stream[Message], target error) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case err, ok := <-s.Errors:
			if !ok {
				t.Fatalf("stream closed without %v", target)
			}
			if errors.Is(err, target) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %v", target)
		}
	}
}

func subscribeMarket(t *testing.T, c *Client, ids []string, custom bool) *Stream[Message] {
	t.Helper()
	s, err := c.SubscribeMarket(context.Background(), ids, custom)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func subscribeUser(t *testing.T, c *Client, ids []string, creds Credentials) *Stream[Message] {
	t.Helper()
	s, err := c.SubscribeUser(context.Background(), ids, creds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMultiplexMarketRoutingAndRefcounts(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a", "b", "a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a", "b")
	b := subscribeMarket(t, c, []string{"b", "c"}, false)
	p.request(t, "subscribe", "c")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	receive(t, a, `"a"`)
	p.send(t, `{"event_type":"price_change","price_changes":[{"asset_id":"b"}]}`)
	receive(t, a, `"b"`)
	receive(t, b, `"b"`)
	p.send(t, `{"event_type":"new_market","assets_ids":["c"]}`)
	receive(t, b, `"c"`)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "a")
	p.send(t, `{"event_type":"book","asset_id":"b"}`)
	receive(t, b, `"b"`)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "b", "c")
}

func TestMixedPriceChangeBatchIsDeliveredWhole(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	b := subscribeMarket(t, c, []string{"b"}, false)
	p.request(t, "subscribe", "b")
	other := subscribeMarket(t, c, []string{"c"}, false)
	p.request(t, "subscribe", "c")
	batch := `{"event_type":"price_change","price_changes":[{"asset_id":"a"},{"asset_id":"b"}]}`
	p.send(t, batch)
	p.send(t, `{"event_type":"book","asset_id":"c"}`)
	var gotA, gotB Message
	select {
	case gotA = <-a.Messages:
	case <-time.After(3 * time.Second):
		t.Fatal("missing a batch")
	}
	select {
	case gotB = <-b.Messages:
	case <-time.After(3 * time.Second):
		t.Fatal("missing b batch")
	}
	if string(gotA.Raw) != batch || string(gotB.Raw) != batch {
		t.Fatalf("batch was filtered: %s / %s", gotA.Raw, gotB.Raw)
	}
	gotA.Raw[0] = 'x'
	if string(gotB.Raw) != batch {
		t.Fatal("streams share raw message bytes")
	}
	receive(t, other, `"c"`)
}

func TestMultiplexCustomUpgradeAndReplay(t *testing.T) {
	h, c := newWSHarness(t)
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	b := subscribeMarket(t, c, []string{"a"}, true)
	requireStreamError(t, a, ErrReconnecting)
	requireStreamError(t, b, ErrReconnecting)
	p = h.peer(t)
	r := p.request(t, "subscribe", "a")
	if r.CustomFeatureEnabled == nil || !*r.CustomFeatureEnabled || r.InitialDump == nil || !*r.InitialDump {
		t.Fatalf("upgrade request: %+v", r)
	}
	_ = p.conn.Close()
	p = h.peer(t)
	r = p.request(t, "subscribe", "a")
	if r.CustomFeatureEnabled == nil || !*r.CustomFeatureEnabled {
		t.Fatalf("replay request: %+v", r)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	p.send(t, `{"event_type":"best_bid_ask","asset_id":"a"}`)
	receive(t, b, `"a"`)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "a")
}

func TestMultiplexUsersWildcardAndCredentialIsolation(t *testing.T) {
	h, c := newWSHarness(t)
	cred := Credentials{Key: "one", Secret: "secret", Passphrase: "pass"}
	other := Credentials{Key: "one", Secret: "different", Passphrase: "pass"}
	a := subscribeUser(t, c, []string{"m1"}, cred)
	p := h.peer(t)
	if req := p.request(t, "subscribe", "m1"); req.Auth == nil || *req.Auth != cred {
		t.Fatal("wrong credentials")
	}
	x := subscribeUser(t, c, nil, other)
	otherPeer := h.peer(t)
	if req := otherPeer.request(t, "subscribe"); req.Auth == nil || *req.Auth != other {
		t.Fatal("wrong credentials")
	}
	b := subscribeUser(t, c, []string{"m2"}, cred)
	p.request(t, "subscribe", "m2")
	wild := subscribeUser(t, c, nil, cred)
	requireStreamError(t, a, ErrReconnecting)
	requireStreamError(t, b, ErrReconnecting)
	requireStreamError(t, wild, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe")
	p.send(t, `{"event_type":"order","market":"m1"}`)
	receive(t, a, `"m1"`)
	receive(t, wild, `"m1"`)
	p.send(t, `{"event_type":"trade","market":"m2"}`)
	receive(t, b, `"m2"`)
	receive(t, wild, `"m2"`)
	otherPeer.send(t, `{"event_type":"order","market":"private"}`)
	receive(t, x, `"private"`)
	if err := wild.Close(); err != nil {
		t.Fatal(err)
	}
	requireStreamError(t, a, ErrReconnecting)
	requireStreamError(t, b, ErrReconnecting)
	p = h.peer(t)
	p.request(t, "subscribe", "m1", "m2")
	p.send(t, `{"event_type":"order","market":"m2"}`)
	receive(t, b, `"m2"`)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "m1")
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "m2")
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	otherPeer.request(t, "unsubscribe")
}

func TestMultipleWildcardsAndMissingMarket(t *testing.T) {
	h, c := newWSHarness(t)
	cred := Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	wild1 := subscribeUser(t, c, nil, cred)
	p := h.peer(t)
	p.request(t, "subscribe")
	wild2 := subscribeUser(t, c, nil, cred)
	explicit := subscribeUser(t, c, []string{"m"}, cred)
	if err := wild1.Close(); err != nil {
		t.Fatal(err)
	}
	p.send(t, `{"event_type":"trade","market":"m"}`)
	receive(t, wild2, `"m"`)
	receive(t, explicit, `"m"`)
	p.send(t, `{"event_type":"order","id":"no-market"}`)
	receive(t, wild2, `"no-market"`)
	if err := wild2.Close(); err != nil {
		t.Fatal(err)
	}
	p = h.peer(t)
	p.request(t, "subscribe", "m")
	p.send(t, `{"event_type":"order","market":"m","id":"after"}`)
	receive(t, explicit, `"after"`)
	if err := explicit.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "m")
}

func TestMultiplexSlowConsumerIsolated(t *testing.T) {
	h, c := newWSHarness(t)
	slow := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	fast := subscribeMarket(t, c, []string{"a"}, false)
	p.send(t, `{"event_type":"book","asset_id":"a","n":1}`)
	receive(t, fast, `"n":1`)
	p.send(t, `{"event_type":"book","asset_id":"a","n":2}`)
	receive(t, fast, `"n":2`)
	select {
	case err := <-slow.Errors:
		if err != errSlowConsumer {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no overflow error")
	}
	p.send(t, `{"event_type":"book","asset_id":"a","n":3}`)
	receive(t, fast, `"n":3`)
	if err := fast.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "a")
}
