package clob

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testAuthenticatedClient(t *testing.T, host string, opts ...AuthenticatedClientOptions) *AuthenticatedClient {
	t.Helper()
	c, err := NewAuthenticatedClient(host, Address{}, Credentials{Key: "key", Secret: base64.URLEncoding.EncodeToString([]byte("secret")), Passphrase: "pass"}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAuthenticatedRoutes(t *testing.T) {
	ctx := context.Background()
	cursor := "a+/="
	id := "id +/"
	code := B256("0x" + strings.Repeat("1", 64))
	order := SignedOrder{SignableOrder: SignableOrder{Order: Order{Version: 2, TokenID: big.NewInt(1), MakerAmount: big.NewInt(2), TakerAmount: big.NewInt(3), Timestamp: big.NewInt(4), Expiration: big.NewInt(0)}}}
	tests := []struct {
		name, method, path, response, query, body string
		call                                      func(*AuthenticatedClient) error
	}{
		{"keys", "GET", "/auth/api-keys", `{"apiKeys":[]}`, "", "", func(c *AuthenticatedClient) error { _, e := c.APIKeys(ctx); return e }},
		{"delete key", "DELETE", "/auth/api-key", `{}`, "", "", func(c *AuthenticatedClient) error { _, e := c.DeleteAPIKey(ctx); return e }},
		{"ban", "GET", "/auth/ban-status/closed-only", `{"closed_only":true}`, "", "", func(c *AuthenticatedClient) error { _, e := c.ClosedOnlyMode(ctx); return e }},
		{"post order", "POST", "/order", `{` + fixturePostFields + `}`, "", "json", func(c *AuthenticatedClient) error { _, e := c.PostOrder(ctx, order); return e }},
		{"post orders", "POST", "/orders", `[]`, "", "json", func(c *AuthenticatedClient) error { _, e := c.PostOrders(ctx, []SignedOrder{order}); return e }},
		{"order", "GET", "/data/order/id%20+%2F", fixtureOpen, "", "", func(c *AuthenticatedClient) error { _, e := c.Order(ctx, id); return e }},
		{"orders", "GET", "/data/orders", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "id=id+%2B%2F&next_cursor=a%2B%2F%3D", "", func(c *AuthenticatedClient) error {
			r, e := c.Orders(ctx, OrdersRequest{OrderID: &id}, &cursor)
			if e == nil && r.NextCursor != TerminalCursor {
				return fmt.Errorf("bad cursor: %q", r.NextCursor)
			}
			return e
		}},
		{"cancel order", "DELETE", "/order", `{}`, "", `{"orderID":"id +/"}`, func(c *AuthenticatedClient) error { _, e := c.CancelOrder(ctx, id); return e }},
		{"cancel orders", "DELETE", "/orders", `{}`, "", `["a","b"]`, func(c *AuthenticatedClient) error { _, e := c.CancelOrders(ctx, []string{"a", "b"}); return e }},
		{"cancel all", "DELETE", "/cancel-all", `{}`, "", "", func(c *AuthenticatedClient) error { _, e := c.CancelAllOrders(ctx); return e }},
		{"cancel market", "DELETE", "/cancel-market-orders", `{}`, "", `{"market":null,"asset_id":null}`, func(c *AuthenticatedClient) error {
			_, e := c.CancelMarketOrders(ctx, CancelMarketOrderRequest{})
			return e
		}},
		{"trades", "GET", "/data/trades", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "next_cursor=a%2B%2F%3D", "", func(c *AuthenticatedClient) error { _, e := c.Trades(ctx, TradesRequest{}, &cursor); return e }},
		{"notifications", "GET", "/notifications", `[]`, "signature_type=1", "", func(c *AuthenticatedClient) error { _, e := c.Notifications(ctx); return e }},
		{"delete notifications", "DELETE", "/notifications", ``, "ids=a%2Cb", "", func(c *AuthenticatedClient) error {
			return c.DeleteNotifications(ctx, DeleteNotificationsRequest{NotificationIDs: []string{"a", "b"}})
		}},
		{"balance", "GET", "/balance-allowance", `{"balance":"1"}`, "asset_type=COLLATERAL&signature_type=1", "", func(c *AuthenticatedClient) error {
			_, e := c.BalanceAllowance(ctx, BalanceAllowanceRequest{AssetType: Collateral})
			return e
		}},
		{"update balance", "GET", "/balance-allowance/update", ``, "asset_type=COLLATERAL&signature_type=0", "", func(c *AuthenticatedClient) error {
			s := EOA
			return c.UpdateBalanceAllowance(ctx, BalanceAllowanceRequest{AssetType: Collateral, SignatureType: &s})
		}},
		{"scoring", "GET", "/order-scoring", `{"scoring":true}`, "order_id=id+%2B%2F", "", func(c *AuthenticatedClient) error { _, e := c.IsOrderScoring(ctx, id); return e }},
		{"scoring batch", "POST", "/orders-scoring", `{"a":true}`, "", `["a"]`, func(c *AuthenticatedClient) error { _, e := c.AreOrdersScoring(ctx, []string{"a"}); return e }},
		{"earnings", "GET", "/rewards/user", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "date=2026-01-01&next_cursor=a%2B%2F%3D&signature_type=1", "", func(c *AuthenticatedClient) error {
			_, e := c.EarningsForUserForDay(ctx, "2026-01-01", &cursor)
			return e
		}},
		{"earnings total", "GET", "/rewards/user/total", `[]`, "date=2026-01-01&signature_type=1", "", func(c *AuthenticatedClient) error { _, e := c.TotalEarningsForUserForDay(ctx, "2026-01-01"); return e }},
		{"earnings markets", "GET", "/rewards/user/markets", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "date=2026-01-01&next_cursor=a%2B%2F%3D&no_competition=false&order_by=&position=&signature_type=1", "", func(c *AuthenticatedClient) error {
			_, e := c.UserEarningsAndMarketsConfig(ctx, UserRewardsEarningRequest{Date: "2026-01-01"}, &cursor)
			return e
		}},
		{"percentages", "GET", "/rewards/user/percentages", `{}`, "signature_type=1", "", func(c *AuthenticatedClient) error { _, e := c.RewardPercentages(ctx); return e }},
		{"current rewards", "GET", "/rewards/markets/current", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "next_cursor=a%2B%2F%3D", "", func(c *AuthenticatedClient) error { _, e := c.CurrentRewards(ctx, &cursor); return e }},
		{"market rewards", "GET", "/rewards/markets/id%20+%2F", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "", "", func(c *AuthenticatedClient) error { _, e := c.RawRewardsForMarket(ctx, id, nil); return e }},
		{"create readonly", "POST", "/auth/readonly-api-key", `{"apiKey":"ro"}`, "", "", func(c *AuthenticatedClient) error {
			r, e := c.CreateReadonlyAPIKey(ctx)
			if e == nil && r.APIKey != "ro" {
				return fmt.Errorf("bad key")
			}
			return e
		}},
		{"readonly keys", "GET", "/auth/readonly-api-keys", `{"readonlyApiKeys":["ro"]}`, "", "", func(c *AuthenticatedClient) error {
			r, e := c.ReadonlyAPIKeys(ctx)
			if e == nil && !reflect.DeepEqual(r, []string{"ro"}) {
				return fmt.Errorf("bad keys")
			}
			return e
		}},
		{"delete readonly", "DELETE", "/auth/readonly-api-key", ``, "", `{"key":"ro"}`, func(c *AuthenticatedClient) error { return c.DeleteReadonlyAPIKey(ctx, "ro") }},
		{"pre migration", "GET", "/data/pre-migration-orders", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "next_cursor=a%2B%2F%3D", "", func(c *AuthenticatedClient) error { _, e := c.PreMigrationOrders(ctx, &cursor); return e }},
		{"builder fees", "GET", "/fees/builder-fees/" + string(code), `{"builderMakerFeeRateBps":0,"builderTakerFeeRateBps":0}`, "", "", func(c *AuthenticatedClient) error { _, e := c.BuilderFeeRate(ctx, code); return e }},
		{"create builder", "POST", "/auth/builder-api-key", `{"apiKey":"b","secret":"s","passphrase":"p"}`, "", "", func(c *AuthenticatedClient) error {
			r, e := c.CreateBuilderAPIKey(ctx)
			if e == nil && r.Key != "b" {
				return fmt.Errorf("bad key")
			}
			return e
		}},
		{"builder keys", "GET", "/auth/builder-api-key", `[]`, "", "", func(c *AuthenticatedClient) error { _, e := c.BuilderAPIKeys(ctx); return e }},
		{"revoke builder", "DELETE", "/auth/builder-api-key", ``, "", "", func(c *AuthenticatedClient) error { return c.RevokeBuilderAPIKey(ctx) }},
		{"builder trades", "GET", "/builder/trades", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "builder_code=" + string(code) + "&next_cursor=a%2B%2F%3D", "", func(c *AuthenticatedClient) error {
			_, e := c.BuilderTrades(ctx, code, TradesRequest{}, &cursor)
			return e
		}},
		{"heartbeat", "POST", "/v1/heartbeats", `{"heartbeat_id":"00000000-0000-0000-0000-000000000001"}`, "", `{"heartbeat_id":null}`, func(c *AuthenticatedClient) error { _, e := c.PostHeartbeat(ctx, nil); return e }},
		{"rfq create request", "POST", "/rfq/request", `{"requestId":"r","expiry":1}`, "", "json", func(c *AuthenticatedClient) error { _, e := c.CreateRequest(ctx, CreateRFQRequestRequest{}); return e }},
		{"rfq cancel request", "DELETE", "/rfq/request", `OK`, "", `{"requestId":"r"}`, func(c *AuthenticatedClient) error {
			return c.CancelRequest(ctx, CancelRFQRequestRequest{RequestID: "r"})
		}},
		{"rfq requests", "GET", "/rfq/data/requests", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "next_cursor=a%2B%2F%3D&requestIds=a&requestIds=b", "", func(c *AuthenticatedClient) error {
			_, e := c.Requests(ctx, RFQRequestsRequest{RequestIDs: []string{"a", "b"}}, &cursor)
			return e
		}},
		{"rfq create quote", "POST", "/rfq/quote", `{"quoteId":"q"}`, "", "json", func(c *AuthenticatedClient) error { _, e := c.CreateQuote(ctx, CreateRFQQuoteRequest{}); return e }},
		{"rfq cancel quote", "DELETE", "/rfq/quote", `OK`, "", `{"quoteId":"q"}`, func(c *AuthenticatedClient) error { return c.CancelQuote(ctx, CancelRFQQuoteRequest{QuoteID: "q"}) }},
		{"rfq quotes", "GET", "/rfq/data/quotes", `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`, "quoteIds=a&quoteIds=b", "", func(c *AuthenticatedClient) error {
			_, e := c.Quotes(ctx, RFQQuotesRequest{QuoteIDs: []string{"a", "b"}}, nil)
			return e
		}},
		{"rfq accept", "POST", "/rfq/request/accept", `OK`, "", "json", func(c *AuthenticatedClient) error { _, e := c.AcceptQuote(ctx, AcceptRFQQuoteRequest{}); return e }},
		{"rfq approve", "POST", "/rfq/quote/approve", `{"tradeIds":["t"]}`, "", "json", func(c *AuthenticatedClient) error { _, e := c.ApproveOrder(ctx, ApproveRFQOrderRequest{}); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				b, _ := io.ReadAll(r.Body)
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path || r.URL.RawQuery != tt.query {
					t.Errorf("got %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
				}
				if tt.body == "json" {
					if !json.Valid(b) {
						t.Errorf("invalid JSON body: %s", b)
					}
				} else if string(b) != tt.body {
					t.Errorf("body %s want %s", b, tt.body)
				}
				if r.Header.Get("POLY_API_KEY") != "key" || r.Header.Get("POLY_PASSPHRASE") != "pass" || r.Header.Get("POLY_ADDRESS") != (Address{}).Checksum() {
					t.Error("missing authentication headers")
				}
				message := r.Header.Get("POLY_TIMESTAMP") + r.Method + r.URL.EscapedPath() + strings.ReplaceAll(string(b), "'", `"`)
				mac := hmac.New(sha256.New, []byte("secret"))
				mac.Write([]byte(message))
				if r.Header.Get("POLY_SIGNATURE") != base64.URLEncoding.EncodeToString(mac.Sum(nil)) {
					t.Error("incorrect HMAC")
				}
				fmt.Fprint(w, tt.response)
			}))
			defer s.Close()
			c := testAuthenticatedClient(t, s.URL, AuthenticatedClientOptions{SignatureType: Proxy})
			if err := tt.call(c); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("got %d calls", calls)
			}
		})
	}
}

