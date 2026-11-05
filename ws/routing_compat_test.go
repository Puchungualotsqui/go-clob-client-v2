package ws

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func receiveTypedRouting(t *testing.T, s *Stream[Message], want string) {
	t.Helper()
	select {
	case m, ok := <-s.Messages:
		if !ok || string(m.Raw) != want {
			t.Fatalf("received %s (open=%v), want %s", m.Raw, ok, want)
		}
		if _, err := m.Typed(); err != nil {
			t.Fatalf("routed event is not typed-decodable: %v", err)
		}
	case err := <-s.Errors:
		t.Fatalf("unexpected routing error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func TestMarketRoutingCanonicalIdentifiersOverWebSocket(t *testing.T) {
	h, client := newWSHarness(t)
	a := subscribeMarket(t, client, []string{"0x2a", "00042"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "42")
	b := subscribeMarket(t, client, []string{"0b101010"}, false)
	other := subscribeMarket(t, client, []string{"43"}, false)
	p.request(t, "subscribe", "43")

	market := "0x" + strings.Repeat("11", 32)
	unmatched := `{"event_type":"book","asset_id":43,"market":"` + market + `","timestamp":"1"}`
	p.send(t, unmatched)
	receiveTypedRouting(t, other, unmatched)
	for _, body := range []string{
		`{"event_type":"book","asset_id":42,"market":"` + market + `","timestamp":"1"}`,
		`{"event_type":"price_change","market":"` + market + `","timestamp":"1","price_changes":[{"asset_id":"0x2A","price":"0.5","side":"BUY"}]}`,
		`{"event_type":"price_change","market":"` + market + `","timestamp":"1","price_changes":[{"asset_id":42,"price":"0.5","side":"BUY"}]}`,
		`{"event_type":"new_market","id":"1","question":"q","market":"` + market + `","slug":"s","description":"d","asset_ids":[42],"outcomes":[],"timestamp":"1"}`,
		`{"event_type":"market_resolved","id":"1","market":"` + market + `","assets_ids":["0o52"],"winning_asset_id":42,"winning_outcome":"Yes","timestamp":"1"}`,
	} {
		p.send(t, body)
		receiveTypedRouting(t, a, body)
		receiveTypedRouting(t, b, body)
	}
	// The other stream must not have received any of the events for asset 42.
	p.send(t, unmatched)
	receiveTypedRouting(t, other, unmatched)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	p.send(t, `{"event_type":"book","asset_id":"42","market":"`+market+`","timestamp":"2"}`)
	receiveTypedRouting(t, b, `{"event_type":"book","asset_id":"42","market":"`+market+`","timestamp":"2"}`)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "42")
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", "43")
}

func TestUserRoutingCanonicalMarketsOverWebSocket(t *testing.T) {
	h, client := newWSHarness(t)
	creds := Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	market := "0x" + strings.Repeat("ab", 32)
	a := subscribeUser(t, client, []string{strings.ToUpper(strings.TrimPrefix(market, "0x"))}, creds)
	p := h.peer(t)
	p.request(t, "subscribe", market)
	b := subscribeUser(t, client, []string{market}, creds)
	otherID := "0x" + strings.Repeat("cd", 32)
	other := subscribeUser(t, client, []string{otherID}, creds)
	p.request(t, "subscribe", otherID)

	bytes := make([]int, 32)
	for i := range bytes {
		bytes[i] = 0xab
	}
	encoded, err := json.Marshal(bytes)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"event_type":"order","id":"o","market":` + string(encoded) + `,"asset_id":"1","side":"BUY","price":"0.5"}`
	otherBody := `{"event_type":"order","id":"other","market":"` + otherID + `","asset_id":"1","side":"BUY","price":"0.5"}`
	p.send(t, otherBody)
	receiveTypedRouting(t, other, otherBody)
	p.send(t, body)
	receiveTypedRouting(t, a, body)
	receiveTypedRouting(t, b, body)
	p.send(t, otherBody)
	receiveTypedRouting(t, other, otherBody)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	p.send(t, body)
	receiveTypedRouting(t, b, body)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", market)
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	p.request(t, "unsubscribe", otherID)
}

func TestRoutingMalformedIdentifiersAndWildcard(t *testing.T) {
	h, client := newWSHarness(t)
	market := subscribeMarket(t, client, []string{"42"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "42")
	p.send(t, `{"event_type":"book","asset_id":-1}`)
	select {
	case err := <-market.Errors:
		if err == nil || !strings.Contains(err.Error(), "asset_id") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("malformed asset ID not reported")
	}
	validMarket := "0x" + strings.Repeat("aa", 32)
	body := `{"event_type":"book","asset_id":42,"market":"` + validMarket + `","timestamp":"1"}`
	p.send(t, body)
	receiveTypedRouting(t, market, body)

	user := subscribeUser(t, client, nil, Credentials{Key: "k", Secret: "s", Passphrase: "p"})
	up := h.peer(t)
	up.request(t, "subscribe")
	up.send(t, `{"event_type":"order","market":[1,2]}`)
	select {
	case err := <-user.Errors:
		if err == nil || !strings.Contains(err.Error(), "market") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("malformed market ID not reported")
	}
	// Wildcards still receive events without a market, as before.
	up.send(t, `{"event_type":"order","id":"no-market"}`)
	receive(t, user, `"no-market"`)
}
