package clob

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func builderTradeFixture() string {
	return fmt.Sprintf(`{"id":"trade","tradeType":"TRADE","takerOrderHash":%q,"builder":%q,"market":%q,"assetId":"115792089237316195423570985008687907853269984665640564039457584007913129639935","side":"BUY","size":"0.1234567890123456789012345678","sizeUsdc":"2","price":"0.5","status":"MATCHED","outcome":"yes","outcomeIndex":0,"owner":%q,"maker":%q,"transactionHash":%q,"matchTime":"1","bucketIndex":0,"fee":"0","feeUsdc":"0"}`, safetyHash, safetyAddress, safetyHash, safetyUUID, safetyAddress, safetyHash)
}

func TestKeyFieldsBuilderTrade(t *testing.T) {
	base := builderTradeFixture()
	var trade BuilderTradeResponse
	if err := json.Unmarshal([]byte(base), &trade); err != nil {
		t.Fatal(err)
	}
	if trade.Size != "0.1234567890123456789012345678" || trade.AssetID.String() != "115792089237316195423570985008687907853269984665640564039457584007913129639935" {
		t.Fatalf("lost precision: %+v", trade)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &obj); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "tradeType", "takerOrderHash", "builder", "market", "assetId", "side", "size", "sizeUsdc", "price", "status", "outcome", "outcomeIndex", "owner", "maker", "transactionHash", "matchTime", "bucketIndex", "fee", "feeUsdc"} {
		for _, null := range []bool{false, true} {
			copyObj := make(map[string]json.RawMessage, len(obj))
			for k, v := range obj {
				copyObj[k] = v
			}
			if null {
				copyObj[field] = json.RawMessage("null")
			} else {
				delete(copyObj, field)
			}
			b, err := json.Marshal(copyObj)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &trade); err == nil {
				t.Errorf("accepted %s null=%v", field, null)
			}
		}
	}
	for _, field := range []string{"takerOrderHash", "market", "transactionHash", "builder", "maker", "owner"} {
		copyObj := make(map[string]json.RawMessage, len(obj))
		for k, v := range obj {
			copyObj[k] = v
		}
		copyObj[field] = json.RawMessage(`"bad"`)
		b, err := json.Marshal(copyObj)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &trade); err == nil {
			t.Errorf("accepted malformed %s", field)
		}
	}
	for _, suffix := range []string{`,"errMsg":"a","err_msg":null}`, `,"owner":"` + safetyUUID + `"}`, `,"OWNER":"bad"}`} {
		if err := json.Unmarshal([]byte(strings.TrimSuffix(base, "}")+suffix), &trade); err == nil {
			t.Errorf("accepted %s", suffix)
		}
	}
	for _, suffix := range []string{`,"err_msg":"oops"}`, `,"errMsg":"oops"}`} {
		if err := json.Unmarshal([]byte(strings.TrimSuffix(base, "}")+suffix), &trade); err != nil || trade.ErrMsg == nil || *trade.ErrMsg != "oops" {
			t.Fatalf("alias %s: %+v %v", suffix, trade, err)
		}
	}
	if err := json.Unmarshal([]byte(`null`), &trade); err == nil {
		t.Error("accepted null builder trade")
	}
}

