package ws

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTypedRequiredFields(t *testing.T) {
	tests := []struct {
		event string
		body  string
		keys  []string
	}{
		{"book", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0"}`, []string{"asset_id", "market", "timestamp"}},
		{"price_change", `{"market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0"}`, []string{"market", "timestamp"}},
		{"tick_size_change", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","old_tick_size":"0.1","new_tick_size":"0.01","timestamp":"0"}`, []string{"asset_id", "market", "old_tick_size", "new_tick_size", "timestamp"}},
		{"last_trade_price", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","price":"0","timestamp":"0"}`, []string{"asset_id", "market", "price", "timestamp"}},
		{"best_bid_ask", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","best_bid":"0","best_ask":"0","spread":"0","timestamp":"0"}`, []string{"asset_id", "market", "best_bid", "best_ask", "spread", "timestamp"}},
		{"new_market", `{"id":"","question":"","market":"0x0000000000000000000000000000000000000000000000000000000000000001","slug":"","description":"","assets_ids":[],"outcomes":[],"timestamp":"0"}`, []string{"id", "question", "market", "slug", "description", "assets_ids", "outcomes", "timestamp"}},
		{"market_resolved", `{"id":"","market":"0x0000000000000000000000000000000000000000000000000000000000000001","assets_ids":[],"winning_asset_id":"1","winning_outcome":"","timestamp":"0"}`, []string{"id", "market", "assets_ids", "winning_asset_id", "winning_outcome", "timestamp"}},
		{"trade", `{"id":"","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","size":"0","price":"0","status":"MATCHED"}`, []string{"id", "market", "asset_id", "side", "size", "price", "status"}},
		{"order", `{"id":"","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","price":"0"}`, []string{"id", "market", "asset_id", "side", "price"}},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tt.body), &fields); err != nil {
				t.Fatal(err)
			}
			decode := func() error {
				raw, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				_, err = (Message{EventType: tt.event, Raw: raw}).Typed()
				return err
			}
			if err := decode(); err != nil {
				t.Fatalf("valid: %v", err)
			}
			for _, key := range tt.keys {
				original := fields[key]
				delete(fields, key)
				if err := decode(); err == nil {
					t.Errorf("missing %s accepted", key)
				}
				fields[key] = json.RawMessage(`null`)
				if err := decode(); err == nil {
					t.Errorf("null %s accepted", key)
				}
				fields[key] = original
			}
		})
	}
}

