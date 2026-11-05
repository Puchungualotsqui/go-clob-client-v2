package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutesAndBodies(t *testing.T) {
	ctx := context.Background()
	var path, method, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			t.Errorf("got %s %s; want %s %s", r.Method, r.URL.Path, method, path)
		}
		if method == "POST" {
			var b map[string]any
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				t.Error(err)
			}
			if path == "/quote" && (b["fromChainId"] != "137" || b["toChainId"] != "1" || b["fromAmountBaseUnit"] != "123456789012345678901234567890") {
				t.Errorf("body %v", b)
			}
			if path == "/withdraw" && b["toChainId"] != "1" {
				t.Errorf("body %v", b)
			}
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, srv.Client())
	cases := []struct {
		path, method, body string
		call               func() error
	}{
		{"/deposit", "POST", `{"address":{"evm":"0x56687bf447db6ffa42ffe2204a05edaa20f55839","svm":"sol","btc":"btc"}}`, func() error {
			v, e := c.Deposit(ctx, DepositRequest{Address: testEVM})
			if e == nil && v.Address.SVM != "sol" {
				t.Error(v)
			}
			return e
		}},
		{"/withdraw", "POST", `{"address":{"evm":"0x56687bf447db6ffa42ffe2204a05edaa20f55839","svm":"sol","btc":"btc"},"note":"ok"}`, func() error {
			_, e := c.Withdraw(ctx, WithdrawRequest{Address: testEVM, ToChainID: 1, ToTokenAddress: testEVM, RecipientAddr: testEVM})
			return e
		}},
		{"/supported-assets", "GET", `{"supportedAssets":[{"chainId":"137","chainName":"Polygon","token":{"name":"USD Coin","symbol":"USDC","address":"0x3c499c542cef5e3811e1192ce70d8cc03d5c3359","decimals":6},"minCheckoutUsd":1.234567890123456789}]}`, func() error {
			v, e := c.SupportedAssets(ctx)
			if e == nil && (v.SupportedAssets[0].ChainID != 137 || v.SupportedAssets[0].MinCheckoutUSD != "1.234567890123456789") {
				t.Error(v)
			}
			return e
		}},
		{"/status/abc", "GET", `{"transactions":[{"fromChainId":"137","fromTokenAddress":"sol","toChainId":"1","toTokenAddress":"0x56687bf447db6ffa42ffe2204a05edaa20f55839","fromAmountBaseUnit":"999999999999999999999999","status":"COMPLETED"}]}`, func() error {
			v, e := c.Status(ctx, StatusRequest{Address: "abc"})
			if e == nil && (v.Transactions[0].Status != Completed || v.Transactions[0].FromAmountBaseUnit != "999999999999999999999999") {
				t.Error(v)
			}
			return e
		}},
		{"/quote", "POST", `{"estCheckoutTimeMs":0,"estFeeBreakdown":` + testFees + `,"estInputUsd":0,"estOutputUsd":0,"estToTokenBaseUnit":"999999999999999999999999","quoteId":"q"}`, func() error {
			v, e := c.Quote(ctx, QuoteRequest{FromChainID: 137, ToChainID: 1, FromAmountBaseUnit: "123456789012345678901234567890", FromTokenAddress: testEVM, ToTokenAddress: testEVM, RecipientAddress: testEVM})
			if e == nil && v.EstToTokenBaseUnit != "999999999999999999999999" {
				t.Error(v)
			}
			return e
		}},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			path, method, body = tt.path, tt.method, tt.body
			if e := tt.call(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestNullDepositResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "null")
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	_, err := c.Deposit(context.Background(), DepositRequest{Address: testEVM})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("null response: %v", err)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failure", http.StatusBadRequest) }))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	_, err := c.SupportedAssets(context.Background())
	var e *HTTPError
	if !errors.As(err, &e) || e.StatusCode != 400 {
		t.Fatal(err)
	}
}