func settlementTestOrder(deferred bool) SignedOrder {
	return SignedOrder{SignableOrder: SignableOrder{
		Order:     Order{Version: 2, TokenID: big.NewInt(1), MakerAmount: big.NewInt(2), TakerAmount: big.NewInt(3), Timestamp: big.NewInt(4), Expiration: big.NewInt(0)},
		DeferExec: &deferred,
	}}
}

func boundSettlementTestOrder(address Address, owner string) SignedOrder {
	order := settlementTestOrder(false)
	order.Order.Maker = address
	order.Order.Signer = address
	order.Owner = owner
	return order
}

func TestPostOrdersSettlementPolling(t *testing.T) {
	hashA := "0x" + strings.Repeat("1", 64)
	hashB := "0x" + strings.Repeat("2", 64)
	existing := "0x" + strings.Repeat("3", 64)
	calls := map[string]int{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/orders":
			fmt.Fprintf(w, `[{%s,"tradeIds":["a","b","a","failed",""]},{%s,"tradeIds":["deferred"]},{%s,"tradeIds":["existing"],"transactionHashes":[%q]},{%s,"tradeIds":["b"]}]`, fixturePostFields, fixturePostFields, fixturePostFields, existing, fixturePostFields)
		case "/data/trades":
			if r.Header.Get("POLY_API_KEY") != "key" {
				t.Error("poll missing authentication")
			}
			id := r.URL.Query().Get("id")
			calls[id]++
			switch id {
			case "a":
				fmt.Fprintf(w, `{"data":[%s,%s],"next_cursor":"LTE=","limit":100,"count":2}`, fixtureTrade("other", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, hashB)), fixtureTrade("a", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, hashA)))
			case "b":
				if calls[id] == 1 {
					fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("b", "MATCHED", ""))
				} else {
					fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("b", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, hashB)))
				}
			case "failed":
				fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("failed", "FAILED", fmt.Sprintf(`,"transaction_hash":%q`, hashB)))
			default:
				t.Errorf("unexpected trade %q", id)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL)
	orders := []SignedOrder{settlementTestOrder(false), settlementTestOrder(true), settlementTestOrder(false)}
	responses, err := c.PostOrders(context.Background(), orders)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]B256{{B256(hashA), B256(hashB), B256(hashA)}, nil, {B256(existing)}, {B256(hashB)}}
	for i := range want {
		if !reflect.DeepEqual(responses[i].TransactionHashes, want[i]) && !(len(responses[i].TransactionHashes) == 0 && len(want[i]) == 0) {
			t.Errorf("response %d hashes %v, want %v", i, responses[i].TransactionHashes, want[i])
		}
	}
	if !reflect.DeepEqual(calls, map[string]int{"a": 1, "b": 2, "failed": 1}) {
		t.Fatalf("trade calls %v", calls)
	}
}