func TestTypedNestedAndDefaults(t *testing.T) {
	tests := []struct {
		event, body string
		ok          bool
	}{
		{"book", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0","bids":null}`, false},
		{"book", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0","bids":[null]}`, false},
		{"book", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0","bids":[{"price":"0"}]}`, false},
		{"book", `{"asset_id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":""}`, false},
		{"price_change", `{"market":"0x0000000000000000000000000000000000000000000000000000000000000001","timestamp":"0","price_changes":[{"asset_id":"1","price":"0","side":null}]}`, false},
		{"trade", `{"id":"t","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","size":"1","price":"0","status":"future","maker_orders":null}`, false},
		{"trade", `{"id":"t","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","size":"1","price":"0","status":"future","maker_orders":[{"asset_id":"1","price":"0"}]}`, false},
		{"new_market", `{"id":"1","question":"q","market":"0x0000000000000000000000000000000000000000000000000000000000000001","slug":"s","description":"d","asset_ids":[],"outcomes":[],"timestamp":"0","event_message":{"id":"e"}}`, false},
		{"market_resolved", `{"id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_ids":[],"winning_asset_id":"1","winning_outcome":"Yes","timestamp":"0","outcomes":null}`, true},
		{"market_resolved", `{"id":"1","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_ids":[],"winning_asset_id":"1","winning_outcome":"Yes","timestamp":"0","outcomes":[null]}`, false},
		{"order", `{"id":"o","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","price":"0","associate_trades":[null]}`, false},
		{"order", `{"id":"o","market":"0x0000000000000000000000000000000000000000000000000000000000000001","asset_id":"1","side":"BUY","price":"0","associate_trades":null,"status":"new"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.event+"/"+tt.body, func(t *testing.T) {
			_, err := (Message{EventType: tt.event, Raw: json.RawMessage(tt.body)}).Typed()
			if (err == nil) != tt.ok {
				t.Fatalf("got error %v, want success %v", err, tt.ok)
			}
		})
	}
}

func TestTypedDefaultVectors(t *testing.T) {
	market := "0x" + strings.Repeat("01", 32)
	cases := []struct {
		event, body string
		valid       func(any) bool
	}{
		{"book", `{"asset_id":"1","market":"` + market + `","timestamp":"0"}`, func(v any) bool { e := v.(*BookUpdate); return e.Bids != nil && e.Asks != nil }},
		{"price_change", `{"market":"` + market + `","timestamp":"0"}`, func(v any) bool { return v.(*PriceChange).PriceChanges != nil }},
		{"trade", `{"id":"t","market":"` + market + `","asset_id":"1","side":"BUY","size":"1","price":"1","status":"ok"}`, func(v any) bool { return v.(*TradeMessage).MakerOrders != nil }},
		{"market_resolved", `{"id":"1","market":"` + market + `","assets_ids":[],"winning_asset_id":"1","winning_outcome":"Yes","timestamp":"0","outcomes":null}`, func(v any) bool { return v.(*MarketResolved).Outcomes != nil }},
	}
	for _, tc := range cases {
		v, err := (Message{EventType: tc.event, Raw: json.RawMessage(tc.body)}).Typed()
		if err != nil || !tc.valid(v) {
			t.Errorf("%s: %v, %v", tc.event, v, err)
		}
	}
}

func TestTypedRustIdentifiersAndEnums(t *testing.T) {
	market := "0x" + strings.Repeat("01", 32)
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	base := map[string]any{"id": "t", "market": market, "asset_id": "1", "side": "BUY", "size": "1", "price": "0", "status": "MATCHED"}
	decode := func() (any, error) {
		raw, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		return (Message{EventType: "trade", Raw: raw}).Typed()
	}
	check := func(key string, value any, ok bool) {
		t.Helper()
		old, had := base[key]
		base[key] = value
		_, err := decode()
		if (err == nil) != ok {
			t.Errorf("%s=%v: %v", key, value, err)
		}
		if had {
			base[key] = old
		} else {
			delete(base, key)
		}
	}
	for _, v := range []any{"1", "0xFF", "0b11", "0o77", "1_000", "", uint64(12), "115792089237316195423570985008687907853269984665640564039457584007913129639935"} {
		check("asset_id", v, true)
	}
	for _, v := range []any{"-1", "x1", "0xgg", "+1", "115792089237316195423570985008687907853269984665640564039457584007913129639936", -1, 1.5, true, nil} {
		check("asset_id", v, false)
	}
	for _, v := range []any{market, strings.TrimPrefix(market, "0x"), strings.Repeat("ab", 32), make([]int, 32)} {
		check("market", v, true)
	}
	for _, v := range []any{"0x1", "0x" + strings.Repeat("gg", 32), make([]int, 31), make([]int, 33), nil, 1} {
		check("market", v, false)
	}
	for _, key := range []string{"owner", "trade_owner"} {
		for _, v := range []any{uuid, strings.ReplaceAll(uuid, "-", ""), nil} {
			check(key, v, true)
		}
		for _, v := range []any{"owner", "", 1} {
			check(key, v, false)
		}
	}
	for _, v := range []any{market, "", nil} {
		check("transaction_hash", v, true)
	}
	for _, v := range []any{"0x123", 1, make([]int, 31), make([]int, 32)} {
		check("transaction_hash", v, false)
	}
	for _, key := range []string{"side", "status"} {
		for _, v := range []any{"BUY", "future", ""} {
			check(key, v, true)
		}
		for _, v := range []any{1, true, nil, map[string]any{}} {
			check(key, v, false)
		}
	}
	for _, key := range []string{"type", "trader_side"} {
		for _, v := range []any{"future", "", nil} {
			check(key, v, true)
		}
		for _, v := range []any{1, true, []any{}} {
			check(key, v, false)
		}
	}
	for _, key := range []string{"timestamp", "last_update", "matchtime"} {
		for _, v := range []any{"0", "-9223372036854775808", "+0001", nil} {
			check(key, v, true)
		}
		for _, v := range []any{"1.0", "9223372036854775808", "", 1, true} {
			check(key, v, false)
		}
	}
	base["maker_orders"] = []any{map[string]any{"asset_id": "2", "matched_amount": "1", "order_id": "o", "outcome": "Yes", "owner": uuid, "price": "1"}}
	if _, err := decode(); err != nil {
		t.Fatalf("valid maker order: %v", err)
	}
	base["maker_orders"].([]any)[0].(map[string]any)["owner"] = "invalid"
	if _, err := decode(); err == nil {
		t.Error("invalid maker UUID accepted")
	}
}

func TestTypedIdentifierRepresentations(t *testing.T) {
	body := `{"asset_id":42,"market":[1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"timestamp":"+0001"}`
	msg := Message{EventType: "book", Raw: json.RawMessage(body)}
	v, err := msg.Typed()
	if err != nil {
		t.Fatal(err)
	}
	e := v.(*BookUpdate)
	if e.AssetID != "42" || e.Market != "0x01"+strings.Repeat("00", 31) || e.Timestamp != "1" || string(msg.Raw) != body {
		t.Fatalf("unexpected normalized book: %+v", e)
	}
}

func TestTypedMarketAssetLists(t *testing.T) {
	market := "0x" + strings.Repeat("01", 32)
	for _, tc := range []struct {
		ids string
		ok  bool
	}{
		{`[1,"0x2","3"]`, true}, {`["-1"]`, false}, {`[null]`, false}, {`["0xzz"]`, false},
	} {
		body := `{"id":"1","market":"` + market + `","assets_ids":` + tc.ids + `,"winning_asset_id":"1","winning_outcome":"Yes","timestamp":"0"}`
		_, err := (Message{EventType: "market_resolved", Raw: json.RawMessage(body)}).Typed()
		if (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.ids, err)
		}
	}
}

func TestRequestWireModel(t *testing.T) {
	initial, custom := true, false
	raw, err := json.Marshal(SubscriptionRequest{Type: "market", Operation: "subscribe", Markets: []string{}, AssetIDs: []string{"123"}, InitialDump: &initial, CustomFeatureEnabled: &custom})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"type":"market"`, `"operation":"subscribe"`, `"markets":[]`, `"assets_ids":["123"]`, `"initial_dump":true`, `"custom_feature_enabled":false`} {
		if !strings.Contains(string(raw), part) {
			t.Errorf("missing %s in %s", part, raw)
		}
	}
}
