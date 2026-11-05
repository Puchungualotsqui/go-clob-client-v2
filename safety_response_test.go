package clob

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const safetyUUID = "00000000-0000-0000-0000-000000000001"
const safetyHash = "0x1111111111111111111111111111111111111111111111111111111111111111"
const safetyAddress = "0x1111111111111111111111111111111111111111"

func safetyObject(t *testing.T, input string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSafetyRequiredFields(t *testing.T) {
	cases := []struct {
		name, input string
		validate    func([]byte) error
		required    []string
	}{
		{"post", `{"makingAmount":"","takingAmount":"0","orderID":"id","status":"LIVE","success":false}`, validatePostOrderResponse, []string{"makingAmount", "takingAmount", "orderID", "status", "success"}},
		{"open", fmt.Sprintf(`{"id":"id","status":"LIVE","owner":%q,"maker_address":%q,"market":%q,"asset_id":"1","side":"BUY","original_size":"1","size_matched":"0","price":"1","outcome":"yes","created_at":1,"expiration":"1","order_type":"GTC"}`, safetyUUID, safetyAddress, safetyHash), validateOpenOrderResponse, []string{"id", "status", "owner", "maker_address", "market", "asset_id", "side", "original_size", "size_matched", "price", "outcome", "created_at", "expiration", "order_type"}},
		{"trade", fmt.Sprintf(`{"id":"id","taker_order_id":"id","market":%q,"asset_id":"1","side":"BUY","size":"1","fee_rate_bps":"0","price":"1","status":"MATCHED","match_time":"1","last_update":"1","outcome":"yes","bucket_index":0,"owner":%q,"maker_address":%q,"trader_side":"TAKER"}`, safetyHash, safetyUUID, safetyAddress), validateTradeResponse, []string{"id", "taker_order_id", "market", "asset_id", "side", "size", "fee_rate_bps", "price", "status", "match_time", "last_update", "outcome", "bucket_index", "owner", "maker_address", "trader_side"}},
		{"balance", `{"balance":"0"}`, validateBalanceAllowanceResponse, []string{"balance"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.validate([]byte(tc.input)); err != nil {
				t.Fatal(err)
			}
			for _, field := range tc.required {
				for _, null := range []bool{false, true} {
					obj := safetyObject(t, tc.input)
					if null {
						obj[field] = nil
					} else {
						delete(obj, field)
					}
					b, err := json.Marshal(obj)
					if err != nil {
						t.Fatal(err)
					}
					if err := tc.validate(b); err == nil {
						t.Errorf("field %s null=%v accepted", field, null)
					}
				}
			}
			if err := tc.validate([]byte("null")); err == nil {
				t.Error("null object accepted")
			}
		})
	}
}

func TestSafetyOptionalAndIdentifiers(t *testing.T) {
	post := `{"makingAmount":"","takingAmount":"","orderID":"","status":"LIVE","success":false`
	for _, suffix := range []string{`}`, `,"transactionHashes":null,"tradeIds":null}`, `,"transactionsHashes":["` + safetyHash + `"],"tradeIDs":["id"]}`} {
		if err := validatePostOrderResponse([]byte(post + suffix)); err != nil {
			t.Fatal(err)
		}
	}
	for _, suffix := range []string{`,"transactionHashes":["bad"]}`, `,"transactionsHashes":[null]}`, `,"tradeIds":[null]}`, `,"transactionHashes":null,"transactionsHashes":[]}`, `,"tradeIds":[],"tradeIDs":null}`} {
		if err := validatePostOrderResponse([]byte(post + suffix)); err == nil {
			t.Errorf("accepted %s", suffix)
		}
	}
	for _, input := range []string{`{"balance":"0","allowances":null}`, `{"balance":"0","allowances":{"bad":"1"}}`, fmt.Sprintf(`{"balance":"0","allowances":{%q:null}}`, safetyAddress)} {
		if err := validateBalanceAllowanceResponse([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	if err := validateBalanceAllowanceResponse([]byte(fmt.Sprintf(`{"balance":"0","allowances":{%q:"1"}}`, safetyAddress))); err != nil {
		t.Fatal(err)
	}
}

func TestSafetyTypedIdentifiersAndDefaultCollections(t *testing.T) {
	open := fmt.Sprintf(`{"id":"","status":"LIVE","owner":%q,"maker_address":%q,"market":%q,"asset_id":"1","side":"BUY","original_size":"1","size_matched":"0","price":"1","outcome":"","created_at":1,"expiration":"1","order_type":"GTC"}`, safetyUUID, safetyAddress, safetyHash)
	trade := fmt.Sprintf(`{"id":"","taker_order_id":"","market":%q,"asset_id":"1","side":"BUY","size":"1","fee_rate_bps":"0","price":"1","status":"MATCHED","match_time":"1","last_update":"1","outcome":"","bucket_index":0,"owner":%q,"maker_address":%q,"trader_side":"TAKER"}`, safetyHash, safetyUUID, safetyAddress)
	for _, tc := range []struct {
		name, input string
		validate    func([]byte) error
	}{{"open", open, validateOpenOrderResponse}, {"trade", trade, validateTradeResponse}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, owner := range []string{safetyUUID, strings.ReplaceAll(safetyUUID, "-", ""), "{" + safetyUUID + "}", "urn:uuid:" + safetyUUID} {
				b := strings.Replace(tc.input, safetyUUID, owner, 1)
				if err := tc.validate([]byte(b)); err != nil {
					t.Errorf("valid owner %q: %v", owner, err)
				}
			}
			for _, owner := range []string{"bad", "x" + safetyUUID + "y", "URN:UUID:" + safetyUUID} {
				b := strings.Replace(tc.input, safetyUUID, owner, 1)
				if err := tc.validate([]byte(b)); err == nil {
					t.Errorf("invalid owner %q accepted", owner)
				}
			}
			for _, pair := range [][2]string{{safetyAddress, ""}, {safetyAddress, "0x1234"}, {safetyHash, "bad"}, {safetyHash, ""}} {
				b := strings.Replace(tc.input, pair[0], pair[1], 1)
				if err := tc.validate([]byte(b)); err == nil {
					t.Errorf("invalid identifier %q accepted", pair[1])
				}
			}
		})
	}
	for _, list := range []string{`null`, `[]`, `["", "id"]`} {
		b := strings.TrimSuffix(open, "}") + `,"associate_trades":` + list + `}`
		if err := validateOpenOrderResponse([]byte(b)); err != nil {
			t.Errorf("associate_trades %s: %v", list, err)
		}
	}
	for _, list := range []string{`[null]`, `{"bad":1}`} {
		b := strings.TrimSuffix(open, "}") + `,"associate_trades":` + list + `}`
		if err := validateOpenOrderResponse([]byte(b)); err == nil {
			t.Errorf("accepted associate_trades %s", list)
		}
	}
	for _, extra := range []string{``, `,"maker_orders":null`, `,"maker_orders":[]`, `,"transaction_hash":""`, `,"transaction_hash":"   "`, `,"transaction_hash":"` + safetyHash + `"`} {
		var v TradeResponse
		if err := json.Unmarshal([]byte(strings.TrimSuffix(trade, "}")+extra+`}`), &v); err != nil {
			t.Errorf("trade %s: %v", extra, err)
		}
	}
	for _, extra := range []string{`,"maker_orders":[null]`, `,"maker_orders":[{}]`, `,"transaction_hash":null`, `,"transaction_hash":"bad"`} {
		var v TradeResponse
		if err := json.Unmarshal([]byte(strings.TrimSuffix(trade, "}")+extra+`}`), &v); err == nil {
			t.Errorf("accepted trade %s", extra)
		}
	}
}

func TestSafetyRejectsCaseVariantKeys(t *testing.T) {
	maker := fmt.Sprintf(`{"order_id":"id","owner":%q,"maker_address":%q,"matched_amount":"1","price":"1","fee_rate_bps":"0","asset_id":"1","outcome":"yes","side":"BUY"}`, safetyUUID, safetyAddress)
	trade := fmt.Sprintf(`{"id":"id","taker_order_id":"id","market":%q,"asset_id":"1","side":"BUY","size":"1","fee_rate_bps":"0","price":"1","status":"MATCHED","match_time":"1","last_update":"1","outcome":"yes","bucket_index":0,"owner":%q,"maker_address":%q,"trader_side":"TAKER"}`, safetyHash, safetyUUID, safetyAddress)
	cases := []struct {
		name, input string
		fields      []string
		newValue    func() any
	}{
		{"post", `{"makingAmount":"1","takingAmount":"1","orderID":"id","status":"LIVE","success":true}`, []string{"makingAmount", "takingAmount", "orderID", "status", "success", "errorMsg", "transactionHashes", "transactionsHashes", "tradeIds", "tradeIDs"}, func() any { return new(PostOrderResponse) }},
		{"open", fmt.Sprintf(`{"id":"id","status":"LIVE","owner":%q,"maker_address":%q,"market":%q,"asset_id":"1","side":"BUY","original_size":"1","size_matched":"0","price":"1","outcome":"yes","created_at":1,"expiration":"1","order_type":"GTC"}`, safetyUUID, safetyAddress, safetyHash), []string{"id", "status", "owner", "maker_address", "market", "asset_id", "side", "original_size", "size_matched", "price", "associate_trades", "outcome", "created_at", "expiration", "order_type"}, func() any { return new(OpenOrderResponse) }},
		{"trade", trade, []string{"id", "taker_order_id", "market", "asset_id", "side", "size", "fee_rate_bps", "price", "status", "match_time", "last_update", "outcome", "bucket_index", "owner", "maker_address", "maker_orders", "transaction_hash", "trader_side", "error_msg"}, func() any { return new(TradeResponse) }},
		{"balance", `{"balance":"1"}`, []string{"balance", "allowances"}, func() any { return new(BalanceAllowanceResponse) }},
		{"maker", maker, []string{"order_id", "owner", "maker_address", "matched_amount", "price", "fee_rate_bps", "asset_id", "outcome", "side"}, func() any { return new(MakerOrder) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.input), tc.newValue()); err != nil {
				t.Fatal(err)
			}
			for _, field := range tc.fields {
				// Appending makes the variant overwrite the canonical field in Go.
				input := strings.TrimSuffix(tc.input, "}") + fmt.Sprintf(`,%q:null}`, strings.ToUpper(field))
				if err := json.Unmarshal([]byte(input), tc.newValue()); err == nil {
					t.Errorf("accepted variant of %s", field)
				}
			}
			input := strings.TrimSuffix(tc.input, "}") + `,"unrelated_extension":true}`
			if err := json.Unmarshal([]byte(input), tc.newValue()); err != nil {
				t.Errorf("unknown field rejected: %v", err)
			}
		})
	}
	for _, variant := range []string{`"MARKET":"bad"`, `"\u004dARKET":"bad"`, `"ſtatus":"bad"`} {
		input := strings.TrimSuffix(trade, "}") + "," + variant + "}"
		if err := json.Unmarshal([]byte(input), new(TradeResponse)); err == nil {
			t.Errorf("accepted %s", variant)
		}
	}
	nested := strings.TrimSuffix(maker, "}") + `,"OWNER":"bad"}`
	input := strings.TrimSuffix(trade, "}") + `,"maker_orders":[` + nested + `]}`
	if err := json.Unmarshal([]byte(input), new(TradeResponse)); err == nil {
		t.Error("accepted case-variant nested maker owner")
	}
	for _, duplicate := range []string{
		`{"balance":"1","allowances":{"` + safetyAddress + `":null},"allowances":{}}`,
		`{"balance":"1","\u0062alance":"2"}`,
	} {
		if err := json.Unmarshal([]byte(duplicate), new(BalanceAllowanceResponse)); err == nil {
			t.Errorf("accepted duplicate field: %s", duplicate)
		}
	}
	for _, field := range []string{"transactionHashes", "transactionsHashes", "tradeIds", "tradeIDs"} {
		value := `["id"]`
		if strings.Contains(field, "Hashes") {
			value = `["` + safetyHash + `"]`
		}
		input := strings.TrimSuffix(cases[0].input, "}") + fmt.Sprintf(`,%q:%s}`, field, value)
		if err := json.Unmarshal([]byte(input), new(PostOrderResponse)); err != nil {
			t.Errorf("valid alias %s: %v", field, err)
		}
	}
}

