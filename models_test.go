package clob

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const fixturePostFields = `"makingAmount":"0","takingAmount":"0","orderID":"id","status":"LIVE","success":true`
const fixtureOpen = `{"id":"id","status":"LIVE","owner":"00000000-0000-0000-0000-000000000001","maker_address":"0x1111111111111111111111111111111111111111","market":"0x1111111111111111111111111111111111111111111111111111111111111111","asset_id":"1","side":"BUY","original_size":"1","size_matched":"0","price":"1","outcome":"yes","created_at":123,"expiration":"456","order_type":"GTC"}`

const fixtureRewardsConfigFields = `"asset_address":"` + safetyAddress + `","start_date":"2024-01-01","end_date":"2024-12-31","rate_per_day":"1","total_rewards":"2","total_days":"365"`
const fixtureCurrentRewardFields = `"condition_id":"` + safetyHash + `","rewards_max_spread":"0.1","rewards_min_size":"1"`
const fixtureMarketRewardFields = fixtureCurrentRewardFields + `,"question":"q","market_slug":"m","event_slug":"e","image":"","market_competitiveness":"0"`
const fixtureUserRewardFields = fixtureMarketRewardFields + `,"maker_address":"` + safetyAddress + `","earning_percentage":"0"`
const fixtureSimpleFields = `"rewards":{"min_size":"0","max_spread":"0"},"active":false,"closed":false,"archived":false,"accepting_orders":false`

func fixtureTrade(id, status, hash string) string {
	return fmt.Sprintf(`{"id":%q,"taker_order_id":"order","market":"0x1111111111111111111111111111111111111111111111111111111111111111","asset_id":"1","side":"BUY","size":"1","fee_rate_bps":"0","price":"1","status":%q,"match_time":"1","last_update":"1","outcome":"yes","bucket_index":0,"owner":"00000000-0000-0000-0000-000000000001","maker_address":"0x1111111111111111111111111111111111111111","trader_side":"TAKER"%s}`, id, status, hash)
}

