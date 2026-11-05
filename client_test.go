package clob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const marketTestCondition = "0x1111111111111111111111111111111111111111111111111111111111111111"
const marketTestOldCondition = "0x2222222222222222222222222222222222222222222222222222222222222222"
const marketTestNewCondition = "0x3333333333333333333333333333333333333333333333333333333333333333"

func testClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(server.URL+"/prefix", DefaultConfig(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPublicMarketRequests(t *testing.T) {
	tests := []struct {
		name, method, path, query, body, response string
		run                                       func(*Client) error
	}{
		{"ok", "GET", "/prefix/", "", "", `"OK"`, func(c *Client) error {
			v, e := c.OK(context.Background())
			if e == nil && v != "OK" {
				return fmt.Errorf("got %q", v)
			}
			return e
		}},
		{"time", "GET", "/prefix/time", "", "", `123`, func(c *Client) error {
			v, e := c.ServerTime(context.Background())
			if e == nil && v != 123 {
				return fmt.Errorf("got %d", v)
			}
			return e
		}},
		{"midpoint", "GET", "/prefix/midpoint", "token_id=123", "", `{"mid":"0.5"}`, func(c *Client) error {
			v, e := c.Midpoint(context.Background(), MidpointRequest{TokenID: "123"})
			if e == nil && v.Mid != "0.5" {
				return fmt.Errorf("got %v", v)
			}
			return e
		}},
		{"midpoints", "POST", "/prefix/midpoints", "", `[{"token_id":"123"}]`, `{"123":"0.5"}`, func(c *Client) error {
			_, e := c.Midpoints(context.Background(), []MidpointRequest{{TokenID: "123"}})
			return e
		}},
		{"price", "GET", "/prefix/price", "side=SELL&token_id=123", "", `{"price":"0.5"}`, func(c *Client) error {
			_, e := c.Price(context.Background(), PriceRequest{TokenID: "123", Side: Sell})
			return e
		}},
		{"prices", "POST", "/prefix/prices", "", `[{"token_id":"123","side":"BUY"}]`, `{"123":{"BUY":"0.5"}}`, func(c *Client) error {
			_, e := c.Prices(context.Background(), []PriceRequest{{TokenID: "123", Side: Buy}})
			return e
		}},
		{"history", "GET", "/prefix/prices-history", "endTs=20&fidelity=5&market=123&startTs=10", "", `{"history":[{"t":11,"p":"0.5"}]}`, func(c *Client) error {
			f := uint32(5)
			_, e := c.PriceHistory(context.Background(), PriceHistoryRequest{Market: "123", TimeRange: TimestampRange(10, 20), Fidelity: &f})
			return e
		}},
		{"spread", "GET", "/prefix/spread", "token_id=123", "", `{"spread":"0.1"}`, func(c *Client) error { _, e := c.Spread(context.Background(), SpreadRequest{TokenID: "123"}); return e }},
		{"spreads", "POST", "/prefix/spreads", "", `[{"token_id":"123"}]`, `{"spreads":{"123":"0.1"}}`, func(c *Client) error {
			_, e := c.Spreads(context.Background(), []SpreadRequest{{TokenID: "123"}})
			return e
		}},
		{"book", "GET", "/prefix/book", "token_id=123", "", `{"neg_risk":false}`, func(c *Client) error {
			_, e := c.OrderBook(context.Background(), OrderBookSummaryRequest{TokenID: "123"})
			return e
		}},
		{"books", "POST", "/prefix/books", "", `[{"token_id":"123"}]`, `[]`, func(c *Client) error {
			_, e := c.OrderBooks(context.Background(), []OrderBookSummaryRequest{{TokenID: "123"}})
			return e
		}},
		{"last price", "GET", "/prefix/last-trade-price", "token_id=123", "", `{"price":"0.5","side":"BUY"}`, func(c *Client) error {
			_, e := c.LastTradePrice(context.Background(), LastTradePriceRequest{TokenID: "123"})
			return e
		}},
		{"last prices", "GET", "/prefix/last-trades-prices", "", `[{"token_id":"123"}]`, `[{"token_id":"123","price":"0.5","side":"SELL"}]`, func(c *Client) error {
			_, e := c.LastTradesPrices(context.Background(), []LastTradePriceRequest{{TokenID: "123"}})
			return e
		}},
		{"market", "GET", "/prefix/markets/condition", "", "", `{` + fixtureMarketFields + `}`, func(c *Client) error { _, e := c.Market(context.Background(), "condition"); return e }},
		{"markets", "GET", "/prefix/markets", "next_cursor=a%2Bb%2F%3D", "", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, func(c *Client) error { cur := "a+b/="; _, e := c.Markets(context.Background(), &cur); return e }},
		{"sampling", "GET", "/prefix/sampling-markets", "", "", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, func(c *Client) error { _, e := c.SamplingMarkets(context.Background(), nil); return e }},
		{"simplified", "GET", "/prefix/simplified-markets", "", "", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, func(c *Client) error { _, e := c.SimplifiedMarkets(context.Background(), nil); return e }},
		{"sampling simplified", "GET", "/prefix/sampling-simplified-markets", "", "", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, func(c *Client) error { _, e := c.SamplingSimplifiedMarkets(context.Background(), nil); return e }},
		{"market by token", "GET", "/prefix/markets-by-token/123", "", "", `{}`, func(c *Client) error { _, e := c.MarketByToken(context.Background(), "123"); return e }},
		{"events", "GET", "/prefix/markets/live-activity/condition", "", "", `[{"event":1}]`, func(c *Client) error { _, e := c.MarketTradesEvents(context.Background(), "condition"); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path || r.URL.RawQuery != tt.query || string(b) != tt.body {
					t.Errorf("request: %s %s?%s body=%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery, b)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing JSON content type")
				}
				fmt.Fprint(w, tt.response)
			}))
			defer server.Close()
			if err := tt.run(testClient(t, server)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientErrorsAndContext(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		switch r.URL.Path {
		case "/prefix/price":
			w.WriteHeader(429)
			fmt.Fprint(w, "slow down")
		case "/prefix/midpoint":
			fmt.Fprint(w, "null")
		default:
			fmt.Fprint(w, "bad json")
		}
	}))
	defer server.Close()
	c := testClient(t, server)
	_, err := c.Price(context.Background(), PriceRequest{TokenID: "1"})
	var se StatusError
	if !errors.As(err, &se) || se.StatusCode != 429 || se.Method != "GET" || se.Path != "/prefix/price" || se.Message != "slow down" {
		t.Fatalf("status error: %v", err)
	}
	_, err = c.Midpoint(context.Background(), MidpointRequest{TokenID: "1"})
	if !errors.As(err, &se) || se.StatusCode != 404 {
		t.Fatalf("null response: %v", err)
	}
	if _, err = c.OK(context.Background()); err == nil {
		t.Fatal("accepted invalid JSON")
	}
	before := count.Load()
	_, err = c.PriceHistory(context.Background(), PriceHistoryRequest{Market: "1"})
	if err == nil || count.Load() != before {
		t.Fatal("invalid query sent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ServerTime(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context: %v", err)
	}
	if _, err = NewClient("https://bad.example/?q=1", DefaultConfig()); err == nil {
		t.Fatal("accepted invalid host")
	}
}

func TestCacheAndMarketInfo(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/prefix/tick-size":
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/prefix/neg-risk":
			fmt.Fprint(w, `{"neg_risk":true}`)
		case "/prefix/fee-rate":
			fmt.Fprint(w, `{"base_fee":42}`)
		case "/prefix/version":
			fmt.Fprint(w, `{"version":2}`)
		case "/prefix/markets-by-token/1":
			fmt.Fprint(w, `{"condition_id":"`+marketTestCondition+`","primary_token_id":"1","secondary_token_id":"2"}`)
		case "/prefix/clob-markets/" + marketTestCondition:
			fmt.Fprint(w, `{"c":"`+marketTestCondition+`","t":[{"t":"1","o":"yes"},{"t":"2","o":"no"},null],"mts":"0.005","nr":true,"fd":{"r":"0.02","e":2}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := testClient(t, server)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := c.TickSize(ctx, "3"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.NegRisk(ctx, "3"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.FeeRateBPS(ctx, "3"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Version(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if v, err := c.FeeInfo(ctx, "1"); err != nil || v.Exponent != 2 || v.Rate != "0.02" {
		t.Fatalf("fee info: %+v %v", v, err)
	}
	if v, err := c.TickSize(ctx, "2"); err != nil || v.MinimumTickSize != HalfCent {
		t.Fatalf("tick: %+v %v", v, err)
	}
	if v, err := c.NegRisk(ctx, "2"); err != nil || !v.NegRisk {
		t.Fatalf("neg risk: %+v %v", v, err)
	}
	if v, err := c.FeeRateBPS(ctx, "2"); err != nil || v.BaseFee != 42 {
		t.Fatalf("V1 fee must be fetched: %+v %v", v, err)
	}
	c.SetTickSize("0004", Tenth)
	if v, err := c.TickSize(ctx, "4"); err != nil || v.MinimumTickSize != Tenth {
		t.Fatalf("seeded tick: %+v %v", v, err)
	}
	c.SetNegRisk("4", false)
	if v, err := c.NegRisk(ctx, "4"); err != nil || v.NegRisk {
		t.Fatalf("seeded risk: %+v %v", v, err)
	}
	c.SetFeeRateBPS("4", 10)
	bad := uint32(11)
	if _, err := c.ResolveFeeRateBPS(ctx, "4", &bad); err == nil {
		t.Fatal("fee mismatch accepted")
	}
	c.InvalidateInternalCaches()
	if _, err := c.TickSize(ctx, "3"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Version(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["/prefix/tick-size"] != 2 || counts["/prefix/neg-risk"] != 1 || counts["/prefix/fee-rate"] != 2 || counts["/prefix/version"] != 2 || counts["/prefix/clob-markets/"+marketTestCondition] != 1 {
		t.Fatal(counts)
	}
}

func TestGeoblockAndRequestExtension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/geoblock" {
			fmt.Fprint(w, `{"blocked":true,"ip":"127.0.0.1"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Signature") != "signed" || string(b) != `{"x":1}` || r.URL.Query().Get("k") != "a+b" {
			t.Errorf("bad signed request: %s %v", b, r.Header)
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	geo := server.URL
	config := DefaultConfig()
	config.GeoblockHost = &geo
	c, err := NewClient(server.URL, config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	geo = "bad"
	if result, err := c.CheckGeoblock(context.Background()); err != nil || !result.Blocked {
		t.Fatalf("geoblock: %+v %v", result, err)
	}
	req, err := c.NewRequest(context.Background(), "POST", "custom", url.Values{"k": {"a+b"}}, []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Signature", "signed")
	var result map[string]bool
	if err = c.Do(req, &result); err != nil || !result["ok"] {
		t.Fatalf("request: %v %v", result, err)
	}
}

func TestAuthenticatedRequestDoesNotFollowRedirect(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		fmt.Fprint(w, `"ok"`)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	c := testClient(t, origin)
	for _, header := range []string{"POLY_SIGNATURE", "POLY_API_KEY", "POLY_PASSPHRASE"} {
		t.Run(header, func(t *testing.T) {
			req, err := c.NewRequest(context.Background(), http.MethodGet, "test", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(header, "secret")
			var out string
			err = c.Do(req, &out)
			var status StatusError
			if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect {
				t.Fatalf("redirect error: %v", err)
			}
		})
	}
	if targetCalls.Load() != 0 {
		t.Fatal("authenticated request followed a redirect")
	}
	if got, err := c.OK(context.Background()); err != nil || got != "ok" || targetCalls.Load() != 1 {
		t.Fatalf("unsigned redirect: got %q, error %v, calls %d", got, err, targetCalls.Load())
	}
}

func TestCacheInvalidationDuringRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		fmt.Fprint(w, `{"base_fee":1}`)
	}))
	defer server.Close()
	c := testClient(t, server)
	done := make(chan error, 1)
	go func() { _, err := c.FeeRateBPS(context.Background(), "1"); done <- err }()
	<-started
	c.InvalidateInternalCaches()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := c.FeeRateBPS(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("stale response repopulated cache")
	}
}

func TestFeeInfoInvalidationDuringMarketLookup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var lookups atomic.Int32
	var infos atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/markets-by-token/1":
			if lookups.Add(1) == 1 {
				close(started)
				<-release
				fmt.Fprint(w, `{"condition_id":"`+marketTestOldCondition+`"}`)
				return
			}
			fmt.Fprint(w, `{"condition_id":"`+marketTestNewCondition+`"}`)
		case "/prefix/clob-markets/" + marketTestOldCondition:
			infos.Add(1)
			fmt.Fprint(w, `{"c":"`+marketTestOldCondition+`","t":[{"t":"1","o":"yes"}],"mts":"0.01","fd":{"r":"0.01","e":1}}`)
		case "/prefix/clob-markets/" + marketTestNewCondition:
			infos.Add(1)
			fmt.Fprint(w, `{"c":"`+marketTestNewCondition+`","t":[{"t":"1","o":"yes"}],"mts":"0.005","fd":{"r":"0.02","e":2}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := testClient(t, server)
	type result struct {
		info FeeInfo
		err  error
	}
	done := make(chan result, 1)
	go func() {
		info, err := c.FeeInfo(context.Background(), "1")
		done <- result{info, err}
	}()
	<-started
	c.InvalidateInternalCaches()
	close(release)
	first := <-done
	if first.err != nil || first.info.Rate != "0.01" {
		t.Fatalf("in-flight result: %+v %v", first.info, first.err)
	}
	info, err := c.FeeInfo(context.Background(), "1")
	if err != nil || info.Rate != "0.02" || info.Exponent != 2 {
		t.Fatalf("fresh result: %+v %v", info, err)
	}
	if lookups.Load() != 2 || infos.Load() != 2 {
		t.Fatalf("lookups=%d info requests=%d", lookups.Load(), infos.Load())
	}
	if tick, err := c.TickSize(context.Background(), "1"); err != nil || tick.MinimumTickSize != HalfCent {
		t.Fatalf("fresh tick size: %+v %v", tick, err)
	}
}

func TestCalculateMarketPriceAndHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"market":"0x1111111111111111111111111111111111111111111111111111111111111111","asset_id":"1","timestamp":"1000","bids":[{"price":"0.4","size":"10"}],"asks":[{"price":"0.7","size":"10"},{"price":"0.6","size":"1"}],"min_order_size":"1","neg_risk":false,"tick_size":"0.01"}`)
	}))
	defer server.Close()
	c := testClient(t, server)
	amount, err := NewSharesAmount("2")
	if err != nil {
		t.Fatal(err)
	}
	price, err := c.CalculateMarketPrice(context.Background(), "1", Buy, amount, FOK)
	if err != nil || price != "0.7" {
		t.Fatalf("price %s %v", price, err)
	}
	book, err := c.OrderBook(context.Background(), OrderBookSummaryRequest{TokenID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.OrderBookHash(book)
	if err != nil || len(h) != 64 {
		t.Fatalf("hash: %s %v", h, err)
	}
	if _, err = json.Marshal(book); err != nil {
		t.Fatal(err)
	}
}

func TestRustOrderBookHashVector(t *testing.T) {
	const fixture = `{"market":"0x00000000000000000000000000000000000000000000000000000000aabbcc00","asset_id":"15871154585880608648532107628464183779895785213830018178010423617714102767076","timestamp":"123456789","bids":[{"price":"0.3","size":"100"},{"price":"0.4","size":"100"}],"asks":[{"price":"0.6","size":"100"},{"price":"0.7","size":"100"}],"min_order_size":"100","neg_risk":false,"tick_size":"0.01"}`
	var book OrderBookSummaryResponse
	if err := json.Unmarshal([]byte(fixture), &book); err != nil {
		t.Fatal(err)
	}
	hash, err := (&Client{}).OrderBookHash(book)
	if err != nil || hash != "03196cc4f520d81c0748b4f042f2096441d160e8ef5eac4f0378cb5bd80fd183" {
		t.Fatalf("hash %s: %v", hash, err)
	}
}

func TestStreamData(t *testing.T) {
	var seen []int
	calls := 0
	err := StreamData(context.Background(), func(ctx context.Context, cursor *string) (Page[int], error) {
		calls++
		if cursor == nil {
			return Page[int]{Data: []int{1, 2}, NextCursor: "next"}, nil
		}
		if *cursor != "next" {
			t.Fatalf("cursor %q", *cursor)
		}
		return Page[int]{Data: []int{3}, NextCursor: TerminalCursor}, nil
	}, func(n int) error { seen = append(seen, n); return nil })
	if err != nil || calls != 2 || fmt.Sprint(seen) != "[1 2 3]" {
		t.Fatalf("%v %d %v", seen, calls, err)
	}
}

func TestStreamDataRejectsInvalidCursors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cursors []string
	}{
		{"empty", []string{""}},
		{"same", []string{"a", "a"}},
		{"cycle", []string{"a", "b", "a"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls, yielded := 0, 0
			err := StreamData(context.Background(), func(context.Context, *string) (Page[int], error) {
				if calls >= len(tt.cursors) {
					return Page[int]{}, fmt.Errorf("unexpected extra fetch")
				}
				cursor := tt.cursors[calls]
				calls++
				return Page[int]{Data: []int{calls}, NextCursor: cursor}, nil
			}, func(int) error { yielded++; return nil })
			if err == nil || !strings.Contains(err.Error(), "cursor") || calls != len(tt.cursors) || yielded != len(tt.cursors)-1 {
				t.Fatalf("error=%v calls=%d yielded=%d", err, calls, yielded)
			}
		})
	}
}

func TestVersionRejectsMissingOrNull(t *testing.T) {
	for _, body := range []string{`{}`, `{"version":null}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer s.Close()
			c := testClient(t, s)
			if _, err := c.Version(context.Background()); err == nil {
				t.Fatal("expected missing version error")
			}
		})
	}
}

func TestPublicStatusErrorUsesEscapedPath(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "missing", http.StatusNotFound) }))
	defer s.Close()
	c := testClient(t, s)
	_, err := c.Market(context.Background(), "a/b")
	var status StatusError
	if !errors.As(err, &status) || status.Path != "/prefix/markets/a%2Fb" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEscapedPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.EscapedPath(), "/markets/a%2Fb%3Fc") {
			t.Errorf("path %s", r.URL.EscapedPath())
		}
		fmt.Fprint(w, `{`+fixtureMarketFields+`}`)
	}))
	defer server.Close()
	if _, err := testClient(t, server).Market(context.Background(), "a/b?c"); err != nil {
		t.Fatal(err)
	}
}
