package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	ctx := context.Background()
	var wantPath, response string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != wantPath {
			t.Errorf("got %s %s, want GET %s", r.Method, r.URL.Path, wantPath)
		}
		fmt.Fprint(w, response)
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path, body string
		call       func() error
	}{
		{"/", `{"data":"OK"}`, func() error { _, e := c.Health(ctx); return e }},
		{"/positions", "[]", func() error { _, e := c.Positions(ctx, PositionsRequest{User: testAddress}); return e }},
		{"/trades", "[]", func() error { _, e := c.Trades(ctx, TradesRequest{}); return e }},
		{"/activity", "[]", func() error { _, e := c.Activity(ctx, ActivityRequest{User: testAddress}); return e }},
		{"/holders", "[]", func() error { _, e := c.Holders(ctx, HoldersRequest{}); return e }},
		{"/value", "[]", func() error { _, e := c.Value(ctx, ValueRequest{User: testAddress}); return e }},
		{"/closed-positions", "[]", func() error { _, e := c.ClosedPositions(ctx, ClosedPositionsRequest{User: testAddress}); return e }},
		{"/v1/leaderboard", "[]", func() error { _, e := c.Leaderboard(ctx, TraderLeaderboardRequest{}); return e }},
		{"/traded", `{"user":"` + testAddress + `","traded":0}`, func() error { _, e := c.Traded(ctx, TradedRequest{User: testAddress}); return e }},
		{"/oi", "[]", func() error { _, e := c.OpenInterest(ctx, OpenInterestRequest{}); return e }},
		{"/live-volume", "[]", func() error { _, e := c.LiveVolume(ctx, LiveVolumeRequest{}); return e }},
		{"/v1/builders/leaderboard", "[]", func() error { _, e := c.BuilderLeaderboard(ctx, BuilderLeaderboardRequest{}); return e }},
		{"/v1/builders/volume", "[]", func() error { _, e := c.BuilderVolume(ctx, BuilderVolumeRequest{}); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			wantPath, response = tt.path, tt.body
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQueryAndDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("market") != testHash+","+testHash2 || q.Get("takerOnly") != "false" || q.Get("filterType") != "CASH" || q.Get("filterAmount") != "0.100000000000000001" {
			t.Errorf("query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[`+testTrade+`]`)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	b := false
	got, err := c.Trades(context.Background(), TradesRequest{Filter: &MarketFilter{Markets: []string{testHash, testHash2}}, TakerOnly: &b, TradeFilter: &TradeFilter{FilterType: "CASH", FilterAmount: "0.100000000000000001"}})
	if err != nil || len(got) != 1 || got[0].Asset != "123456789012345678901234567890" || got[0].Price != "0.1234567890123456789" || got[0].Name != nil {
		t.Fatalf("%+v %v", got, err)
	}
	_, err = c.Positions(context.Background(), PositionsRequest{Filter: &MarketFilter{Markets: []string{"a"}, EventIDs: []string{"b"}}})
	if err == nil {
		t.Fatal("expected invalid filter error")
	}
	limit := int32(501)
	_, err = c.Positions(context.Background(), PositionsRequest{Limit: &limit})
	if err == nil {
		t.Fatal("expected invalid limit error")
	}
}

func TestLeaderboardRankParsing(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  QuotedInt
		valid bool
	}{
		{`"08"`, 8, true},
		{`12`, 12, true},
		{`"1.5"`, 0, false},
		{`"12oops"`, 0, false},
		{`"2147483648"`, 0, false},
		{`null`, 0, false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			var entry TraderLeaderboardEntry
			err := json.Unmarshal([]byte(`{"rank":`+tt.input+`,"proxyWallet":"`+testAddress+`","vol":0,"pnl":0}`), &entry)
			if (err == nil) != tt.valid || (err == nil && entry.Rank != tt.want) {
				t.Fatalf("rank=%d err=%v", entry.Rank, err)
			}
		})
	}
}

func TestLiveVolumeNullMarketsAndNullResponse(t *testing.T) {
	var body = `[{"total":"1","markets":null},{"total":"2"},{"total":"3","markets":[{"market":"global","value":"3"}]}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	got, err := c.LiveVolume(context.Background(), LiveVolumeRequest{ID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Markets == nil || got[1].Markets == nil || len(got[2].Markets) != 1 {
		t.Fatalf("markets: %+v", got)
	}
	var direct LiveVolume
	if err := json.Unmarshal([]byte(`{"total":0,"markets":null}`), &direct); err != nil || direct.Markets == nil {
		t.Fatalf("direct decode: %+v, %v", direct, err)
	}
	body = "null"
	_, err = c.LiveVolume(context.Background(), LiveVolumeRequest{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("null response: %v", err)
	}
}

func TestHTTPErrorAndContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", http.StatusTooManyRequests) }))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	_, err := c.Health(context.Background())
	var e *HTTPError
	if !errors.As(err, &e) || e.StatusCode != 429 || e.Body != "bad\n" {
		t.Fatalf("%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Health(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
