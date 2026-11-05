package clob

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestClobCompatRequiredFields(t *testing.T) {
	request := fmt.Sprintf(`{"requestId":"r","userAddress":%q,"proxyAddress":%q,"condition":%q,"token":"115792089237316195423570985008687907853269984665640564039457584007913129639935","complement":"0","side":"BUY","sizeIn":"0.1234567890123456789012345678","sizeOut":"2","price":"0.5","expiry":0}`, safetyAddress, safetyAddress, safetyHash)
	quote := strings.TrimSuffix(request, `,"expiry":0}`) + `,"quoteId":"q"}`
	cases := []struct {
		name, body string
		fields     []string
		newValue   func() any
	}{
		{"rfq-create-request", `{"requestId":"r","expiry":0}`, []string{"requestId", "expiry"}, func() any { return new(CreateRFQRequestResponse) }},
		{"rfq-create-quote", `{"quoteId":"q"}`, []string{"quoteId"}, func() any { return new(CreateRFQQuoteResponse) }},
		{"rfq-approve", `{"tradeIds":[]}`, []string{"tradeIds"}, func() any { return new(ApproveRFQOrderResponse) }},
		{"rfq-request", request, []string{"requestId", "userAddress", "proxyAddress", "condition", "token", "complement", "side", "sizeIn", "sizeOut", "price", "expiry"}, func() any { return new(RFQRequest) }},
		{"rfq-quote", quote, []string{"quoteId", "requestId", "userAddress", "proxyAddress", "condition", "token", "complement", "side", "sizeIn", "sizeOut", "price"}, func() any { return new(RFQQuote) }},
		{"book-level", `{"price":"0.1","size":"0"}`, []string{"price", "size"}, func() any { return new(OrderSummary) }},
		{"compact-token", `{"t":"1","o":"yes"}`, []string{"t", "o"}, func() any { return new(ClobToken) }},
		{"token", `{"token_id":"1","outcome":"yes","price":"0.5"}`, []string{"token_id", "outcome", "price"}, func() any { return new(Token) }},
		{"earning", fmt.Sprintf(`{"asset_address":%q,"earnings":"0","asset_rate":"1"}`, safetyAddress), []string{"asset_address", "earnings", "asset_rate"}, func() any { return new(Earning) }},
		{"reward-rate", fmt.Sprintf(`{"asset_address":%q,"rewards_daily_rate":"0"}`, safetyAddress), []string{"asset_address", "rewards_daily_rate"}, func() any { return new(RewardRate) }},
		{"reward-config", fmt.Sprintf(`{"asset_address":%q,"start_date":"2024-02-29","end_date":"2025-01-01","rate_per_day":"1","total_rewards":"2"}`, safetyAddress), []string{"asset_address", "start_date", "end_date", "rate_per_day", "total_rewards"}, func() any { return new(RewardsConfig) }},
		{"user-earning", fmt.Sprintf(`{"date":"2024-02-29","condition_id":%q,"asset_address":%q,"maker_address":%q,"earnings":"1","asset_rate":"2"}`, safetyHash, safetyAddress, safetyAddress), []string{"date", "condition_id", "asset_address", "maker_address", "earnings", "asset_rate"}, func() any { return new(UserEarningResponse) }},
		{"total-earning", fmt.Sprintf(`{"date":"2024-02-29","asset_address":%q,"maker_address":%q,"earnings":"1","asset_rate":"2"}`, safetyAddress, safetyAddress), []string{"date", "asset_address", "maker_address", "earnings", "asset_rate"}, func() any { return new(TotalUserEarningResponse) }},
		{"rewards", `{"min_size":"0","max_spread":"0"}`, []string{"min_size", "max_spread"}, func() any { return new(Rewards) }},
		{"market-rewards-config", `{"id":1,` + fixtureRewardsConfigFields + `}`, []string{"id", "asset_address", "start_date", "end_date", "rate_per_day", "total_rewards", "total_days"}, func() any { return new(MarketRewardsConfig) }},
		{"current-reward", `{` + fixtureCurrentRewardFields + `}`, []string{"condition_id", "rewards_max_spread", "rewards_min_size"}, func() any { return new(CurrentRewardResponse) }},
		{"market-reward", `{` + fixtureMarketRewardFields + `}`, []string{"condition_id", "question", "market_slug", "event_slug", "image", "rewards_max_spread", "rewards_min_size", "market_competitiveness"}, func() any { return new(MarketRewardResponse) }},
		{"user-reward", `{` + fixtureUserRewardFields + `}`, []string{"condition_id", "question", "market_slug", "event_slug", "image", "rewards_max_spread", "rewards_min_size", "market_competitiveness", "maker_address", "earning_percentage"}, func() any { return new(UserRewardsEarningResponse) }},
		{"simplified-market", `{` + fixtureSimpleFields + `}`, []string{"rewards", "active", "closed", "archived", "accepting_orders"}, func() any { return new(SimplifiedMarketResponse) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.body), tc.newValue()); err != nil {
				t.Fatalf("valid: %v", err)
			}
			for _, field := range tc.fields {
				for _, null := range []bool{false, true} {
					var obj map[string]json.RawMessage
					if err := json.Unmarshal([]byte(tc.body), &obj); err != nil {
						t.Fatal(err)
					}
					if null {
						obj[field] = json.RawMessage("null")
					} else {
						delete(obj, field)
					}
					b, err := json.Marshal(obj)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(b, tc.newValue()); err == nil {
						t.Errorf("accepted %s null=%v: %s", field, null, b)
					}
				}
			}
			if err := json.Unmarshal([]byte(`null`), tc.newValue()); err == nil {
				t.Error("accepted null object")
			}
		})
	}
}