func TestExactNumbers(t *testing.T) {
	var v struct {
		Price Decimal `json:"price"`
		ID    U256    `json:"id"`
	}
	if err := json.Unmarshal([]byte(`{"price":0.123456789012345678901,"id":"115792089237316195423570985008687907853269984665640564039457584007913129639935"}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.Price != "0.123456789012345678901" {
		t.Fatal(v.Price)
	}
	if _, err := ParseU256("115792089237316195423570985008687907853269984665640564039457584007913129639936"); err == nil {
		t.Fatal("expected overflow")
	}
	if _, err := ParseDecimal("1e3"); err == nil {
		t.Fatal("invalid decimal accepted")
	}
}
func TestAmountsAndTick(t *testing.T) {
	a, err := NewUSDCAmount("1.1234560")
	if err != nil || a.Value() != "1.123456" {
		t.Fatal(a, err)
	}
	if _, err = NewUSDCAmount("0.0000001"); err == nil {
		t.Fatal("excess precision")
	}
	a, err = NewSharesAmount("12.349")
	if err != nil || a.Value() != "12.34" {
		t.Fatal(a, err)
	}
	if _, err = NewSharesAmount("0.009"); err == nil {
		t.Fatal("expected rounding failure")
	}
	var tick TickSize
	if err = json.Unmarshal([]byte(`0.002500`), &tick); err != nil || tick != QuarterCent {
		t.Fatal(tick, err)
	}
}
func TestConfigAndErrors(t *testing.T) {
	c, ok := ContractConfigFor(Amoy, true)
	if !ok || c.Exchange != mustAddress(t, "0xC5d563A36AE78145C45a50134d48A1215220f80a") || c.NegRiskAdapter == nil {
		t.Fatal(c)
	}
	w, ok := WalletContractConfigFor(Amoy)
	if !ok || w.ProxyFactory != nil {
		t.Fatal(w)
	}
	if _, ok = ContractConfigFor(1, false); ok {
		t.Fatal("unsupported chain")
	}
	err := NewMissingContractConfigError(1, true)
	var inner MissingContractConfigError
	if err.Kind != KindInternal || !errors.As(err, &inner) || inner.ChainID != 1 {
		t.Fatal(err)
	}
	if DefaultWebSocketConfig().HeartbeatTimeout != 15*time.Second {
		t.Fatal("timeout")
	}
}
func TestSignedOrderWire(t *testing.T) {
	f := false
	p := OrderPayload{V3: &OrderPayloadV3{Order: OrderV2{Salt: 7, TokenID: "123", MakerAmount: "99", TakerAmount: "200", Side: 1, Timestamp: "345"}, Expiration: "456"}}
	order, err := p.ToOrder()
	if err != nil {
		t.Fatal(err)
	}
	s := SignedOrder{SignableOrder: SignableOrder{Order: order, OrderType: GTC, PostOnly: &f}, Signature: "0xabcd", Owner: "key"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var o map[string]json.RawMessage
	if err = json.Unmarshal(m["order"], &o); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"salt": "7", "side": `"SELL"`, "tokenId": `"123"`, "expiration": `"456"`, "signature": `"0xabcd"`} {
		if string(o[k]) != v {
			t.Fatalf("%s=%s", k, o[k])
		}
	}
	if string(m["postOnly"]) != "false" || m["deferExec"] != nil {
		t.Fatal(string(b))
	}
	s.Order.Side = 2
	if _, err = json.Marshal(s); err == nil {
		t.Fatal("invalid side")
	}
}
func TestRFQQueriesAndOptionalPrice(t *testing.T) {
	market1, market2 := "0x"+strings.Repeat("01", 32), "0x"+strings.Repeat("02", 32)
	r := RFQQuotesRequest{QuoteIDs: []string{"a", "b"}, Markets: []B256{B256(market1), B256(market2)}}
	q, err := QueryParams(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "quoteIds=a&quoteIds=b") || !strings.Contains(q, "markets="+market1+"%2C"+market2) {
		t.Fatal(q)
	}
	var book OrderBookSummaryResponse
	if err := json.Unmarshal([]byte(`{"timestamp":"1700000000000","tick_size":"0.01","neg_risk":false,"last_trade_price":"not-a-number","bids":null,"asks":null}`), &book); err != nil {
		t.Fatal(err)
	}
	if book.LastTradePrice != nil || book.Bids == nil || book.Asks == nil || book.Timestamp.UnixMilli() != 1700000000000 {
		t.Fatal(book)
	}
}
func TestModelResponseParity(t *testing.T) {
	for _, id := range []string{`123`, `-42`, `18446744073709551615`, `"abc"`} {
		var r MarketRewardsConfig
		if err := json.Unmarshal([]byte(`{"id":`+id+`,`+fixtureRewardsConfigFields+`}`), &r); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		want := strings.Trim(id, `"`)
		if r.ID != want {
			t.Fatalf("id = %q, want %q", r.ID, want)
		}
	}
	for _, id := range []string{`null`, `true`, `1.5`, `18446744073709551616`} {
		var r MarketRewardsConfig
		if err := json.Unmarshal([]byte(`{"id":`+id+`,`+fixtureRewardsConfigFields+`}`), &r); err == nil {
			t.Fatalf("accepted id %s", id)
		}
	}
	var user UserRewardsEarningResponse
	if err := json.Unmarshal([]byte(`{`+fixtureUserRewardFields+`,"tokens":null,"rewards_config":null,"earnings":null}`), &user); err != nil || user.Tokens == nil || user.RewardsConfig == nil || user.Earnings == nil {
		t.Fatalf("user: %+v %v", user, err)
	}
	var market MarketRewardResponse
	if err := json.Unmarshal([]byte(`{`+fixtureMarketRewardFields+`,"rewards_config":[{"id":123,`+fixtureRewardsConfigFields+`}],"tokens":null}`), &market); err != nil || market.Tokens == nil || market.RewardsConfig[0].ID != "123" {
		t.Fatalf("market: %+v %v", market, err)
	}
	var current CurrentRewardResponse
	if err := json.Unmarshal([]byte(`{`+fixtureCurrentRewardFields+`}`), &current); err != nil || current.RewardsConfig == nil {
		t.Fatalf("current: %+v %v", current, err)
	}
	var simple SimplifiedMarketResponse
	if err := json.Unmarshal([]byte(`{"condition_id":"","tokens":null,"rewards":{"rates":null,"min_size":"0","max_spread":"0"},"active":false,"closed":false,"archived":false,"accepting_orders":false}`), &simple); err != nil || simple.ConditionID != nil || simple.Tokens == nil || simple.Rewards.Rates == nil {
		t.Fatalf("simple: %+v %v", simple, err)
	}
	var canceled CancelOrdersResponse
	if err := json.Unmarshal([]byte(`{"canceled":null,"not_canceled":null}`), &canceled); err != nil || canceled.Canceled == nil || canceled.NotCanceled == nil {
		t.Fatalf("canceled: %+v %v", canceled, err)
	}
	var post PostOrderResponse
	if err := json.Unmarshal([]byte(`{`+fixturePostFields+`,"transactionHashes":null,"tradeIds":null}`), &post); err != nil || post.TransactionHashes == nil || post.TradeIDs == nil {
		t.Fatalf("post: %+v %v", post, err)
	}
	var open OpenOrderResponse
	if err := json.Unmarshal([]byte(strings.TrimSuffix(fixtureOpen, `}`)+`,"associate_trades":null}`), &open); err != nil || open.AssociateTrades == nil {
		t.Fatalf("open: %+v %v", open, err)
	}
	var trade BuilderTradeResponse
	builderTrade := fmt.Sprintf(`{"id":"id","tradeType":"TRADE","takerOrderHash":%q,"builder":%q,"market":%q,"assetId":"1","side":"BUY","size":"1","sizeUsdc":"2","price":"0.5","status":"MATCHED","outcome":"yes","outcomeIndex":0,"owner":%q,"maker":%q,"transactionHash":%q,"matchTime":"1","bucketIndex":0,"fee":"0","feeUsdc":"0","err_msg":"oops"}`, safetyHash, safetyAddress, safetyHash, safetyUUID, safetyAddress, safetyHash)
	if err := json.Unmarshal([]byte(builderTrade), &trade); err != nil || trade.ErrMsg == nil || *trade.ErrMsg != "oops" {
		t.Fatalf("trade: %+v %v", trade, err)
	}
	var info ClobMarketInfoResponse
	if err := json.Unmarshal([]byte(`{"c":"`+marketTestCondition+`","mts":"0.01","t":null}`), &info); err != nil || info.Tokens == nil {
		t.Fatalf("info: %+v %v", info, err)
	}
	var balance BalanceAllowanceResponse
	if err := json.Unmarshal([]byte(`{"balance":"1"}`), &balance); err != nil || balance.Allowances == nil {
		t.Fatalf("balance: %+v %v", balance, err)
	}
	if err := json.Unmarshal([]byte(`{"balance":"1","allowances":null}`), &balance); err == nil {
		t.Fatal("accepted null allowances")
	}
}

func TestMarketAndTradeHashValidation(t *testing.T) {
	valid := "0x" + strings.Repeat("12", 32)
	for _, field := range []string{"condition_id", "question_id", "neg_risk_request_id"} {
		for _, value := range []string{"", valid} {
			var r MarketResponse
			if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`,"`+field+`":"`+value+`"}`), &r); err != nil {
				t.Fatalf("%s: %v", field, err)
			}
		}
		var r MarketResponse
		if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`,"`+field+`":"0x01"}`), &r); err == nil {
			t.Fatalf("accepted invalid %s", field)
		}
	}
	var m MarketResponse
	if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`,"neg_risk_market_id":"bad"}`), &m); err != nil || m.NegRiskMarketID != nil {
		t.Fatalf("%+v %v", m, err)
	}
	var s SimplifiedMarketResponse
	if err := json.Unmarshal([]byte(`{"condition_id":"bad",`+fixtureSimpleFields+`}`), &s); err == nil {
		t.Fatal("accepted invalid simplified condition")
	}
	var trade TradeResponse
	for _, hash := range []string{`"bad"`, `null`, `123`} {
		if err := json.Unmarshal([]byte(fixtureTrade("id", "MATCHED", `,"transaction_hash":`+hash)), &trade); err == nil {
			t.Fatalf("accepted transaction hash %s", hash)
		}
	}
	if err := json.Unmarshal([]byte(fixtureTrade("id", "MATCHED", `,"transaction_hash":"`+valid+`"`)), &trade); err != nil || string(trade.TransactionHash) != valid {
		t.Fatalf("%+v %v", trade, err)
	}
}

func TestWireAliasesAndQuery(t *testing.T) {
	var r PostOrderResponse
	if err := json.Unmarshal([]byte(`{"makingAmount":"","takingAmount":"2.10","orderID":"o","status":"matched","success":true,"transactionsHashes":["0x1111111111111111111111111111111111111111111111111111111111111111"],"tradeIDs":["a"]}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.MakingAmount.Decimal != "0" || r.Status != OrderMatched || len(r.TransactionHashes) != 1 || len(r.TradeIDs) != 1 {
		t.Fatalf("%+v", r)
	}
	cursor := "a+b/="
	q, err := QueryParams(PriceHistoryRequest{Market: "123", TimeRange: TimestampRange(0, 99)}, &cursor)
	if err != nil || !strings.Contains(q, "startTs=0") || !strings.Contains(q, "next_cursor=a%2Bb%2F%3D") {
		t.Fatal(q, err)
	}
	var order OpenOrderResponse
	if err = json.Unmarshal([]byte(fixtureOpen), &order); err != nil || order.CreatedAt.Unix() != 123 || order.Expiration.Unix() != 456 {
		t.Fatal(order, err)
	}
}
