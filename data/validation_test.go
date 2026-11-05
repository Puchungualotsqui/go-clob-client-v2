package data

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const testAddress = "0x1234567890abcdef1234567890abcdef12345678"
const testHash = "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
const testHash2 = "0x1111111111111111111111111111111111111111111111111111111111111111"
const testTrade = `{"proxyWallet":"` + testAddress + `","side":"BUY","asset":"123456789012345678901234567890","conditionId":"` + testHash + `","size":50,"price":0.1234567890123456789,"timestamp":1703980800,"title":"Market","slug":"market","icon":"","eventSlug":"event","outcome":"Yes","outcomeIndex":0,"name":"","transactionHash":"` + testHash2 + `"}`
const testPosition = `{"proxyWallet":"` + testAddress + `","asset":"` + testHash2 + `","conditionId":"` + testHash + `","size":100.5,"avgPrice":0.65,"initialValue":65.325,"currentValue":70.35,"cashPnl":5.025,"percentPnl":7.69,"totalBought":100.5,"realizedPnl":0,"percentRealizedPnl":0,"curPrice":0.7,"redeemable":false,"mergeable":false,"title":"Market","slug":"market","icon":"","eventSlug":"event","eventId":"","outcome":"Yes","outcomeIndex":0,"oppositeOutcome":"No","oppositeAsset":"` + testHash2 + `","endDate":"","negativeRisk":false}`

func TestResponseRequiredFields(t *testing.T) {
	fixtures := []struct {
		dst      any
		body     string
		optional []string
	}{
		{&Health{}, `{"data":"OK"}`, nil},
		{&ApiError{}, `{"error":"bad"}`, nil},
		{&Position{}, testPosition, []string{"eventId", "endDate"}},
		{&ClosedPosition{}, `{"proxyWallet":"` + testAddress + `","asset":"1","conditionId":"` + testHash + `","avgPrice":0.5,"totalBought":10,"realizedPnl":1,"curPrice":1,"timestamp":0,"title":"","slug":"","icon":"","eventSlug":"","outcome":"Yes","outcomeIndex":0,"oppositeOutcome":"No","oppositeAsset":"2","endDate":"2025-12-31T00:00:00Z"}`, nil},
		{&Trade{}, testTrade, []string{"name"}},
		{&Activity{}, `{"proxyWallet":"` + testAddress + `","timestamp":0,"type":"REWARD","size":0,"usdcSize":0,"transactionHash":"` + testHash + `"}`, nil},
		{&Holder{}, `{"proxyWallet":"` + testAddress + `","asset":"123","amount":"1.25","outcomeIndex":0}`, nil},
		{&MetaHolder{}, `{"token":"1","holders":[]}`, nil},
		{&Traded{}, `{"user":"` + testAddress + `","traded":0}`, nil},
		{&Value{}, `{"user":"` + testAddress + `","value":0}`, nil},
		{&OpenInterest{}, `{"market":"GLOBAL","value":1}`, nil},
		{&MarketVolume{}, `{"market":"` + testHash + `","value":1}`, nil},
		{&LiveVolume{}, `{"total":1,"markets":[]}`, []string{"markets"}},
		{&BuilderLeaderboardEntry{}, `{"rank":"1","builder":"A","volume":1,"activeUsers":0,"verified":false}`, nil},
		{&BuilderVolumeEntry{}, `{"rank":"1","builder":"A","volume":1,"activeUsers":0,"verified":false,"dt":"2025-01-15T00:00:00Z"}`, nil},
		{&TraderLeaderboardEntry{}, `{"rank":"1","proxyWallet":"` + testAddress + `","vol":1,"pnl":-1}`, nil},
	}
	for _, tc := range fixtures {
		t.Run(reflect.TypeOf(tc.dst).String(), func(t *testing.T) {
			fresh := func() any { return reflect.New(reflect.TypeOf(tc.dst).Elem()).Interface() }
			if err := json.Unmarshal([]byte(tc.body), fresh()); err != nil {
				t.Fatalf("valid: %v", err)
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.body), &obj); err != nil {
				t.Fatal(err)
			}
			for key, value := range obj {
				optional := false
				for _, o := range tc.optional {
					if key == o {
						optional = true
					}
				}
				if optional {
					continue
				}
				delete(obj, key)
				b, _ := json.Marshal(obj)
				if err := json.Unmarshal(b, fresh()); err == nil {
					t.Errorf("missing %s accepted", key)
				}
				obj[key] = json.RawMessage("null")
				b, _ = json.Marshal(obj)
				if err := json.Unmarshal(b, fresh()); err == nil {
					t.Errorf("null %s accepted", key)
				}
				obj[key] = value
			}
			if err := json.Unmarshal([]byte(`null`), fresh()); err == nil {
				t.Error("null object accepted")
			}
		})
	}
}

