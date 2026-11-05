package ws

import (
	"strings"
	"testing"
	"time"
)

func TestParseMessagesMalformedBatchReportsPartialResults(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		count      int
	}{
		{"single-element batch", `[{"event_type":123}]`, 0},
		{"mixed batch", `[{"event_type":"book"},{"event_type":123},{"event_type":"trade"}]`, 2},
		{"non-object element", `[{"event_type":"book"},42]`, 1},
		{"null element", `[{"event_type":"book"},null]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages, err := ParseMessages([]byte(tc.data))
			if err == nil || len(messages) != tc.count {
				t.Fatalf("got %d messages and error %v; want %d messages and an error", len(messages), err, tc.count)
			}
		})
	}
	if messages, err := ParseMessages([]byte(`[{"event_type":"book"},`)); err == nil || len(messages) != 0 {
		t.Fatalf("invalid JSON: got %v, %v; want no messages and an error", messages, err)
	}
}

func TestMixedMalformedBatchesRouteOnce(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.BufferSize = 16
	market := subscribeMarket(t, c, []string{"0x2a", "42"}, false)
	mp := h.peer(t)
	mp.request(t, "subscribe", "42")
	mp.send(t, `[{"event_type":"price_change","price_changes":[{"asset_id":42},{"asset_id":"0x2a"}]},{"event_type":123},{"event_type":"price_change","price_changes":[{"asset_id":42},{"asset_id":-1}]},{"event_type":"book","asset_id":"42","id":"sentinel"}]`)
	receive(t, market, `"price_changes"`)
	receive(t, market, `"sentinel"`)
	for _, want := range []string{"batch element 1", "asset_id"} {
		select {
		case err := <-market.Errors:
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("market error = %v; want %q", err, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing malformed batch error")
		}
	}
	select {
	case msg := <-market.Messages:
		t.Fatalf("duplicate market delivery: %s", msg.Raw)
	default:
	}

	creds := Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	wild := subscribeUser(t, c, nil, creds)
	up := h.peer(t)
	up.request(t, "subscribe")
	id := "0x" + strings.Repeat("ab", 32)
	explicit := subscribeUser(t, c, []string{strings.ToUpper(id[2:]), id}, creds)
	up.send(t, `[{"event_type":"order","market":"`+id+`","id":"first"},{"event_type":"order","market":[1,2]},{"event_type":"trade","id":"no-market"},{"event_type":"order","market":"`+id+`","id":"sentinel"}]`)
	receive(t, wild, `"first"`)
	receive(t, wild, `"no-market"`)
	receive(t, wild, `"sentinel"`)
	receive(t, explicit, `"first"`)
	receive(t, explicit, `"sentinel"`)
	for _, stream := range []*Stream[Message]{wild, explicit} {
		select {
		case err := <-stream.Errors:
			if err == nil || !strings.Contains(err.Error(), "market") {
				t.Fatalf("unexpected error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing malformed market error")
		}
		select {
		case msg := <-stream.Messages:
			t.Fatalf("duplicate or misrouted user delivery: %s", msg.Raw)
		default:
		}
	}
}
