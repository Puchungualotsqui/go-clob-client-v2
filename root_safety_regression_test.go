package clob

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type pageNullAware string

func (v *pageNullAware) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*v = "null handled"
		return nil
	}
	return json.Unmarshal(b, (*string)(v))
}

func TestRootSafetyPageAtomicAndNullable(t *testing.T) {
	const valid = `{"data":[1,2],"next_cursor":"next","limit":2,"count":2}`
	var page Page[int]
	if err := json.Unmarshal([]byte(valid), &page); err != nil {
		t.Fatal(err)
	}
	before := page
	for _, body := range []string{
		`{"data":[1,null],"next_cursor":"next","limit":2,"count":2}`,
		`{"data":[1,"bad"],"next_cursor":"next","limit":2,"count":2}`,
		`{"data":[],"next_cursor":"next","limit":2,"count":2,"count":3}`,
		`{"data":[],"next_cursor":"next","limit":2}`,
	} {
		if err := json.Unmarshal([]byte(body), &page); err == nil || !reflect.DeepEqual(page, before) {
			t.Errorf("failed decode mutated page: %s: %+v, %v", body, page, err)
		}
	}
	if err := json.Unmarshal([]byte(`{"data":[],"next_cursor":"","limit":0,"count":0}`), &page); err != nil || page.Data == nil || len(page.Data) != 0 || page.NextCursor != "" {
		t.Fatalf("replacement page: %+v, %v", page, err)
	}
	const nullable = `{"data":[null],"next_cursor":"","limit":1,"count":1}`
	var uintptrs Page[uintptr]
	if err := json.Unmarshal([]byte(nullable), &uintptrs); err == nil {
		t.Fatal("null uintptr page item silently became zero")
	}
	var custom Page[pageNullAware]
	if err := json.Unmarshal([]byte(nullable), &custom); err != nil || len(custom.Data) != 1 || custom.Data[0] != "null handled" {
		t.Fatalf("custom null handling: %+v, %v", custom, err)
	}
	var generic Page[any]
	if err := json.Unmarshal([]byte(nullable), &generic); err != nil || len(generic.Data) != 1 || generic.Data[0] != nil {
		t.Fatalf("interface null: %+v, %v", generic, err)
	}
}

func TestRootSafetyRewardDefaultsAndAtomicity(t *testing.T) {
	cases := []struct {
		name, valid, nullCollections, invalid string
		newValue                              func() any
	}{
		{"current", `{` + fixtureCurrentRewardFields + `}`, `,"rewards_config":null`, `,"rewards_config":[{}]`, func() any { return new(CurrentRewardResponse) }},
		{"market", `{` + fixtureMarketRewardFields + `}`, `,"tokens":null,"rewards_config":null`, `,"rewards_config":[{}]`, func() any { return new(MarketRewardResponse) }},
		{"user", `{` + fixtureUserRewardFields + `}`, `,"tokens":null,"rewards_config":null,"earnings":null`, `,"earnings":[{}]`, func() any { return new(UserRewardsEarningResponse) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, suffix := range []string{"", tc.nullCollections} {
				body := strings.TrimSuffix(tc.valid, "}") + suffix + "}"
				v := tc.newValue()
				if err := json.Unmarshal([]byte(body), v); err != nil {
					t.Fatalf("decode %s: %v", body, err)
				}
				value := reflect.ValueOf(v).Elem()
				for _, name := range []string{"Tokens", "RewardsConfig", "Earnings"} {
					field := value.FieldByName(name)
					if field.IsValid() && (field.IsNil() || field.Len() != 0) {
						t.Errorf("%s did not default to empty: %+v", name, v)
					}
				}
			}
			v := tc.newValue()
			if err := json.Unmarshal([]byte(tc.valid), v); err != nil {
				t.Fatal(err)
			}
			before := reflect.ValueOf(v).Elem().Interface()
			invalid := strings.TrimSuffix(tc.valid, "}") + tc.invalid + "}"
			if err := json.Unmarshal([]byte(invalid), v); err == nil || !reflect.DeepEqual(reflect.ValueOf(v).Elem().Interface(), before) {
				t.Fatalf("nested failure mutated receiver: %+v, %v", v, err)
			}
		})
	}
}

