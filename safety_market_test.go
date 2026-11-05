package clob

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSafetyRequiredMarketFields(t *testing.T) {
	tests := []struct {
		name, field string
		newValue    func() any
		zero        string
	}{
		{"risk", "neg_risk", func() any { return &NegRiskResponse{} }, "false"},
		{"book", "neg_risk", func() any { return &OrderBookSummaryResponse{} }, "false"},

		{"fee", "base_fee", func() any { return &FeeRateResponse{} }, "0"},
		{"geoblock", "blocked", func() any { return &GeoblockResponse{} }, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, body := range []string{"{}", fmt.Sprintf(`{"%s":null}`, tt.field)} {
				if err := json.Unmarshal([]byte(body), tt.newValue()); err == nil {
					t.Errorf("accepted %s", body)
				}
			}
			body := fmt.Sprintf(`{"%s":%s}`, tt.field, tt.zero)
			if err := json.Unmarshal([]byte(body), tt.newValue()); err != nil {
				t.Errorf("rejected %s: %v", body, err)
			}
		})
	}
	var compact ClobMarketInfoResponse
	base := `"c":"` + marketTestCondition + `","mts":"0.01"`
	if err := json.Unmarshal([]byte(`{`+base+`}`), &compact); err != nil || compact.NegRisk || compact.negRiskPresent {
		t.Fatalf("missing nr: %+v %v", compact, err)
	}
	if err := json.Unmarshal([]byte(`{`+base+`,"nr":null}`), &compact); err == nil {
		t.Fatal("accepted null nr")
	}
	if err := json.Unmarshal([]byte(`{`+base+`,"nr":false}`), &compact); err != nil || compact.NegRisk || !compact.negRiskPresent {
		t.Fatalf("false nr: %+v %v", compact, err)
	}
}