func TestSafetyPostOrderDefaultsOnReuse(t *testing.T) {
	var response PostOrderResponse
	first := `{"makingAmount":"1","takingAmount":"1","orderID":"first","status":"LIVE","success":true,"transactionHashes":["` + safetyHash + `"],"tradeIds":["trade"]}`
	second := `{"makingAmount":"","takingAmount":"","orderID":"second","status":"LIVE","success":false}`
	if err := json.Unmarshal([]byte(first), &response); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(second), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.TransactionHashes) != 0 || len(response.TradeIDs) != 0 {
		t.Fatalf("defaulted collections retained previous response: %+v", response)
	}
}

func TestSafetyUnmarshalHooks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"post", new(PostOrderResponse)},
		{"open", new(OpenOrderResponse)},
		{"trade", new(TradeResponse)},
		{"balance", new(BalanceAllowanceResponse)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{}`), tc.value); err == nil {
				t.Fatal("accepted response missing required fields")
			}
		})
	}
}

func TestSafetyMakerOrder(t *testing.T) {
	input := fmt.Sprintf(`{"order_id":"id","owner":%q,"maker_address":%q,"matched_amount":"1","price":"1","fee_rate_bps":"0","asset_id":"1","outcome":"yes","side":"BUY"}`, safetyUUID, safetyAddress)
	var order MakerOrder
	if err := json.Unmarshal([]byte(input), &order); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"order_id", "owner", "maker_address", "matched_amount", "price", "fee_rate_bps", "asset_id", "outcome", "side"} {
		for _, null := range []bool{false, true} {
			obj := safetyObject(t, input)
			if null {
				obj[field] = nil
			} else {
				delete(obj, field)
			}
			b, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &order); err == nil {
				t.Errorf("field %s null=%v accepted", field, null)
			}
		}
	}
	if err := json.Unmarshal([]byte(strings.Replace(input, safetyUUID, "bad", 1)), &order); err == nil {
		t.Error("invalid owner accepted")
	}
	if err := json.Unmarshal([]byte(strings.Replace(input, safetyAddress, "", 1)), &order); err == nil {
		t.Error("empty maker address accepted")
	}
}