func TestRootSafetyRFQCaseVariants(t *testing.T) {
	request := fmt.Sprintf(`{"requestId":"r","userAddress":%q,"proxyAddress":%q,"condition":%q,"token":"1","complement":"0","side":"BUY","sizeIn":"1","sizeOut":"2","price":"0.5","expiry":0}`, safetyAddress, safetyAddress, safetyHash)
	quote := strings.TrimSuffix(request, `,"expiry":0}`) + `,"quoteId":"q"}`
	for _, tc := range []struct {
		name, body string
		fields     []string
		newValue   func() any
	}{
		{"create request", `{"requestId":"r","expiry":0}`, []string{"requestId", "expiry"}, func() any { return new(CreateRFQRequestResponse) }},
		{"create quote", `{"quoteId":"q"}`, []string{"quoteId"}, func() any { return new(CreateRFQQuoteResponse) }},
		{"approve", `{"tradeIds":["t"]}`, []string{"tradeIds"}, func() any { return new(ApproveRFQOrderResponse) }},
		{"request", request, []string{"requestId", "userAddress", "proxyAddress", "condition", "token", "complement", "side", "sizeIn", "sizeOut", "price", "expiry"}, func() any { return new(RFQRequest) }},
		{"quote", quote, []string{"quoteId", "requestId", "userAddress", "proxyAddress", "condition", "token", "complement", "side", "sizeIn", "sizeOut", "price"}, func() any { return new(RFQQuote) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.newValue()
			if err := json.Unmarshal([]byte(tc.body), want); err != nil {
				t.Fatal(err)
			}
			for _, field := range tc.fields {
				variant := strings.ToUpper(field)
				for _, body := range []string{
					`{"` + variant + `":[],` + strings.TrimPrefix(tc.body, "{"),
					strings.TrimSuffix(tc.body, "}") + `,"` + variant + `":[]}`,
				} {
					got := tc.newValue()
					if err := json.Unmarshal([]byte(body), got); err != nil || !reflect.DeepEqual(got, want) {
						t.Errorf("variant %s changed response: %+v, %v", field, got, err)
					}
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(tc.body), &fields); err != nil {
					t.Fatal(err)
				}
				fields[variant] = fields[field]
				delete(fields, field)
				body, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, tc.newValue()); err == nil {
					t.Errorf("case variant %s satisfied required field", field)
				}
			}
		})
	}
}

func TestRootSafetyZeroHashWire(t *testing.T) {
	var zero B256
	for _, value := range []any{zero, &zero} {
		b, err := json.Marshal(value)
		if err != nil || string(b) != `"`+zeroB256+`"` {
			t.Fatalf("zero hash: %s, %v", b, err)
		}
	}
	q, err := QueryParams(OrdersRequest{Market: &zero}, nil)
	if err != nil || !strings.Contains(q, "market="+zeroB256) {
		t.Fatalf("zero market query: %s, %v", q, err)
	}
	for _, request := range []any{RFQRequestsRequest{Markets: []B256{zero, B256(safetyHash)}}, RFQQuotesRequest{Markets: []B256{zero, B256(safetyHash)}}} {
		b, err := json.Marshal(request)
		if err != nil || !strings.Contains(string(b), `"markets":"`+zeroB256+`,`+safetyHash+`"`) {
			t.Fatalf("RFQ markets: %s, %v", b, err)
		}
		q, err := QueryParams(request, nil)
		if err != nil || !strings.Contains(q, "markets="+zeroB256+"%2C"+safetyHash) {
			t.Fatalf("RFQ query: %s, %v", q, err)
		}
	}
	invalid := B256("bad")
	if _, err := json.Marshal(invalid); err == nil {
		t.Fatal("accepted malformed hash")
	}
}