func TestPostOrderSettlementBestEffort(t *testing.T) {
	hash := "0x" + strings.Repeat("a", 64)
	var calls int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/order":
			fmt.Fprint(w, `{`+fixturePostFields+`,"tradeIds":["good","missing"]}`)
		case "/data/trades":
			calls++
			if r.URL.Query().Get("id") == "good" {
				fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("good", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, hash)))
			} else {
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			}
		}
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	response, err := c.PostOrder(ctx, settlementTestOrder(false))
	if err != nil || !reflect.DeepEqual(response.TransactionHashes, []B256{B256(hash)}) || calls < 2 {
		t.Fatalf("response=%+v err=%v calls=%d", response, err, calls)
	}
	calls = 0
	response, err = c.PostOrder(context.Background(), settlementTestOrder(true))
	if err != nil || calls != 0 || len(response.TransactionHashes) != 0 {
		t.Fatalf("deferred response=%+v err=%v calls=%d", response, err, calls)
	}
}

func TestPostOrderSettlementRetry(t *testing.T) {
	hash := "0x" + strings.Repeat("f", 64)
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/order" {
			fmt.Fprint(w, `{`+fixturePostFields+`,"tradeIds":["t"]}`)
			return
		}
		calls++
		switch calls {
		case 1:
			http.Error(w, "unavailable", 503)
		case 2:
			fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("t", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, "0x"+strings.Repeat("0", 64))))
		default:
			fmt.Fprintf(w, `{"data":[%s],"next_cursor":"LTE=","limit":100,"count":1}`, fixtureTrade("t", "MATCHED", fmt.Sprintf(`,"transaction_hash":%q`, hash)))
		}
	}))
	defer s.Close()
	response, err := testAuthenticatedClient(t, s.URL).PostOrder(context.Background(), settlementTestOrder(false))
	if err != nil || calls != 3 || !reflect.DeepEqual(response.TransactionHashes, []B256{B256(hash)}) {
		t.Fatalf("response=%+v err=%v calls=%d", response, err, calls)
	}
}

