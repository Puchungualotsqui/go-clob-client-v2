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

func TestStrictMarketRejectsWholeMessage(t *testing.T) {
	valid := `{"event_type":"book","asset_id":"a","id":"must-not-arrive"}`
	cases := []struct {
		name, data string
		binary     bool
	}{
		{"malformed-json", "[" + valid + ",", false},
		{"malformed-element-after", "[" + valid + ",null]", false},
		{"malformed-element-before", "[42," + valid + "]", false},
		{"missing-event-type", "[" + valid + `,{"asset_id":"a"}]`, false},
		{"empty-event-type", "[" + valid + `,{"event_type":"","asset_id":"a"}]`, false},
		{"unknown-event", "[" + valid + `,{"event_type":"future","asset_id":"a"}]`, false},
		{"unknown-before-valid", `[{"event_type":"future","asset_id":"a"},` + valid + "]", false},
		{"missing-routing", "[" + valid + `,{"event_type":"book"}]`, false},
		{"bad-routing", "[" + valid + `,{"event_type":"book","asset_id":-1}]`, false},
		{"empty-routing", "[" + valid + `,{"event_type":"book","asset_id":""}]`, false},
		{"null-list-id", "[" + valid + `,{"event_type":"new_market","assets_ids":["a",null]}]`, false},
		{"bad-list-id", "[" + valid + `,{"event_type":"new_market","assets_ids":["a",{}]}]`, false},
		{"bad-routing-shape", "[" + valid + `,{"event_type":"book","assets_ids":{}}]`, false},
		{"missing-change-id", "[" + valid + `,{"event_type":"price_change","asset_id":"a","price_changes":[{"asset_id":"a"},{}]}]`, false},
		{"null-change", "[" + valid + `,{"event_type":"price_change","price_changes":[{"asset_id":"a"},null]}]`, false},
		{"empty-frame", "", false},
		{"unicode-space-before", "\u00a0" + valid, false},
		{"unicode-space-after", valid + "\u2003", false},
		{"unicode-space-batch", "\u2003[" + valid + "]", false},
		{"vertical-tab", valid + "\v", false},
		{"form-feed", "\f" + valid, false},
		{"invalid-utf8", "[" + valid + `,{"event_type":"book","asset_id":"` + string([]byte{0xff}) + `"}]`, false},
		{"duplicate-id", "[" + valid + `,{"event_type":"book","asset_id":"a","asset_id":"other"}]`, false},
		{"escaped-duplicate-id", "[" + valid + `,{"event_type":"book","asset_id":"a","asset_\u0069d":"other"}]`, false},
		{"case-variant-id", "[" + valid + `,{"event_type":"book","asset_id":"other","ASSET_ID":"a"}]`, false},
		{"case-variant-single", "[" + valid + `,{"event_type":"price_change","asset_id":"other","price_changes":[{"ASSET_ID":"a"}]}]`, false},
		{"case-variant-event-type", "[" + valid + `,{"EVENT_TYPE":"book","asset_id":"a"}]`, false},
		{"case-variant-changes", "[" + valid + `,{"event_type":"price_change","asset_id":"other","PRICE_CHANGES":[{"asset_id":"a"}]}]`, false},
		{"case-variant-nested-field", "[" + valid + `,{"event_type":"book","asset_id":"a","bids":[{"PRICE":"0.5","size":"1"}]}]`, false},
		{"nested-duplicate", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":[{"x":1,"x":2}]}]`, false},
		{"nested-case-duplicate", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"x":1,"X":2}}]`, false},
		{"unicode-fold-duplicate", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"s":1,"ſ":2}}]`, false},
		{"unicode-fold-reserved", "[" + valid + `,{"event_type":"book","asset_id":"other","aſſet_id":"a"}]`, false},
		{"empty-key", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"":1}}]`, false},
		{"control-key", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"x\u0000":1}}]`, false},
		{"invalid-utf8-nested-key", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"` + string([]byte{0xff}) + `":1}}]`, false},
		{"invalid-utf8-nested-value", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":[{"x":"` + string([]byte{0xff}) + `"}]}]`, false},
		{"unpaired-high-key", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":{"\ud800":1}}]`, false},
		{"unpaired-low-value", "[" + valid + `,{"event_type":"book","asset_id":"a","extra":["\udc00"]}]`, false},
		{"binary", valid, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, c := newWSHarness(t)
			c.config.StrictMarket = true
			a := subscribeMarket(t, c, []string{"a"}, false)
			p := h.peer(t)
			p.request(t, "subscribe", "a")
			b := subscribeMarket(t, c, []string{"a"}, false)
			kind := websocket.TextMessage
			if tc.binary {
				kind = websocket.BinaryMessage
			}
			if err := p.conn.WriteMessage(kind, []byte(tc.data)); err != nil {
				t.Fatal(err)
			}
			requireStreamError(t, a, ErrStrictMarketMessage)
			requireStreamError(t, b, ErrStrictMarketMessage)
			p = h.peer(t)
			p.request(t, "subscribe", "a")
			for _, s := range []*Stream[Message]{a, b} {
				state := awaitState(t, s, PhaseReplaySent, 2, 1)
				if state.StrictMarketRejections != 1 {
					t.Fatalf("rejection state: %+v", state)
				}
				select {
				case msg := <-s.Messages:
					t.Fatalf("part of rejected frame delivered: %s", msg.Raw)
				default:
				}
			}
			p.send(t, `{"event_type":"book","asset_id":"a","id":"fresh"}`)
			for _, s := range []*Stream[Message]{a, b} {
				msg := nextMessage(t, s)
				if msg.Generation != 2 || string(msg.Raw) != `{"event_type":"book","asset_id":"a","id":"fresh"}` {
					t.Fatalf("fresh message: %+v", msg)
				}
			}
		})
	}
}

