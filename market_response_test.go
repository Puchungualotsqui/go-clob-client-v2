package clob

import (
	"encoding/json"
	"strings"
	"testing"
)

const fixtureMarketFields = `"enable_order_book":false,"active":false,"closed":false,"archived":false,"accepting_orders":false,"minimum_order_size":"0","minimum_tick_size":"0","question":"","description":"","market_slug":"","seconds_delay":0,"maker_base_fee":"0","taker_base_fee":"0","notifications_enabled":false,"neg_risk":false,"icon":"","image":"","rewards":{"min_size":"0","max_spread":"0"},"is_50_50_outcome":false`

func TestMarketResponseRequiredFields(t *testing.T) {
	fields := []string{
		"enable_order_book", "active", "closed", "archived", "accepting_orders",
		"minimum_order_size", "minimum_tick_size", "question", "description", "market_slug",
		"seconds_delay", "maker_base_fee", "taker_base_fee", "notifications_enabled", "neg_risk",
		"icon", "image", "rewards", "is_50_50_outcome",
	}
	for _, field := range fields {
		for _, mode := range []string{"missing", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				var obj map[string]json.RawMessage
				if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`}`), &obj); err != nil {
					t.Fatal(err)
				}
				if mode == "missing" {
					delete(obj, field)
				} else {
					obj[field] = json.RawMessage(`null`)
				}
				body, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				var market MarketResponse
				if err := json.Unmarshal(body, &market); err == nil || !strings.Contains(err.Error(), field) {
					t.Fatalf("expected error for %s, got %v", field, err)
				}
			})
		}
	}
}

func TestMarketResponseCollectionsAndOptionalFields(t *testing.T) {
	for _, tc := range []struct {
		name, extra, rewards string
		tokens, tags, rates  int
	}{
		{"absent", "", `{"min_size":"0","max_spread":"0"}`, 0, 0, 0},
		{"null", `,"tokens":null,"tags":null,"accepting_order_timestamp":null,"end_date_iso":null,"game_start_time":null,"condition_id":null,"question_id":null,"fpmm":null,"neg_risk_market_id":null,"neg_risk_request_id":null`, `{"min_size":"0","max_spread":"0","rates":null}`, 0, 0, 0},
		{"empty", `,"tokens":[],"tags":[]`, `{"min_size":"0","max_spread":"0","rates":[]}`, 0, 0, 0},
		{"populated", `,"tokens":[{"token_id":"1","outcome":"yes","price":"0.5"}],"tags":["tag"]`, `{"min_size":"0","max_spread":"0","rates":[{"asset_address":"0x0000000000000000000000000000000000000000","rewards_daily_rate":"1"}]}`, 1, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var market MarketResponse
			fields := strings.Replace(fixtureMarketFields, `{"min_size":"0","max_spread":"0"}`, tc.rewards, 1)
			if err := json.Unmarshal([]byte(`{`+fields+tc.extra+`}`), &market); err != nil {
				t.Fatal(err)
			}
			if market.Tokens == nil || len(market.Tokens) != tc.tokens || market.Tags == nil || len(market.Tags) != tc.tags || market.Rewards.Rates == nil || len(market.Rewards.Rates) != tc.rates {
				t.Fatalf("unexpected collections: %+v", market)
			}
			if market.AcceptingOrderTimestamp != nil || market.EndDateISO != nil || market.GameStartTime != nil || market.ConditionID != nil || market.QuestionID != nil || market.FPMM != nil || market.NegRiskMarketID != nil || market.NegRiskRequestID != nil {
				t.Fatalf("unexpected optional fields: %+v", market)
			}
		})
	}
	for _, rewards := range []string{`{}`, `{"min_size":null,"max_spread":"0"}`, `{"min_size":"0","max_spread":null}`} {
		body := `{` + strings.Replace(fixtureMarketFields, `{"min_size":"0","max_spread":"0"}`, rewards, 1) + `}`
		var market MarketResponse
		if err := json.Unmarshal([]byte(body), &market); err == nil {
			t.Errorf("accepted incomplete rewards: %s", rewards)
		}
	}
}