func TestIdentifiersAndNumbers(t *testing.T) {
	max := "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	for _, tc := range []struct {
		s     string
		valid bool
	}{
		{"0", true}, {max, true}, {"0x" + strings.Repeat("f", 64), true},
		{"115792089237316195423570985008687907853269984665640564039457584007913129639936", false},
		{"-1", false}, {"1.0", false}, {"1e2", false}, {"0x", false}, {"0xgg", false},
	} {
		body := `{"token":"` + tc.s + `","holders":[]}`
		var v MetaHolder
		err := json.Unmarshal([]byte(body), &v)
		if (err == nil) != tc.valid {
			t.Errorf("U256 %q: %v", tc.s, err)
		}
	}
	for _, tc := range []struct {
		s     string
		valid bool
	}{
		{"0", true}, {"-0.125", true}, {"1e-28", true}, {"79228162514264337593543950335", true},
		{"79228162514264337593543950336", false}, {"1e-29", false}, {"1/2", false}, {"NaN", false}, {"", false}, {"0x12", false},
	} {
		body := `{"user":"` + testAddress + `","value":"` + tc.s + `"}`
		var v Value
		err := json.Unmarshal([]byte(body), &v)
		if (err == nil) != tc.valid {
			t.Errorf("decimal %q: %v", tc.s, err)
		}
	}
	for _, body := range []string{
		`{"user":"0x1234","value":1}`, `{"market":"not-a-hash","value":1}`,
		strings.Replace(testTrade, testHash, "0x1234", 1),
		strings.Replace(testPosition, `"endDate":""`, `"endDate":"2025-02-30"`, 1),
	} {
		var dst any
		switch {
		case strings.Contains(body, `"market"`):
			dst = &OpenInterest{}
		case strings.Contains(body, `"user"`):
			dst = &Value{}
		case strings.Contains(body, `"side"`):
			dst = &Trade{}
		default:
			dst = &Position{}
		}
		if err := json.Unmarshal([]byte(body), dst); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	b, err := json.Marshal(Number("0.100000000000000001"))
	if err != nil || string(b) != `"0.100000000000000001"` {
		t.Fatalf("marshal: %s %v", b, err)
	}
	if _, err := json.Marshal(Number("garbage")); err == nil {
		t.Fatal("invalid number marshaled")
	}
}

func TestNestedAndRepeatedDecoding(t *testing.T) {
	var h MetaHolder
	valid := `{"token":"1","holders":[{"proxyWallet":"` + testAddress + `","asset":"2","amount":1,"outcomeIndex":0,"name":"Alice"}]}`
	if err := json.Unmarshal([]byte(valid), &h); err != nil || len(h.Holders) != 1 || h.Holders[0].Name == nil {
		t.Fatalf("valid nested holder: %+v %v", h, err)
	}
	invalid := strings.Replace(valid, `"amount":1,`, "", 1)
	if err := json.Unmarshal([]byte(invalid), &h); err == nil || len(h.Holders) != 1 || h.Holders[0].Name == nil {
		t.Fatalf("invalid nested holder changed receiver: %+v %v", h, err)
	}
	var a Activity
	base := `{"proxyWallet":"` + testAddress + `","timestamp":0,"type":"REWARD","size":0,"usdcSize":0,"transactionHash":"` + testHash + `"`
	if err := json.Unmarshal([]byte(base+`,"title":"first"}`), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(base+`}`), &a); err != nil || a.Title != nil {
		t.Fatalf("stale optional field: %+v %v", a, err)
	}
}

func TestResponseIgnoresUnvalidatedCaseVariants(t *testing.T) {
	var v Position
	body := strings.TrimSuffix(testPosition, "}") + `,"proxywallet":"bad","conditionid":"bad"}`
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	if v.ProxyWallet != testAddress || v.ConditionID != testHash {
		t.Fatalf("unvalidated fields overwrote response: %+v", v)
	}
	var a Activity
	body = `{"proxyWallet":"` + testAddress + `","timestamp":0,"type":"REWARD","size":0,"usdcSize":0,"transactionHash":"` + testHash + `","conditionid":"bad"}`
	if err := json.Unmarshal([]byte(body), &a); err != nil || a.ConditionID != nil {
		t.Fatalf("unvalidated optional field decoded: %+v, %v", a, err)
	}
}