func TestPostOrderVersionRefreshOnlyOnMismatch(t *testing.T) {
	versionCalls, postCalls := 0, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			versionCalls++
			fmt.Fprint(w, `{"version":1}`)
			return
		}
		postCalls++
		http.Error(w, "another error", 400)
	}))
	defer s.Close()
	public, err := NewClient(s.URL, DefaultConfig(), s.Client())
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
	auth, err := public.Authenticate(context.Background(), &vectorSigner{address: Address{1}}, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	order := boundSettlementTestOrder(auth.address, creds.Key)
	bad := order
	bad.Owner = "another-owner"
	if _, err = auth.PostOrder(context.Background(), bad); err == nil || postCalls != 0 || versionCalls != 0 {
		t.Fatalf("identity mismatch: err=%v post calls=%d version calls=%d", err, postCalls, versionCalls)
	}
	_, err = auth.PostOrder(context.Background(), order)
	var status StatusError
	if !errors.As(err, &status) || status.StatusCode != 400 || !strings.Contains(status.Message, "another error") || postCalls != 1 || versionCalls != 0 {
		t.Fatalf("err=%v post calls=%d version calls=%d", err, postCalls, versionCalls)
	}
	// Standalone clients have no cache and must preserve the posting error.
	standalone := testAuthenticatedClient(t, s.URL)
	standalone.refreshVersionOnMismatch(context.Background(), NewStatusError(400, "POST", "/order", "order_version_mismatch"))
	if versionCalls != 0 {
		t.Fatalf("standalone made %d version calls", versionCalls)
	}
	// A non-status error containing the marker must not trigger a refresh.
	auth.refreshVersionOnMismatch(context.Background(), errors.New("order_version_mismatch"))
	if versionCalls != 0 {
		t.Fatalf("non-status error made %d version calls", versionCalls)
	}
}

func TestPostOrderVersionMismatchRefresh(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("batch=%t/refreshFails=%t", batch, refreshFails), func(t *testing.T) {
				versionCalls, postCalls := 0, 0
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/version":
						versionCalls++
						if versionCalls > 1 && refreshFails {
							http.Error(w, "refresh failed", 503)
						} else {
							fmt.Fprintf(w, `{"version":%d}`, versionCalls)
						}
					case "/order", "/orders":
						postCalls++
						http.Error(w, `{"error":"order_version_mismatch"}`, 400)
					default:
						t.Errorf("unexpected path %s", r.URL.Path)
					}
				}))
				defer s.Close()
				public, err := NewClient(s.URL, DefaultConfig(), s.Client())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = public.Version(context.Background()); err != nil {
					t.Fatal(err)
				}
				creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
				auth, err := public.Authenticate(context.Background(), &vectorSigner{address: Address{1}}, Polygon, AuthenticationOptions{Credentials: &creds})
				if err != nil {
					t.Fatal(err)
				}
				order := boundSettlementTestOrder(auth.address, creds.Key)
				if batch {
					_, err = auth.PostOrders(context.Background(), []SignedOrder{order})
				} else {
					_, err = auth.PostOrder(context.Background(), order)
				}
				var status StatusError
				if !errors.As(err, &status) || status.StatusCode != 400 || !strings.Contains(status.Message, "order_version_mismatch") {
					t.Fatalf("post error %v", err)
				}
				v, err := public.Version(context.Background())
				want := uint32(2)
				if refreshFails {
					want = 1
				}
				if err != nil || v != want || versionCalls != 2 || postCalls != 1 {
					t.Fatalf("version=%d err=%v versionCalls=%d postCalls=%d", v, err, versionCalls, postCalls)
				}
			})
		}
	}
}