func TestSafetyRiskAndFeeRejectAmbiguousKeys(t *testing.T) {
	for _, tc := range []struct {
		name     string
		newValue func() any
		invalid  []string
		valid    string
	}{
		{"risk", func() any { return new(NegRiskResponse) }, []string{`{"NEG_RISK":true}`, `{"neg_risk":true,"NEG_RISK":false}`, `{"NEG_RISK":false,"neg_risk":true}`, `{"neg_risk":true,"neg_risk":false}`}, `{"neg_risk":false,"extra":1}`},
		{"fee", func() any { return new(FeeRateResponse) }, []string{`{"BASE_FEE":1}`, `{"base_fee":10,"BASE_FEE":0}`, `{"BASE_FEE":0,"base_fee":10}`, `{"base_fee":10,"base_fee":0}`}, `{"base_fee":0,"extra":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range tc.invalid {
				if err := json.Unmarshal([]byte(body), tc.newValue()); err == nil {
					t.Errorf("accepted %s", body)
				}
			}
			if err := json.Unmarshal([]byte(tc.valid), tc.newValue()); err != nil {
				t.Fatalf("valid response: %v", err)
			}
		})
	}
}

func TestSafetyOtherCriticalFieldsRejectAmbiguousKeys(t *testing.T) {
	for _, tc := range []struct {
		body string
		dst  any
	}{
		{`{"blocked":true,"BLOCKED":false}`, new(GeoblockResponse)},
		{`{"NEG_RISK":false,` + fixtureMarketFields + `}`, new(MarketResponse)},
		{`{"neg_risk":true,"NEG_RISK":false}`, new(OrderBookSummaryResponse)},
		{`{"neg_risk":false,"asset_id":"1","ASSET_ID":"2"}`, new(OrderBookSummaryResponse)},
		{`{"neg_risk":false,"asks":[],"asks":[{"price":"0.9","size":"1"}]}`, new(OrderBookSummaryResponse)},
	} {
		if err := json.Unmarshal([]byte(tc.body), tc.dst); err == nil {
			t.Errorf("accepted ambiguous fields: %s", tc.body)
		}
	}
}

func TestSafetyCompactRequiredFields(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"mts":"0.01"}`,
		`{"c":null,"mts":"0.01"}`,
		`{"c":"bad","mts":"0.01"}`,
		`{"c":"` + marketTestCondition + `"}`,
		`{"c":"` + marketTestCondition + `","mts":null}`,
		`{"c":"` + marketTestCondition + `","mts":"0.02"}`,
	} {
		var info ClobMarketInfoResponse
		if err := json.Unmarshal([]byte(body), &info); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	var info ClobMarketInfoResponse
	body := `{"c":"` + marketTestCondition + `","C":"` + marketTestOldCondition + `","mts":"0.01","MTS":"0.1","nr":true,"NR":false}`
	if err := json.Unmarshal([]byte(body), &info); err != nil || string(info.ConditionID) != marketTestCondition || info.MinTickSize != "0.01" || !info.NegRisk {
		t.Fatalf("case variants changed metadata: %+v %v", info, err)
	}
}

func TestSafetyClobMarketInfoDoesNotPoisonCache(t *testing.T) {
	body := ""
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer s.Close()
	c, err := NewClient(s.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, response := range []string{
		`{"c":"` + marketTestOldCondition + `","mts":"0.01","nr":false,"t":[{"t":"1","o":"yes"}]}`,
		`{"mts":"0.01","nr":false,"t":[{"t":"1","o":"yes"}]}`,
		`{"c":null,"mts":"0.01","nr":false,"t":[{"t":"1","o":"yes"}]}`,
		`{"c":"` + marketTestCondition + `","nr":false,"t":[{"t":"1","o":"yes"}]}`,
		`{"c":"` + marketTestCondition + `","mts":null,"nr":false,"t":[{"t":"1","o":"yes"}]}`,
	} {
		body = response
		if _, err := c.ClobMarketInfo(context.Background(), marketTestCondition); err == nil {
			t.Fatalf("accepted %s", body)
		}
		if len(c.tickSizes) != 0 || len(c.negRisks) != 0 || len(c.feeInfos) != 0 || len(c.tokenConditions) != 0 {
			t.Fatalf("cache poisoned by %s", body)
		}
	}
	body = `{"c":"` + marketTestCondition + `","mts":"0.01","nr":true,"t":[{"t":"1","o":"yes"}]}`
	if _, err := c.ClobMarketInfo(context.Background(), marketTestCondition); err != nil {
		t.Fatal(err)
	}
	if c.tickSizes["1"] != "0.01" || !c.negRisks["1"] || c.tokenConditions["1"] != B256(marketTestCondition) {
		t.Fatal("valid metadata was not cached")
	}
	body = `{"c":"` + marketTestOldCondition + `","mts":"0.1","nr":false,"t":[{"t":"1","o":"yes"}]}`
	if _, err := c.ClobMarketInfo(context.Background(), marketTestCondition); err == nil {
		t.Fatal("accepted mismatched condition")
	}
	if c.tickSizes["1"] != "0.01" || !c.negRisks["1"] || c.tokenConditions["1"] != B256(marketTestCondition) {
		t.Fatal("mismatched response overwrote valid cache")
	}
}

func TestSafetySettersPreventInflightMetadataCaching(t *testing.T) {
	setters := []struct {
		name  string
		set   func(*Client)
		check func(*Client) bool
	}{
		{"tick", func(c *Client) { c.SetTickSize("1", TickSize("0.1")) }, func(c *Client) bool { return c.tickSizes["1"] == "0.1" }},
		{"risk", func(c *Client) { c.SetNegRisk("1", true) }, func(c *Client) bool { return c.negRisks["1"] }},
		{"fee", func(c *Client) { c.SetFeeRateBPS("1", 42) }, func(c *Client) bool { return c.feeRates["1"].BaseFee == 42 }},
	}
	for _, method := range []string{"market-info", "fee-info"} {
		for _, setter := range setters {
			t.Run(method+"/"+setter.name, func(t *testing.T) {
				started, release := make(chan struct{}), make(chan struct{})
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/markets-by-token/1":
						fmt.Fprint(w, `{"condition_id":"`+marketTestCondition+`"}`)
					case "/clob-markets/" + marketTestCondition:
						close(started)
						<-release
						fmt.Fprint(w, `{"c":"`+marketTestCondition+`","mts":"0.01","nr":false,"t":[{"t":"1","o":"yes"}],"fd":{"r":"0.02","e":1}}`)
					default:
						http.Error(w, "unexpected route", http.StatusNotFound)
					}
				}))
				defer s.Close()
				c, err := NewClient(s.URL, DefaultConfig())
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					if method == "market-info" {
						_, err := c.ClobMarketInfo(ctx, marketTestCondition)
						done <- err
					} else {
						_, err := c.FeeInfo(ctx, "1")
						done <- err
					}
				}()
				select {
				case <-started:
				case <-ctx.Done():
					close(release)
					t.Fatal("metadata request did not start")
				}
				setter.set(c)
				close(release)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if !setter.check(c) {
					t.Fatalf("%s setter was overwritten", setter.name)
				}
				if len(c.feeInfos) != 0 || len(c.tokenConditions) != 0 {
					t.Fatal("stale metadata was cached")
				}
				if setter.name != "tick" && len(c.tickSizes) != 0 {
					t.Fatal("stale tick cached")
				}
				if setter.name != "risk" && len(c.negRisks) != 0 {
					t.Fatal("stale risk cached")
				}
			})
		}
	}
}