func TestTimestampTimezoneOffsets(t *testing.T) {
	closed := `{"proxyWallet":"` + testAddress + `","asset":"1","conditionId":"` + testHash + `","avgPrice":0.5,"totalBought":10,"realizedPnl":1,"curPrice":1,"timestamp":0,"title":"","slug":"","icon":"","eventSlug":"","outcome":"Yes","outcomeIndex":0,"oppositeOutcome":"No","oppositeAsset":"2","endDate":"%s"}`
	builder := `{"rank":"1","builder":"A","volume":1,"activeUsers":0,"verified":false,"dt":"%s"}`
	for _, tc := range []struct {
		stamp string
		valid bool
	}{
		{"2025-12-31T12:34:56Z", true},
		{"2025-12-31T12:34:56.123456789+05:30", true},
		{"2025-12-31T12:34:56-23:59", true},
		{"2025-12-31T12:34:56+23:59", true},
		{"2025-12-31T12:34:56+24:00", false},
		{"2025-12-31T12:34:56-24:00", false},
		{"2025-12-31T12:34:56+01:60", false},
		{"2025-12-31T12:34:56-00:60", false},
		{"2025-12-31T12:34:56+25:00", false},
	} {
		for _, fixture := range []struct {
			name, body string
			dst        any
		}{{"endDate", closed, &ClosedPosition{}}, {"dt", builder, &BuilderVolumeEntry{}}} {
			t.Run(fixture.name+"/"+tc.stamp, func(t *testing.T) {
				err := json.Unmarshal([]byte(fmt.Sprintf(fixture.body, tc.stamp)), fixture.dst)
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%v: %v", tc.valid, err)
				}
			})
		}
	}
}

func TestResponseDuplicateFields(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		dst        any
		valid      bool
	}{
		{"required", `{"data":"first","data":"second"}`, &Health{}, false},
		{"escaped key", `{"data":"first","da\u0074a":"second"}`, &Health{}, false},
		{"optional after null", strings.TrimSuffix(strings.Replace(testTrade, `"name":""`, `"name":null`, 1), "}") + `,"name":"second"}`, &Trade{}, false},
		{"optional nulls", `{"total":1,"markets":null,"markets":null}`, &LiveVolume{}, false},
		{"nested", `{"token":"1","holders":[{"proxyWallet":"` + testAddress + `","asset":"2","amount":1,"amount":2,"outcomeIndex":0}]}`, &MetaHolder{}, false},
		{"unknown duplicate", `{"data":"OK","extra":1,"extra":2}`, &Health{}, true},
		{"case variant unknown", `{"data":"OK","Data":"ignored","Data":"ignored"}`, &Health{}, true},
		{"nested unknown duplicate", `{"data":"OK","extra":{"x":1,"x":2}}`, &Health{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.body), tc.dst)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestRequestValidationBeforeHTTP(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, `[]`) }))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	badUser := "0x12"
	badDecimal := Number("1/2")
	for _, req := range []any{
		PositionsRequest{}, ActivityRequest{User: badUser}, TradedRequest{}, ValueRequest{}, ClosedPositionsRequest{},
		TradesRequest{User: &badUser}, TraderLeaderboardRequest{User: &badUser},
		HoldersRequest{Markets: []string{"garbage"}}, OpenInterestRequest{Markets: []string{"garbage"}},
		TradesRequest{Filter: &MarketFilter{Markets: []string{"garbage"}}},
		PositionsRequest{User: testAddress, SizeThreshold: &badDecimal},
		TradesRequest{TradeFilter: &TradeFilter{FilterType: FilterCash, FilterAmount: "-1"}},
	} {
		if err := validateQuery(req); err == nil {
			t.Errorf("accepted %#v", req)
		}
	}
	if _, err := c.Positions(context.Background(), PositionsRequest{}); err == nil || calls != 0 {
		t.Fatalf("HTTP sent for invalid request: calls=%d err=%v", calls, err)
	}
	if err := validateQuery(PositionsRequest{User: testAddress, Filter: &MarketFilter{Markets: []string{testHash}}}); err != nil {
		t.Fatal(err)
	}
}