func TestAuthenticatedErrorsAndTime(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/prefix/time" {
			fmt.Fprint(w, `12345`)
			return
		}
		if r.Header.Get("POLY_TIMESTAMP") != "12345" {
			t.Errorf("timestamp %q", r.Header.Get("POLY_TIMESTAMP"))
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "denied")
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL+"/prefix/", AuthenticatedClientOptions{UseServerTime: true})
	err := c.RevokeBuilderAPIKey(context.Background())
	var status StatusError
	if !errors.As(err, &status) || status.StatusCode != 403 || status.Method != "DELETE" || status.Path != "/prefix/auth/builder-api-key" || status.Message != "denied" {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(paths, []string{"/prefix/time", "/prefix/auth/builder-api-key"}) {
		t.Fatal(paths)
	}
}

func TestAuthenticatedFailurePaths(t *testing.T) {
	for _, host := range []string{"ftp://example.com", "http://", "https://user:password@example.com", "https://example.com/?q=x", "https://example.com/#x"} {
		if _, err := NewAuthenticatedClient(host, Address{}, Credentials{}); err == nil {
			t.Errorf("accepted %q", host)
		}
	}
	c := testAuthenticatedClient(t, "http://127.0.0.1:1")
	if _, err := c.BuilderTrades(context.Background(), B256("0x"+strings.Repeat("0", 64)), TradesRequest{}, nil); err == nil {
		t.Error("accepted zero builder")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.APIKeys(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	bad, err := NewAuthenticatedClient("http://127.0.0.1:1", Address{}, Credentials{Secret: "!"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bad.APIKeys(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid API secret") {
		t.Fatalf("bad secret: %v", err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "not json") }))
	defer s.Close()
	if _, err = testAuthenticatedClient(t, s.URL).APIKeys(context.Background()); err == nil {
		t.Fatal("accepted malformed JSON")
	}
}

func TestAuthenticatedNullResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, " null ") }))
	defer s.Close()
	_, err := testAuthenticatedClient(t, s.URL).APIKeys(context.Background())
	var status StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound {
		t.Fatalf("null response: %v", err)
	}
}

func TestAuthenticatedPaginationAndRedirect(t *testing.T) {
	var cursors []string
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/api-keys" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		cursor := r.URL.Query().Get("next_cursor")
		cursors = append(cursors, cursor)
		if cursor == "" {
			fmt.Fprint(w, `{"data":[],"next_cursor":"second+/=","limit":100,"count":0}`)
		} else {
			fmt.Fprint(w, `{"data":[],"next_cursor":"LTE=","limit":100,"count":0}`)
		}
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL)
	first, err := c.Orders(context.Background(), OrdersRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Orders(context.Background(), OrdersRequest{}, &first.NextCursor)
	if err != nil || second.NextCursor != TerminalCursor {
		t.Fatalf("page: %+v %v", second, err)
	}
	if !reflect.DeepEqual(cursors, []string{"", "second+/="}) {
		t.Fatal(cursors)
	}
	_, err = c.APIKeys(context.Background())
	var status StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusTemporaryRedirect || targetCalls != 0 {
		t.Fatalf("redirect: %v, target calls %d", err, targetCalls)
	}
}