func TestClobCompatInvalidNestedAndIdentifiers(t *testing.T) {
	cases := []struct {
		name, body string
		newValue   func() any
	}{
		{"rfq address", fmt.Sprintf(`{"requestId":"r","userAddress":"","proxyAddress":%q,"condition":%q,"token":"1","complement":"0","side":"BUY","sizeIn":"1","sizeOut":"2","price":"0.5","expiry":0}`, safetyAddress, safetyHash), func() any { return new(RFQRequest) }},
		{"rfq hash", fmt.Sprintf(`{"quoteId":"q","requestId":"r","userAddress":%q,"proxyAddress":%q,"condition":"bad","token":"1","complement":"0","side":"BUY","sizeIn":"1","sizeOut":"2","price":"0.5"}`, safetyAddress, safetyAddress), func() any { return new(RFQQuote) }},
		{"rfq list null item", `{"tradeIds":[null]}`, func() any { return new(ApproveRFQOrderResponse) }},
		{"duplicate", `{"requestId":"r","requestId":"x","expiry":0}`, func() any { return new(CreateRFQRequestResponse) }},

		{"book null level", `{"neg_risk":false,"asks":[null]}`, func() any { return new(OrderBookSummaryResponse) }},
		{"book incomplete level", `{"neg_risk":false,"bids":[{"price":"1"}]}`, func() any { return new(OrderBookSummaryResponse) }},
		{"compact token", fmt.Sprintf(`{"c":%q,"mts":"0.01","t":[{"t":"1"}]}`, safetyHash), func() any { return new(ClobMarketInfoResponse) }},
		{"compact null fee", fmt.Sprintf(`{"c":%q,"mts":"0.01","fd":{"e":null}}`, safetyHash), func() any { return new(ClobMarketInfoResponse) }},
		{"reward bad hash", `{` + strings.Replace(fixtureCurrentRewardFields, safetyHash, "bad", 1) + `}`, func() any { return new(CurrentRewardResponse) }},
		{"reward bad address", `{` + strings.Replace(fixtureUserRewardFields, safetyAddress, "", 1) + `}`, func() any { return new(UserRewardsEarningResponse) }},
		{"reward null scalar", `{` + strings.Replace(fixtureMarketRewardFields, `"question":"q"`, `"question":null`, 1) + `}`, func() any { return new(MarketRewardResponse) }},
		{"reward null config", `{` + fixtureCurrentRewardFields + `,"rewards_config":[null]}`, func() any { return new(CurrentRewardResponse) }},
		{"reward null token", `{` + fixtureMarketRewardFields + `,"tokens":[null]}`, func() any { return new(MarketRewardResponse) }},
		{"reward null earning", `{` + fixtureUserRewardFields + `,"earnings":[null]}`, func() any { return new(UserRewardsEarningResponse) }},
		{"reward date", fmt.Sprintf(`{"asset_address":%q,"start_date":"2023-02-29","end_date":"2025-01-01","rate_per_day":"1","total_rewards":"2"}`, safetyAddress), func() any { return new(RewardsConfig) }},
		{"market config date", `{"id":1,` + strings.Replace(fixtureRewardsConfigFields, "2024-01-01", "2024-13-01", 1) + `}`, func() any { return new(MarketRewardsConfig) }},
		{"token null default", `{"token_id":"1","outcome":"yes","price":"1","winner":null}`, func() any { return new(Token) }},
		{"market config missing id", `{}`, func() any { return new(MarketRewardsConfig) }},
		{"page missing data", `{"next_cursor":"end"}`, func() any { return new(Page[RFQQuote]) }},
		{"page null data", `{"data":null}`, func() any { return new(Page[RFQQuote]) }},
		{"page null cursor", `{"data":[],"next_cursor":null}`, func() any { return new(Page[RFQQuote]) }},
		{"page null count", `{"data":[],"count":null}`, func() any { return new(Page[RFQQuote]) }},
		{"page null entry", `{"data":[null],"next_cursor":"end","limit":1,"count":1}`, func() any { return new(Page[RFQQuote]) }},
		{"simplified page null entry", `{"data":[null],"next_cursor":"end","limit":1,"count":1}`, func() any { return new(Page[SimplifiedMarketResponse]) }},
		{"rewards null scalar", `{"min_size":null}`, func() any { return new(Rewards) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.body), tc.newValue()); err == nil {
				t.Fatalf("accepted %s", tc.body)
			}
		})
	}
}

