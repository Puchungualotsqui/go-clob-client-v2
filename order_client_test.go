package clob

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientOrderMetadataAndSigning(t *testing.T) {
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		switch r.URL.Path {
		case "/version":
			fmt.Fprint(w, `{"version":1}`)
		case "/tick-size":
			if r.URL.Query().Get("token_id") != "123" {
				t.Error("wrong token ID")
			}
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/fee-rate":
			fmt.Fprint(w, `{"base_fee":42}`)
		case "/neg-risk":
			fmt.Fprint(w, `{"neg_risk":true}`)
		default:
			http.Error(w, "unexpected route", 404)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	p := OrderParams{AssetID: "123", Signer: signer.Address(), Price: "0.50"}
	o, err := c.BuildLimitOrder(context.Background(), p, "10")
	if err != nil {
		t.Fatal(err)
	}
	if o.Order.Version != 1 || o.Order.FeeRateBPS.Uint64() != 42 {
		t.Fatalf("unexpected order: %+v", o.Order)
	}
	signed, err := c.SignOrder(context.Background(), signer, o, Polygon, "owner")
	if err != nil {
		t.Fatal(err)
	}
	want, err := SignOrder(signer, o, Polygon, true, "owner")
	if err != nil || signed.Signature != want.Signature {
		t.Fatalf("wrong risk domain: %v", err)
	}
	if _, err = c.BuildLimitOrder(context.Background(), p, "10"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SignOrder(context.Background(), signer, o, Polygon, "owner"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/version", "/tick-size", "/fee-rate", "/neg-risk"} {
		if counts[path] != 1 {
			t.Errorf("%s called %d times", path, counts[path])
		}
	}
	p.FeeRateBPS = 41
	if _, err = c.BuildLimitOrder(context.Background(), p, "10"); err == nil {
		t.Error("accepted mismatched V1 fee")
	}
	p.FeeRateBPS = 0
	p.TickSize = "0.1"
	if _, err = c.BuildLimitOrder(context.Background(), p, "10"); err == nil {
		t.Error("accepted mismatched tick")
	}
}

func TestClientMarketAndPositionOrder(t *testing.T) {
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		switch r.URL.Path {
		case "/version":
			fmt.Fprint(w, `{"version":2}`)
		case "/tick-size":
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/book":
			fmt.Fprint(w, `{"asset_id":"1","timestamp":"1000","bids":[{"price":"0.4","size":"10"}],"asks":[{"price":"0.7","size":"10"},{"price":"0.6","size":"1"}],"tick_size":"0.01","neg_risk":false}`)
		default:
			http.Error(w, "unexpected route", 404)
		}
	}))
	defer server.Close()
	code := B256("0x" + strings.Repeat("01", 32))
	c, err := NewClient(server.URL, Config{BuilderCode: &code})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	p := OrderParams{AssetID: "1", Signer: signer.Address()}
	o, err := c.BuildMarketOrder(context.Background(), p, MarketAmount{Value: "2"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Order.Version != 2 || o.Order.MakerAmount.String() != "1400000" || o.Order.Builder[0] != 1 {
		t.Fatalf("unexpected market order: %+v", o.Order)
	}
	p.Price = "0.5"
	if _, err := c.BuildMarketOrder(context.Background(), p, MarketAmount{Value: "2"}); err != nil {
		t.Fatal(err)
	}
	if counts["/book"] != 1 {
		t.Fatal("explicit price fetched book")
	}
	p.Version = 3
	position, err := c.BuildLimitOrder(context.Background(), p, "10")
	if err != nil {
		t.Fatal(err)
	}
	if position.Order.Version != 3 {
		t.Fatal("expected V3")
	}
	if _, err := c.SignOrder(context.Background(), signer, position, Polygon, "owner"); err != nil {
		t.Fatal(err)
	}
	if counts["/version"] != 1 || counts["/neg-risk"] != 0 {
		t.Fatalf("unexpected lookups: %v", counts)
	}
}

func TestMarketOrderBookAssetIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, requested, assetField string
		wantErr                     bool
	}{
		{"matching", "1", `"asset_id":"1",`, false},
		{"normalized", "001", `"asset_id":"0001",`, false},
		{"zero asset", "0", `"asset_id":"0",`, false},
		{"mismatch", "1", `"asset_id":"2",`, true},
		{"missing", "1", ``, true},
		{"missing for zero", "0", ``, true},
		{"null", "1", `"asset_id":null,`, true},
		{"invalid", "1", `"asset_id":"invalid",`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bookCalls, otherCalls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/tick-size":
					fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
				case "/book":
					bookCalls++
					fmt.Fprintf(w, `{%s"timestamp":"1","tick_size":"0.01","neg_risk":false,"asks":[{"price":"0.5","size":"10"}],"bids":[{"price":"0.5","size":"10"}]}`, tc.assetField)
				default:
					otherCalls++
					http.Error(w, "unexpected request", http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			c, err := NewClient(server.URL, DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			for _, side := range []Side{Buy, Sell} {
				p := OrderParams{Version: 2, AssetID: tc.requested, Side: side, Signer: Address{1}}
				_, err := c.BuildMarketOrder(context.Background(), p, MarketAmount{Value: "2"})
				if (err != nil) != tc.wantErr {
					t.Fatalf("side %v: error %v, want error %v", side, err, tc.wantErr)
				}
				if err != nil && !strings.Contains(err.Error(), "asset") && !strings.Contains(err.Error(), "uint256") {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if bookCalls != 2 || otherCalls != 0 {
				t.Fatalf("unexpected calls: book=%d other=%d", bookCalls, otherCalls)
			}
		})
	}
}

func TestBalanceAwarePositionMarketBuy(t *testing.T) {
	counts := map[string]int{}
	code := B256("0x" + strings.Repeat("12", 32))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		switch r.URL.Path {
		case "/tick-size":
			if r.URL.Query().Get("token_id") != "987" {
				t.Errorf("wrong asset: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/book":
			if r.URL.Query().Get("token_id") != "987" {
				t.Errorf("wrong book asset: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"asset_id":"987","timestamp":"1","tick_size":"0.01","neg_risk":false,"asks":[{"price":"0.5","size":"1000"}]}`)
		case "/markets-by-token/987":
			fmt.Fprint(w, `{"condition_id":"`+marketTestCondition+`"}`)
		case "/clob-markets/" + marketTestCondition:
			fmt.Fprint(w, `{"c":"`+marketTestCondition+`","mts":"0.01","t":[{"t":"987","o":"yes"}],"fd":{"r":"0.02","e":1}}`)
		case "/fees/builder-fees/" + string(code):
			if r.Header.Get("POLY_SIGNATURE") == "" {
				t.Error("missing authentication")
			}
			if r.Header.Get("POLY_API_KEY") == "key2" {
				fmt.Fprint(w, `{"builderMakerFeeRateBps":0,"builderTakerFeeRateBps":100}`)
			} else {
				fmt.Fprint(w, `{"builderMakerFeeRateBps":1,"builderTakerFeeRateBps":50}`)
			}
		default:
			http.Error(w, "unexpected route", 404)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, Config{BuilderCode: &code})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
	a, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	balance := "100"
	p := OrderParams{Version: 3, AssetID: "987", UserUSDCBalance: &balance}
	amount := MarketAmount{Value: "100", InUSDC: true}
	fee, err := a.BuilderFeeRate(context.Background(), code)
	if err != nil || fee.BuilderTakerFeeRateBPS != 50 {
		t.Fatalf("prefetch builder fee: %+v %v", fee, err)
	}
	for i := 0; i < 2; i++ {
		o, err := a.BuildMarketOrder(context.Background(), p, amount)
		if err != nil {
			t.Fatal(err)
		}
		// 100 / (1 + 0.02*(0.5*0.5)/0.5 + 0.005), then lot-price rounding.
		if o.Order.Version != 3 || o.Order.Maker != signer.Address() || o.Order.Signer != signer.Address() || o.Order.SignatureType != SignatureEOA || o.Order.MakerAmount.String() != "98522150" || o.Order.TakerAmount.String() != "197044300" {
			t.Fatalf("unexpected order: %+v", o.Order)
		}
	}
	for _, path := range []string{"/tick-size", "/markets-by-token/987", "/clob-markets/" + marketTestCondition, "/fees/builder-fees/" + string(code)} {
		if counts[path] != 1 {
			t.Errorf("%s called %d times", path, counts[path])
		}
	}
	if counts["/book"] != 2 || counts["/version"] != 0 || counts["/fee-rate"] != 0 {
		t.Fatalf("unexpected requests: %v", counts)
	}
	fee, err = a.BuilderFeeRate(context.Background(), code)
	if err != nil || fee.BuilderMakerFeeRateBPS != 1 || counts["/fees/builder-fees/"+string(code)] != 1 {
		t.Fatalf("order and direct lookup did not share cache: %+v %v", fee, err)
	}
	creds.Key = "key2"
	other, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	second, err := other.BuildMarketOrder(context.Background(), p, amount)
	if err != nil || second.Order.MakerAmount.String() != "98039200" {
		t.Fatalf("cross-account fee cache: %+v %v", second.Order, err)
	}
	c.InvalidateInternalCaches()
	if _, err := a.BuildMarketOrder(context.Background(), p, amount); err != nil {
		t.Fatal(err)
	}
	if counts["/clob-markets/"+marketTestCondition] != 2 || counts["/fees/builder-fees/"+string(code)] != 3 {
		t.Fatalf("fee cache was not invalidated: %v", counts)
	}
	if _, err := a.BuilderFeeRate(context.Background(), code); err != nil || counts["/fees/builder-fees/"+string(code)] != 3 {
		t.Fatalf("order lookup did not warm direct cache: %v, counts %v", err, counts)
	}
}

func TestBalanceAwareOrderFailsClosed(t *testing.T) {
	var marketBody = `{"c":"` + marketTestCondition + `","mts":"0.01","t":[{"t":"99","o":"yes"}],"fd":{"r":"0.02","e":1}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tick-size":
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/markets-by-token/1":
			fmt.Fprint(w, `{"condition_id":"`+marketTestCondition+`"}`)
		case "/clob-markets/" + marketTestCondition:
			fmt.Fprint(w, marketBody)
		case "/fees/builder-fees/" + "0x" + strings.Repeat("ab", 32):
			fmt.Fprint(w, `{}`)
		default:
			http.Error(w, "unexpected route", 404)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	balance := "100"
	p := OrderParams{Version: 3, AssetID: "1", Price: "0.5", UserUSDCBalance: &balance}
	amount := MarketAmount{Value: "100", InUSDC: true}
	for i := 0; i < 2; i++ {
		if _, err := c.BuildMarketOrder(context.Background(), p, amount); err == nil || !strings.Contains(err.Error(), "does not contain asset") {
			t.Fatalf("missing asset must fail: %v", err)
		}
	}
	marketBody = `{"c":"` + marketTestOldCondition + `","mts":"0.01","t":[{"t":"1","o":"yes"}]}`
	if _, err := c.BuildMarketOrder(context.Background(), p, amount); err == nil || !strings.Contains(err.Error(), "condition does not match") {
		t.Fatalf("wrong condition: %v", err)
	}
	marketBody = `{"c":"` + marketTestCondition + `","mts":"0.01","t":[{"t":"1","o":"yes"}],"fd":{"r":"0.02","e":1}}`
	o, err := c.BuildMarketOrder(context.Background(), p, amount)
	if err != nil || o.Order.MakerAmount.String() != "99009900" {
		t.Fatalf("platform fee sizing: %+v %v", o.Order, err)
	}
	p.Builder = Hash{}
	for i := range p.Builder {
		p.Builder[i] = 0xab
	}
	if _, err := c.BuildMarketOrder(context.Background(), p, amount); err == nil || !strings.Contains(err.Error(), "authenticated client required") {
		t.Fatalf("missing auth: %v", err)
	}
	signer, _ := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
	a, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := a.BuildMarketOrder(context.Background(), p, amount); err == nil || !strings.Contains(err.Error(), "missing taker fee") {
			t.Fatalf("missing fee accepted: %v", err)
		}
	}
	p.Side = Sell
	if _, err := c.BuildMarketOrder(context.Background(), p, amount); err == nil {
		t.Fatal("balance on sell accepted")
	}
	p.Side = Buy
	p.MarketBuyFees = &MarketBuyFees{}
	if _, err := c.BuildMarketOrder(context.Background(), p, amount); err == nil {
		t.Fatal("conflicting fees accepted")
	}
	p.TickSize = "0.01"
	if _, err := BuildMarketOrder(p, amount, nil); err == nil {
		t.Fatal("offline balance silently ignored")
	}
}

func TestAuthenticatedMarketOrderBinding(t *testing.T) {
	ctx := context.Background()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/tick-size" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	signer := &vectorSigner{address: Address{1}}
	creds := Credentials{Key: "key"}
	deposit := Address{2}
	params := OrderParams{Version: 2, AssetID: "1", Price: "0.5"}
	amount := MarketAmount{Value: "2"}
	for _, typ := range []SignatureType{SignatureEOA, SignatureProxy, SignatureSafe, SignaturePoly1271} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			opts := AuthenticationOptions{Credentials: &creds, SignatureType: typ}
			if typ == SignaturePoly1271 {
				opts.Funder = &deposit
			}
			a, err := c.Authenticate(ctx, signer, Polygon, opts)
			if err != nil {
				t.Fatal(err)
			}
			maker := signer.Address()
			if typ == SignatureProxy || typ == SignatureSafe {
				maker, err = DeriveWallet(signer.Address(), Polygon, typ)
				if err != nil {
					t.Fatal(err)
				}
			} else if typ == SignaturePoly1271 {
				maker = deposit
			}
			orderSigner := signer.Address()
			if typ == SignaturePoly1271 {
				orderSigner = maker
			}
			o, err := a.BuildMarketOrder(ctx, params, amount)
			if err != nil {
				t.Fatal(err)
			}
			if o.Order.Maker != maker || o.Order.Signer != orderSigner || o.Order.SignatureType != typ {
				t.Fatalf("incorrect identity: %+v", o.Order)
			}
			before := requests
			for _, change := range []func(*OrderParams){
				func(p *OrderParams) { p.Signer = Address{99} },
				func(p *OrderParams) { p.ChainID = Amoy },
				func(p *OrderParams) { p.SignatureType = SignatureType(255) },
				func(p *OrderParams) { f := Address{99}; p.Funder = &f },
			} {
				bad := params
				change(&bad)
				if _, err := a.BuildMarketOrder(ctx, bad, amount); err == nil {
					t.Error("accepted mismatched identity")
				}
			}
			if requests != before {
				t.Error("mismatched identity triggered network request")
			}
		})
	}
	direct, err := NewAuthenticatedClient(server.URL, signer.Address(), creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := direct.BuildMarketOrder(ctx, params, amount); err == nil || !strings.Contains(err.Error(), "Client.Authenticate") {
		t.Fatalf("direct client must reject unbound order: %v", err)
	}
	var nilClient *AuthenticatedClient
	if _, err := nilClient.BuildMarketOrder(ctx, params, amount); err == nil {
		t.Fatal("nil client accepted")
	}
}

func TestClientOrderMetadataFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"version":9}`)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	p := OrderParams{AssetID: "1", Price: "0.5"}
	if _, err := c.BuildLimitOrder(context.Background(), p, "1"); err == nil || !strings.Contains(err.Error(), "unsupported token order version") {
		t.Fatalf("unexpected error: %v", err)
	}
	p.Version = 3
	if _, err := c.BuildLimitOrder(context.Background(), p, "1"); err == nil || !strings.Contains(err.Error(), "resolve tick size") {
		t.Fatalf("unexpected error: %v", err)
	}
	p.AssetID = "-1"
	if _, err := c.BuildLimitOrder(context.Background(), p, "1"); err == nil {
		t.Fatal("invalid ID accepted")
	}
}