func TestAuthenticatedHeartbeatRetryAndCancel(t *testing.T) {
	requests := make(chan string, 8)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID *string `json:"heartbeat_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		id := "nil"
		if body.ID != nil {
			id = *body.ID
		}
		requests <- id
		if id == "nil" {
			fmt.Fprint(w, `{"heartbeat_id":"00000000-0000-0000-0000-000000000002"}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL, AuthenticatedClientOptions{HeartbeatInterval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.StartHeartbeats(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.StopHeartbeats()
	for _, want := range []string{"nil", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000002"} {
		select {
		case got := <-requests:
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("heartbeat timeout")
		}
	}
	cancel()
	c.StopHeartbeats()
	if c.HeartbeatsActive() {
		t.Fatal("worker not stopped")
	}
}

func TestAuthenticatedHeartbeats(t *testing.T) {
	ids := make(chan string, 4)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID *string `json:"heartbeat_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.ID == nil {
			ids <- "nil"
		} else {
			ids <- *body.ID
		}
		fmt.Fprint(w, `{"heartbeat_id":"00000000-0000-0000-0000-000000000003"}`)
	}))
	defer s.Close()
	c := testAuthenticatedClient(t, s.URL, AuthenticatedClientOptions{HeartbeatInterval: 10 * time.Millisecond})
	if err := c.StartHeartbeats(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.StopHeartbeats()
	if err := c.StartHeartbeats(context.Background()); err == nil {
		t.Fatal("started twice")
	}
	for _, want := range []string{"nil", "00000000-0000-0000-0000-000000000003"} {
		select {
		case got := <-ids:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatal("heartbeat timeout")
		}
	}
	c.StopHeartbeats()
	if c.HeartbeatsActive() {
		t.Fatal("still active")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.StartHeartbeats(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	c.StopHeartbeats()
}

func TestBuilderFeeRateCacheAndInvalidation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 2 {
			close(started)
			<-release
		}
		fmt.Fprintf(w, `{"builderMakerFeeRateBps":%d,"builderTakerFeeRateBps":0}`, n)
	}))
	defer s.Close()
	public, err := NewClient(s.URL, DefaultConfig(), s.Client())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
	c, err := public.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	code := B256("0x" + strings.Repeat("1", 64))
	for i := 0; i < 2; i++ {
		r, err := c.BuilderFeeRate(context.Background(), code)
		if err != nil || r.BuilderMakerFeeRateBPS != 1 {
			t.Fatalf("%+v %v", r, err)
		}
	}
	public.InvalidateInternalCaches()
	done := make(chan error, 1)
	go func() { _, err := c.BuilderFeeRate(context.Background(), code); done <- err }()
	<-started
	public.InvalidateInternalCaches()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r, err := c.BuilderFeeRate(context.Background(), code)
	if err != nil || r.BuilderMakerFeeRateBPS != 3 || calls.Load() != 3 {
		t.Fatalf("%+v %v, calls %d", r, err, calls.Load())
	}
	if _, err := c.BuilderFeeRate(context.Background(), B256("invalid")); err == nil {
		t.Fatal("expected invalid builder code error")
	}
}

func TestBuilderFeeRateRequiredFieldsAndAliases(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"builderMakerFeeRateBps":0}`,
		`{"builderTakerFeeRateBps":0}`,
		`{"builderMakerFeeRateBps":null,"builderTakerFeeRateBps":0}`,
		`{"builderMakerFeeRateBps":0,"builderTakerFeeRateBps":null}`,
		`{"builderMakerFeeRateBps":0,"builderTakerFeeRateBps":"5"}`,
		`{"builderMakerFeeRateBps":0,"builder_maker_fee_rate_bps":0,"builderTakerFeeRateBps":0}`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					fmt.Fprint(w, body)
				} else {
					fmt.Fprint(w, `{"builder_maker_fee_rate_bps":0,"builder_taker_fee_rate_bps":5}`)
				}
			}))
			defer s.Close()
			c, err := NewAuthenticatedClient(s.URL, Address{}, Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"})
			if err != nil {
				t.Fatal(err)
			}
			code := B256("0x" + strings.Repeat("ab", 32))
			if _, err := c.BuilderFeeRate(context.Background(), code); err == nil {
				t.Fatal("accepted incomplete or malformed fee response")
			}
			fee, err := c.BuilderFeeRate(context.Background(), code)
			if err != nil || fee.BuilderMakerFeeRateBPS != 0 || fee.BuilderTakerFeeRateBPS != 5 {
				t.Fatalf("retry: %+v %v", fee, err)
			}
			fee, err = c.BuilderFeeRate(context.Background(), B256("0x"+strings.Repeat("AB", 32)))
			if err != nil || fee.BuilderTakerFeeRateBPS != 5 || calls.Load() != 2 {
				t.Fatalf("canonical cache: %+v %v, calls %d", fee, err, calls.Load())
			}
		})
	}
}

func TestBuilderCredentialsRequireFields(t *testing.T) {
	for _, body := range []string{`{}`, `{"apiKey":"key","secret":null,"passphrase":"p"}`, `{"key":"k","apiKey":"k","secret":"s","passphrase":"p"}`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer s.Close()
			c := testAuthenticatedClient(t, s.URL)
			if _, err := c.CreateBuilderAPIKey(context.Background()); err == nil {
				t.Fatal("expected credentials decode error")
			}
		})
	}
}

func TestHeartbeatNilContext(t *testing.T) {
	c := testAuthenticatedClient(t, "http://localhost")
	if err := c.StartHeartbeats(nil); err == nil {
		t.Fatal("expected nil context error")
	}
}