func TestKeyFieldsHeartbeat(t *testing.T) {
	for _, id := range []string{safetyUUID, strings.ReplaceAll(safetyUUID, "-", ""), "{" + safetyUUID + "}", "urn:uuid:" + safetyUUID} {
		var response HeartbeatResponse
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"heartbeat_id":%q,"error":null}`, id)), &response); err != nil || response.HeartbeatID != id {
			t.Errorf("valid %q: %+v %v", id, response, err)
		}
	}
	for _, body := range []string{`{}`, `null`, `{"heartbeat_id":null}`, `{"heartbeat_id":"bad"}`, `{"heartbeat_id":1}`, `{"heartbeat_id":"x` + safetyUUID + `y"}`, `{"heartbeat_id":"` + safetyUUID + `","heartbeat_id":"` + safetyUUID + `"}`, `{"heartbeat_id":"` + safetyUUID + `","HEARTBEAT_ID":"bad"}`} {
		var response HeartbeatResponse
		if err := json.Unmarshal([]byte(body), &response); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestKeyFieldsPages(t *testing.T) {
	base := `{"data":[1],"next_cursor":"","limit":0,"count":0}`
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &obj); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"data", "next_cursor", "limit", "count"} {
		for _, null := range []bool{false, true} {
			copyObj := make(map[string]json.RawMessage, len(obj))
			for k, v := range obj {
				copyObj[k] = v
			}
			if null {
				copyObj[field] = json.RawMessage("null")
			} else {
				delete(copyObj, field)
			}
			b, err := json.Marshal(copyObj)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, new(Page[int])); err == nil {
				t.Errorf("accepted %s null=%v", field, null)
			}
		}
	}
	var page Page[int]
	if err := json.Unmarshal([]byte(base), &page); err != nil || page.Limit != 0 || page.Count != 0 || page.NextCursor != "" {
		t.Fatalf("zero metadata: %+v %v", page, err)
	}
	var optional Page[*int]
	if err := json.Unmarshal([]byte(`{"data":[1,null],"next_cursor":"x","limit":2,"count":2}`), &optional); err != nil || len(optional.Data) != 2 || optional.Data[1] != nil {
		t.Fatalf("nullable entries: %+v %v", optional, err)
	}
	for _, target := range []any{new(Page[struct{ Name string }]), new(Page[map[string]string]), new(Page[[]int])} {
		if err := json.Unmarshal([]byte(`{"data":[null],"next_cursor":"x","limit":1,"count":1}`), target); err == nil {
			t.Errorf("accepted null entry in %T", target)
		}
	}
	for _, body := range []string{`{"data":[],"next_cursor":"x","limit":-1,"count":0}`, `{"data":[],"next_cursor":"x","limit":1.5,"count":0}`, `{"data":[],"next_cursor":"x","limit":0,"count":18446744073709551616}`, `{"data":[],"next_cursor":"x","limit":0,"count":0,"COUNT":1}`} {
		if err := json.Unmarshal([]byte(body), &page); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestKeyFieldsRequestHashes(t *testing.T) {
	valid, invalid := B256(safetyHash), B256("0x1234")
	for _, tc := range []struct {
		name    string
		request func(*B256) any
	}{
		{"cancel", func(b *B256) any { return CancelMarketOrderRequest{Market: b} }},
		{"orders", func(b *B256) any { return OrdersRequest{Market: b} }},
		{"trades", func(b *B256) any { return TradesRequest{Market: b} }},
		{"embedded-builder", func(b *B256) any {
			return struct {
				TradesRequest
				BuilderCode B256 `json:"builder_code"`
			}{TradesRequest{Market: b}, valid}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, h := range []*B256{nil, &valid} {
				if _, err := json.Marshal(tc.request(h)); err != nil {
					t.Fatalf("valid: %v", err)
				}
			}
			if _, err := json.Marshal(tc.request(&invalid)); err == nil {
				t.Error("accepted invalid hash")
			}
			if _, err := QueryParams(tc.request(&invalid), nil); err == nil {
				t.Error("query accepted invalid hash")
			}
		})
	}
	q, err := QueryParams(struct {
		TradesRequest
		BuilderCode B256 `json:"builder_code"`
	}{TradesRequest{Market: &valid}, valid}, nil)
	if err != nil || !strings.Contains(q, "builder_code="+safetyHash) || !strings.Contains(q, "market="+safetyHash) {
		t.Fatalf("embedded query %q: %v", q, err)
	}
	for _, request := range []any{RFQRequestsRequest{Markets: []B256{valid, invalid}}, RFQQuotesRequest{Markets: []B256{invalid}}} {
		if _, err := json.Marshal(request); err == nil {
			t.Error("accepted invalid RFQ market")
		}
		if _, err := QueryParams(request, nil); err == nil {
			t.Error("query accepted invalid RFQ market")
		}
	}
	for _, request := range []any{RFQRequestsRequest{Markets: []B256{valid}}, RFQQuotesRequest{Markets: []B256{valid}}} {
		if _, err := json.Marshal(request); err != nil {
			t.Fatalf("valid RFQ market: %v", err)
		}
	}
}
