package ws

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestMarketRoutingExactKeysAndOpaqueFallback(t *testing.T) {
	for _, tc := range []struct {
		name, id, body string
		matched        bool
	}{
		{"top-level exact", "42", `{"event_type":"book","asset_id":"42","ASSET_ID":"43"}`, true},
		{"top-level unknown", "43", `{"event_type":"book","asset_id":"42","ASSET_ID":"43"}`, false},
		{"unknown only", "42", `{"event_type":"book","ASSET_ID":"42"}`, false},
		{"nested exact", "42", `{"event_type":"price_change","price_changes":[{"asset_id":"42","ASSET_ID":"43"}]}`, true},
		{"nested unknown", "43", `{"event_type":"price_change","price_changes":[{"asset_id":"42","ASSET_ID":"43"}]}`, false},
		{"array unknown", "43", `{"event_type":"new_market","assets_ids":["42"],"ASSETS_IDS":["43"]}`, false},
		{"opaque exact", "legacy-id", `{"event_type":"book","asset_id":"legacy-id"}`, true},
		{"opaque case", "legacy-id", `{"event_type":"book","asset_id":"LEGACY-ID"}`, false},
		{"invalid numeric string exact", "-1", `{"event_type":"book","asset_id":"-1"}`, true},
		{"empty is u256 zero", "0", `{"event_type":"book","asset_id":""}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages, err := marketParser([]string{tc.id})([]byte(tc.body))
			if err != nil || (len(messages) == 1) != tc.matched {
				t.Fatalf("messages = %v, error = %v; want matched %v", messages, err, tc.matched)
			}
		})
	}
}

func TestUserRoutingExactKeysAndOpaqueFallback(t *testing.T) {
	h, c := newWSHarness(t)
	creds := Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	a := subscribeUser(t, c, []string{"legacy-a"}, creds)
	p := h.peer(t)
	p.request(t, "subscribe", "legacy-a")
	b := subscribeUser(t, c, []string{"legacy-b"}, creds)
	p.request(t, "subscribe", "legacy-b")

	p.send(t, `{"event_type":"order","market":"legacy-a","MARKET":"legacy-b","id":"first"}`)
	p.send(t, `{"event_type":"order","market":"legacy-b","id":"second"}`)
	receive(t, a, `"first"`)
	receive(t, b, `"second"`)
}

func TestConcurrentCanonicalSubscriptionsAndDelivery(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.BufferSize = 128
	anchor := subscribeMarket(t, c, []string{"42"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "42")

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 30; j++ {
				ctx, cancel := context.WithCancel(context.Background())
				s, err := c.SubscribeMarket(ctx, []string{"0x2a", "00042", "0b101010"}, false)
				if err != nil {
					t.Error(err)
					cancel()
					return
				}
				cancel()
				_ = s.Close()
			}
		}()
	}
	close(start)
	for i := 0; i < 50; i++ {
		p.send(t, `{"event_type":"book","asset_id":42}`)
	}
	wg.Wait()
	for i := 0; i < 50; i++ {
		receive(t, anchor, `"asset_id":42`)
	}
}

func TestCanonicalAndOpaqueIDsSurviveReconnect(t *testing.T) {
	h, c := newWSHarness(t)
	asset := subscribeMarket(t, c, []string{"0X2a", "00042", "legacy-asset"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "42", "legacy-asset")
	replay(t, asset, 1, false)
	_ = p.conn.Close()
	p = h.peer(t)
	p.request(t, "subscribe", "42", "legacy-asset")
	replay(t, asset, 2, true)
	p.send(t, `{"event_type":"book","asset_id":"0b101010","id":"canonical"}`)
	receive(t, asset, `"canonical"`)
	p.send(t, `{"event_type":"book","asset_id":"legacy-asset","id":"opaque"}`)
	receive(t, asset, `"opaque"`)

	creds := Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	market := "0x" + strings.Repeat("ab", 32)
	user := subscribeUser(t, c, []string{strings.ToUpper(market[2:]), market, "legacy-market"}, creds)
	up := h.peer(t)
	up.request(t, "subscribe", market, "legacy-market")
	replay(t, user, 1, false)
	_ = up.conn.Close()
	up = h.peer(t)
	up.request(t, "subscribe", market, "legacy-market")
	replay(t, user, 2, true)
	values := make([]int, 32)
	for i := range values {
		values[i] = 0xab
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	up.send(t, `{"event_type":"order","market":`+string(encoded)+`,"id":"canonical"}`)
	receive(t, user, `"canonical"`)
	up.send(t, `{"event_type":"order","market":"legacy-market","id":"opaque"}`)
	receive(t, user, `"opaque"`)
}