func TestL1APIKeyEndpoints(t *testing.T) {
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	const signature = "0xf62319a987514da40e57e2f4d7529f7bac38f0355bd88bb5adbb3768d80de6c1682518e0af677d5260366425f4361e7b70c25ae232aff0ab2331e2b164a1aedc1b"
	var routes []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes = append(routes, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/prefix/time" {
			fmt.Fprint(w, "10000000")
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 || r.URL.RawQuery != "" {
			t.Errorf("unexpected request body/query: %q %q", body, r.URL.RawQuery)
		}
		if r.Header.Get("POLY_ADDRESS") != signer.Address().String() || r.Header.Get("POLY_NONCE") != "23" || r.Header.Get("POLY_TIMESTAMP") != "10000000" || r.Header.Get("POLY_SIGNATURE") != signature {
			t.Errorf("incorrect L1 headers: %v", r.Header)
		}
		fmt.Fprint(w, `{"apiKey":"key","secret":"c2VjcmV0","passphrase":"pass"}`)
	}))
	defer s.Close()
	config := DefaultConfig()
	config.UseServerTime = true
	client, err := NewClient(s.URL+"/prefix", config, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	nonce := uint32(23)
	for _, call := range []func(context.Context, HashSigner, uint64, *uint32) (Credentials, error){client.CreateAPIKey, client.DeriveAPIKey, client.CreateOrDeriveAPIKey} {
		creds, err := call(context.Background(), signer, Amoy, &nonce)
		if err != nil || creds.Key != "key" || creds.Secret != "c2VjcmV0" || creds.Passphrase != "pass" {
			t.Fatalf("credentials %v: %v", creds, err)
		}
	}
	want := []string{"GET /prefix/time", "POST /prefix/auth/api-key", "GET /prefix/time", "GET /prefix/auth/derive-api-key", "GET /prefix/time", "POST /prefix/auth/api-key"}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes %v", routes)
	}
}

func TestL1CreateOrDeriveStatusFallback(t *testing.T) {
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		status       int
		body         string
		deriveStatus int
	}{
		{"conflict", http.StatusConflict, "exists", http.StatusOK},
		{"server error", http.StatusInternalServerError, "error", http.StatusOK},
		{"null response is status", http.StatusOK, "null", http.StatusOK},
		{"derive error", http.StatusConflict, "exists", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var routes []string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				routes = append(routes, r.Method+" "+r.URL.Path)
				if r.Header.Get("POLY_NONCE") != "0" || r.Header.Get("POLY_SIGNATURE") == "" {
					t.Errorf("bad L1 headers: %v", r.Header)
				}
				if r.Method == http.MethodPost {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
					return
				}
				w.WriteHeader(tc.deriveStatus)
				if tc.deriveStatus == http.StatusOK {
					fmt.Fprint(w, `{"key":"derived","secret":"cw==","passphrase":"p"}`)
				} else {
					fmt.Fprint(w, "denied")
				}
			}))
			defer s.Close()
			c, err := NewClient(s.URL, DefaultConfig(), s.Client())
			if err != nil {
				t.Fatal(err)
			}
			creds, err := c.CreateOrDeriveAPIKey(context.Background(), signer, Polygon, nil)
			if tc.deriveStatus == http.StatusOK {
				if err != nil || creds.Key != "derived" {
					t.Fatalf("%v %v", creds, err)
				}
			} else {
				var status StatusError
				if !errors.As(err, &status) || status.StatusCode != tc.deriveStatus || status.Path != "/auth/derive-api-key" {
					t.Fatalf("%v", err)
				}
			}
			if !reflect.DeepEqual(routes, []string{"POST /auth/api-key", "GET /auth/derive-api-key"}) {
				t.Fatal(routes)
			}
		})
	}
}

type l1FailSigner struct{ err error }

func (s l1FailSigner) Address() Address              { return Address{} }
func (s l1FailSigner) SignHash(Hash) ([]byte, error) { return nil, s.err }

type l1RoundTripFunc func(*http.Request) (*http.Response, error)