func TestStrictMarketJSONUnicodeAndIndependentObjects(t *testing.T) {
	valid := []string{
		" \t\r\n" + `{"event_type":"book","asset_id":"a"}` + "\n\r\t ",
		`{"event_type":"book","asset_\u0069d":"a","extra":{"UpperCase":"ok","unicode":"中文 😀","pair":"\ud83d\ude80","literal":"\\uD800"}}`,
		`[{"event_type":"book","asset_id":"a","extra":{"x":1}},{"event_type":"book","asset_id":"a","extra":{"X":2}}]`,
		`{"event_type":"book","asset_id":"a","extra":{"literal-replacement":"�","huge":1e10000}}`,
	}
	for _, data := range valid {
		if _, err := strictMarketMessages([]byte(data)); err != nil {
			t.Errorf("valid input %s rejected: %v", data, err)
		}
	}
	invalid := []string{
		`{"event_type":"book","asset_id":"a","extra":"\ud800x"}`,
		`{"event_type":"book","asset_id":"a","extra":"\ud800\u0041"}`,
		`{"event_type":"book","asset_id":"a","extra":"\ud800\ud800"}`,
		`{"event_type":"book","asset_id":"a","extra":"\ud800\\udc00"}`,
		`{"event_type":"book","asset_id":"a","extra":"\udc00\ud800"}`,
	}
	for _, data := range invalid {
		if _, err := strictMarketMessages([]byte(data)); err == nil {
			t.Errorf("unpaired surrogate accepted: %s", data)
		}
	}
}