func TestClobCompatRequestWire(t *testing.T) {
	max := U256("115792089237316195423570985008687907853269984665640564039457584007913129639935")
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"midpoint", MidpointRequest{TokenID: max}, `{"token_id":"115792089237316195423570985008687907853269984665640564039457584007913129639935"}`},
		{"cancel-null", CancelMarketOrderRequest{}, `{"market":null,"asset_id":null}`},
		{"rfq-create", CreateRFQRequestRequest{AssetOut: max, AmountIn: "0.1234567890123456789012345678", AmountOut: "2.000", UserType: Proxy}, `{"assetIn":"0","assetOut":"115792089237316195423570985008687907853269984665640564039457584007913129639935","amountIn":"0.1234567890123456789012345678","amountOut":"2.000","userType":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.value)
			if err != nil || string(b) != tc.want {
				t.Fatalf("got %s, %v; want %s", b, err, tc.want)
			}
		})
	}
	for _, invalid := range []TimeRange{{}, {Interval: func() *Interval { v := OneDay; return &v }(), StartTS: func() *int64 { v := int64(0); return &v }()}} {
		if _, err := json.Marshal(PriceHistoryRequest{Market: max, TimeRange: invalid}); err == nil {
			t.Error("accepted invalid time range")
		}
	}
	d := Decimal("0.1234567890123456789012345678")
	q, err := QueryParams(RFQQuotesRequest{QuoteIDs: []string{"a+b", "x/y"}, PriceMin: &d}, nil)
	if err != nil || q != "?priceMin=0.1234567890123456789012345678&quoteIds=a%2Bb&quoteIds=x%2Fy" {
		t.Fatalf("query %q: %v", q, err)
	}
}

func TestClobCompatDefaultsAndExactNumbers(t *testing.T) {
	for _, value := range []string{`{` + fixtureUserRewardFields + `}`, `{` + fixtureUserRewardFields + `,"tokens":null,"rewards_config":null,"earnings":null}`} {
		var r UserRewardsEarningResponse
		if err := json.Unmarshal([]byte(value), &r); err != nil || r.Tokens == nil || r.RewardsConfig == nil || r.Earnings == nil {
			t.Fatalf("defaults: %+v %v", r, err)
		}
	}
	var r RFQQuote
	body := fmt.Sprintf(`{"quoteId":"q","requestId":"r","userAddress":%q,"proxyAddress":%q,"condition":%q,"token":"115792089237316195423570985008687907853269984665640564039457584007913129639935","complement":"0","side":"BUY","sizeIn":0.1234567890123456789012345678,"sizeOut":"2.000","price":"0.5"}`, safetyAddress, safetyAddress, safetyHash)
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	if r.SizeIn != "0.1234567890123456789012345678" || r.SizeOut != "2.000" || r.Token.String() != "115792089237316195423570985008687907853269984665640564039457584007913129639935" {
		t.Fatalf("lost precision: %+v", r)
	}
	for _, bad := range []string{`"-1"`, `"115792089237316195423570985008687907853269984665640564039457584007913129639936"`, `null`} {
		input := strings.Replace(body, `"complement":"0"`, `"complement":`+bad, 1)
		if err := json.Unmarshal([]byte(input), &r); err == nil {
			t.Errorf("accepted complement %s", bad)
		}
	}
	var existing CreateRFQRequestResponse
	if err := json.Unmarshal([]byte(`{"requestId":"first","expiry":1}`), &existing); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"requestId":null,"expiry":2}`), &existing); err == nil || existing.RequestID != "first" || existing.Expiry != 1 {
		t.Fatalf("failed decode mutated receiver: %+v %v", existing, err)
	}
}