func (f l1RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestL1CreateOrDeriveNonStatusErrors(t *testing.T) {
	badSign := errors.New("sign failed")
	badNetwork := errors.New("network failed")
	for _, tc := range []struct {
		name      string
		signer    HashSigner
		transport http.RoundTripper
		want      error
		wantCalls int
	}{
		{"signing", l1FailSigner{badSign}, l1RoundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unexpected HTTP request")
			return nil, badNetwork
		}), badSign, 0},
		{"network", &vectorSigner{signature: append(make([]byte, 64), 27)}, l1RoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, badNetwork }), badNetwork, 1},
		{"decode", &vectorSigner{signature: append(make([]byte, 64), 27)}, l1RoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("invalid")), Header: http.Header{}}, nil
		}), nil, 1},
		{"missing fields", &vectorSigner{signature: append(make([]byte, 64), 27)}, l1RoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"apiKey":"key"}`)), Header: http.Header{}}, nil
		}), nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			hc := &http.Client{Transport: l1RoundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return tc.transport.RoundTrip(r) })}
			c, err := NewClient("http://example.com", DefaultConfig(), hc)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CreateOrDeriveAPIKey(context.Background(), tc.signer, Polygon, nil)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) || calls != tc.wantCalls {
				t.Fatalf("error %v, calls %d", err, calls)
			}
		})
	}
	c, err := NewClient("http://example.com", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	var nilSigner *LocalSigner
	if _, err = c.CreateAPIKey(context.Background(), nilSigner, Polygon, nil); err == nil {
		t.Fatal("accepted typed nil signer")
	}
	if _, err = c.CreateAPIKey(context.Background(), l1FailSigner{}, 0, nil); err == nil {
		t.Fatal("accepted missing chain")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.CreateOrDeriveAPIKey(ctx, &vectorSigner{signature: append(make([]byte, 64), 27)}, Polygon, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context: %v", err)
	}
}

func TestL1ServerTimeFailureFallback(t *testing.T) {
	signer := &vectorSigner{signature: append(make([]byte, 64), 27)}
	for _, tc := range []struct {
		name       string
		timeStatus int
		timeBody   string
		wantPaths  []string
	}{
		{"status error retries", http.StatusServiceUnavailable, "down", []string{"/time", "/time"}},
		{"decode error does not retry", http.StatusOK, "invalid", []string{"/time"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path != "/time" {
					t.Errorf("unexpected key request: %s", r.URL.Path)
				}
				w.WriteHeader(tc.timeStatus)
				fmt.Fprint(w, tc.timeBody)
			}))
			defer s.Close()
			config := DefaultConfig()
			config.UseServerTime = true
			c, err := NewClient(s.URL, config, s.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CreateOrDeriveAPIKey(context.Background(), signer, Polygon, nil)
			if err == nil || !reflect.DeepEqual(paths, tc.wantPaths) {
				t.Fatalf("error %v, paths %v", err, paths)
			}
		})
	}
}

func TestL1RedirectNotForwarded(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/auth/api-key" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		fmt.Fprint(w, `{"apiKey":"derived","secret":"cw==","passphrase":"p"}`)
	}))
	defer s.Close()
	c, err := NewClient(s.URL, DefaultConfig(), s.Client())
	if err != nil {
		t.Fatal(err)
	}
	signer := &vectorSigner{signature: append(make([]byte, 64), 27)}
	credentials, err := c.CreateOrDeriveAPIKey(context.Background(), signer, Polygon, nil)
	if err != nil || credentials.Key != "derived" || targetCalls != 0 || !reflect.DeepEqual(paths, []string{"/auth/api-key", "/auth/derive-api-key"}) {
		t.Fatalf("redirect: %v, paths %v, target %d", err, paths, targetCalls)
	}
}

func TestClientAuthenticateIntegration(t *testing.T) {
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes = append(routes, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/prefix/time":
			fmt.Fprint(w, "10000000")
		case "/prefix/auth/api-key":
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, "exists")
		case "/prefix/auth/derive-api-key":
			fmt.Fprint(w, `{"apiKey":"key","secret":"c2VjcmV0","passphrase":"pass"}`)
		case "/prefix/notifications":
			if r.URL.Query().Get("signature_type") != "1" || r.Header.Get("POLY_API_KEY") != "key" || r.Header.Get("POLY_ADDRESS") != signer.Address().Checksum() || r.Header.Get("POLY_TIMESTAMP") != "10000000" {
				t.Errorf("bad authenticated request %v %s", r.Header, r.URL)
			}
			mac := hmac.New(sha256.New, []byte("secret"))
			mac.Write([]byte("10000000GET/prefix/notifications"))
			if r.Header.Get("POLY_SIGNATURE") != base64.URLEncoding.EncodeToString(mac.Sum(nil)) {
				t.Error("bad L2 signature")
			}
			fmt.Fprint(w, `[]`)
		case "/prefix/":
			fmt.Fprint(w, `"OK"`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer s.Close()
	config := DefaultConfig()
	config.UseServerTime = true
	config.HeartbeatInterval = 17 * time.Second
	c, err := NewClient(s.URL+"/prefix", config, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	nonce := uint32(23)
	a, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Nonce: &nonce, SignatureType: Proxy})
	if err != nil {
		t.Fatal(err)
	}
	if a.heartbeatInterval != 17*time.Second {
		t.Fatal("heartbeat configuration not copied")
	}
	if _, err = a.Notifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = c.OK(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /prefix/time", "POST /prefix/auth/api-key", "GET /prefix/time", "GET /prefix/auth/derive-api-key", "GET /prefix/time", "GET /prefix/notifications", "GET /prefix/"}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("%v", routes)
	}
}

func TestClientAuthenticateExistingCredentialsAndValidation(t *testing.T) {
	calls := 0
	hc := &http.Client{Transport: l1RoundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected request") })}
	c, err := NewClient("http://example.com", DefaultConfig(), hc)
	if err != nil {
		t.Fatal(err)
	}
	signer := &vectorSigner{address: Address{1}}
	creds := Credentials{Key: "key", Secret: "c2VjcmV0", Passphrase: "pass"}
	a, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds, SignatureType: SignatureSafe})
	if err != nil {
		t.Fatal(err)
	}
	creds.Key = "mutated"
	if a.credentials.Key != "key" || a.address != signer.Address() || a.signatureType != SignatureSafe || calls != 0 {
		t.Fatal("existing credentials not copied without network")
	}
	nonce := uint32(0)
	for _, tc := range []struct {
		signer HashSigner
		chain  uint64
		opts   []AuthenticationOptions
	}{
		{nil, Polygon, nil}, {(*LocalSigner)(nil), Polygon, nil}, {signer, 1, nil},
		{signer, Polygon, []AuthenticationOptions{{Credentials: &creds, Nonce: &nonce}}},
		{signer, Polygon, []AuthenticationOptions{{SignatureType: SignatureType(255)}}},
		{signer, Polygon, []AuthenticationOptions{{}, {}}},
	} {
		if _, err := c.Authenticate(context.Background(), tc.signer, tc.chain, tc.opts...); err == nil {
			t.Error("accepted invalid authentication options")
		}
	}
	if calls != 0 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}