func TestStrictMarketRejectionsSurviveSaturatedDiagnostics(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.StrictMarket = true
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	for i := 0; i < cap(s.errors); i++ {
		reportStream(s, errors.New("older diagnostic"), false)
	}
	g.mu.Unlock()
	// Do not drain Errors or StateChanges while two failures and replays occur.
	for i := uint64(1); i <= 2; i++ {
		p.send(t, `[{"event_type":"book","asset_id":"a"},{"event_type":"future"}]`)
		p = h.peer(t)
		p.request(t, "subscribe", "a")
		deadline := time.Now().Add(3 * time.Second)
		for {
			state := s.State()
			if state.Phase == PhaseReplaySent && state.Generation == i+1 && state.Interruptions == i && state.StrictMarketRejections == i {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("state after %d rejections: %+v", i, state)
			}
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case <-s.StateChanges:
		if state := s.State(); state.StrictMarketRejections != 2 || state.Interruptions != 2 {
			t.Fatalf("coalesced state: %+v", state)
		}
	default:
		t.Fatal("missing state wakeup")
	}
	late := subscribeMarket(t, c, []string{"a"}, false)
	if state := late.State(); state.StrictMarketRejections != 2 || state.Interruptions != 2 {
		t.Fatalf("late subscriber state: %+v", state)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if state := s.State(); state.Phase != PhaseClosed || state.StrictMarketRejections != 2 {
		t.Fatalf("closed state: %+v", state)
	}
	if err := late.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := subscribeMarket(t, c, []string{"a"}, false)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	state := awaitState(t, fresh, PhaseReplaySent, 1, 0)
	if state.StrictMarketRejections != 0 {
		t.Fatalf("new group inherited rejection count: %+v", state)
	}
}

func TestStrictMarketAllowsUnmatchedAndDoesNotAffectUser(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.StrictMarket = true
	market := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `[]`)
	p.send(t, `PONG`)
	p.send(t, `[{"event_type":"book","asset_id":"other"},{"event_type":"book","asset_id":"a","id":"matched"}]`)
	msg := nextMessage(t, market)
	if string(msg.Raw) != `{"event_type":"book","asset_id":"a","id":"matched"}` || market.State().StrictMarketRejections != 0 {
		t.Fatalf("market result: %+v state %+v", msg, market.State())
	}
	user := subscribeUser(t, c, nil, Credentials{Key: "k", Secret: "s", Passphrase: "p"})
	p = h.peer(t)
	p.request(t, "subscribe")
	p.send(t, `[{"event_type":"future"},null,{"event_type":"order","id":"ok"}]`)
	msg = nextMessage(t, user)
	if msg.EventType != "order" || user.State().StrictMarketRejections != 0 {
		t.Fatalf("user result: %+v state %+v", msg, user.State())
	}
}

func TestStrictMarketInterruptedStateBeforeReplay(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.StrictMarket = true
	var dials atomic.Int32
	entered, allow := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(allow) }) }
	defer unblock()
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dials.Add(1) == 2 {
			close(entered)
			select {
			case <-allow:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	c.config.Dialer = &dialer
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `{"event_type":"future","asset_id":"a"}`)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement dial did not start")
	}
	state := s.State()
	if state.Phase != PhaseInterrupted || state.Generation != 1 || state.Interruptions != 1 || state.StrictMarketRejections != 1 {
		t.Fatalf("interrupted state: %+v", state)
	}
	unblock()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	state = awaitState(t, s, PhaseReplaySent, 2, 1)
	if state.StrictMarketRejections != 1 {
		t.Fatalf("replay lost rejection count: %+v", state)
	}
}

func TestStrictMarketStaleRejectionCannotRetireReplacement(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.StrictMarket = true
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	c.mu.Lock()
	g := c.groups[groupKey{channel: "market"}]
	c.mu.Unlock()
	g.mu.Lock()
	old := g.conn
	g.mu.Unlock()
	if old == nil {
		t.Fatal("missing connection")
	}
	_ = p.conn.Close()
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	awaitState(t, s, PhaseReplaySent, 2, 1)
	g.rejectStrictMarket(old, errors.New("stale frame"))
	if state := s.State(); state.StrictMarketRejections != 0 || state.Interruptions != 1 || state.Phase != PhaseReplaySent {
		t.Fatalf("stale rejection changed state: %+v", state)
	}
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	msg := nextMessage(t, s)
	if msg.Generation != 2 {
		t.Fatalf("replacement message: %+v", msg)
	}
}

func TestStrictMarketReadLimitInterruptionIsNotValidationRejection(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.StrictMarket = true
	c.config.ReadLimit = 8
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `123456789`)
	requireStreamError(t, s, websocket.ErrReadLimit)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	state := awaitState(t, s, PhaseReplaySent, 2, 1)
	if state.StrictMarketRejections != 0 {
		t.Fatalf("read failure counted as validation: %+v", state)
	}
}

func TestDefaultMarketRemainsPermissive(t *testing.T) {
	parsed, err := ParseMessages([]byte("\u00a0" + `{"event_type":"book","asset_id":"a"}` + "\u2003"))
	if err != nil || len(parsed) != 1 {
		t.Fatalf("permissive Unicode whitespace changed: %v, %v", parsed, err)
	}
	h, c := newWSHarness(t)
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `[{"event_type":"book","asset_id":"a","id":"valid"},null,{"event_type":"unknown","asset_id":"a"}]`)
	msg := nextMessage(t, s)
	if string(msg.Raw) != `{"event_type":"book","asset_id":"a","id":"valid"}` {
		t.Fatalf("permissive message: %+v", msg)
	}
	select {
	case err := <-s.Errors:
		if err == nil || errors.Is(err, ErrStrictMarketMessage) {
			t.Fatalf("unexpected diagnostic: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing parse diagnostic")
	}
	p.send(t, `{"event_type":"book","asset_id":"other","asset_id":"a","extra":{"x":1,"X":2}}`)
	msg = nextMessage(t, s)
	if msg.EventType != "book" {
		t.Fatalf("permissive duplicate keys: %+v", msg)
	}
	if state := s.State(); state.StrictMarketRejections != 0 || state.Interruptions != 0 {
		t.Fatalf("permissive state: %+v", state)
	}
}