func TestSafetyCalculateMarketPriceRejectsWrongBook(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"asset_id":"2","neg_risk":false,"asks":[{"price":"0.9","size":"10"}]}`)
	}))
	defer s.Close()
	c, err := NewClient(s.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	amount, err := NewSharesAmount("1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CalculateMarketPrice(context.Background(), "1", Buy, amount, FOK); err == nil {
		t.Fatal("accepted order book for another asset")
	}
}

func TestSafetyRiskAndFeeCache(t *testing.T) {
	counts := map[string]int{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		switch r.URL.Path {
		case "/clob-markets/" + marketTestCondition:
			fmt.Fprint(w, `{"c":"`+marketTestCondition+`","mts":"0.01","t":[{"t":"1","o":"yes"}]}`)
		case "/neg-risk":
			if counts[r.URL.Path] == 1 {
				fmt.Fprint(w, `{}`)
			} else {
				fmt.Fprint(w, `{"neg_risk":true}`)
			}
		case "/fee-rate":
			if counts[r.URL.Path] == 1 {
				fmt.Fprint(w, `{"base_fee":null}`)
			} else {
				fmt.Fprint(w, `{"base_fee":0}`)
			}
		default:
			http.Error(w, "unexpected", 404)
		}
	}))
	defer s.Close()
	c, err := NewClient(s.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.ClobMarketInfo(ctx, marketTestCondition); err != nil {
		t.Fatal(err)
	}
	if _, err := c.NegRisk(ctx, "1"); err == nil {
		t.Fatal("missing risk accepted")
	}
	risk, err := c.NegRisk(ctx, "1")
	if err != nil || !risk.NegRisk || counts["/neg-risk"] != 2 {
		t.Fatalf("risk: %+v %v counts %v", risk, err, counts)
	}
	if _, err := c.FeeRateBPS(ctx, "1"); err == nil {
		t.Fatal("null fee accepted")
	}
	fee, err := c.FeeRateBPS(ctx, "1")
	if err != nil || fee.BaseFee != 0 || counts["/fee-rate"] != 2 {
		t.Fatalf("fee: %+v %v counts %v", fee, err, counts)
	}
}

func TestSafetyFeeFreeMarketBuyAndAbsentToken(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tick-size":
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/markets-by-token/1", "/markets-by-token/2":
			fmt.Fprint(w, `{"condition_id":"`+marketTestCondition+`"}`)
		case "/clob-markets/" + marketTestCondition:
			fmt.Fprint(w, `{"c":"`+marketTestCondition+`","mts":"0.01","t":[{"t":"1","o":"yes"}],"nr":false}`)
		default:
			http.Error(w, "unexpected", 404)
		}
	}))
	defer s.Close()
	c, err := NewClient(s.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.FeeInfo(ctx, "2"); err == nil {
		t.Fatal("absent token treated as fee-free")
	}
	balance := "100"
	p := OrderParams{Version: 2, AssetID: "1", Price: "0.5", Side: Buy, UserUSDCBalance: &balance}
	order, err := c.BuildMarketOrder(ctx, p, MarketAmount{Value: "100", InUSDC: true})
	if err != nil {
		t.Fatal(err)
	}
	if order.Order.MakerAmount.String() != "100000000" || order.Order.TakerAmount.String() != "200000000" {
		t.Fatalf("unexpected fee-free order: %+v", order.Order)
	}
	fee, err := c.FeeInfo(ctx, "1")
	if err != nil || fee.Rate.String() != "0" || fee.Exponent != 0 {
		t.Fatalf("fee: %+v %v", fee, err)
	}
}
