package gamma

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTypedRequestQueries(t *testing.T) {
	date := "2024-02-29T23:59:59.123456789+02:30"
	decimal := Number("0.1234567890123456789012345678")
	hash := "0x" + strings.Repeat("ab", 32)
	address := "0x" + strings.Repeat("12", 20)
	maxToken := Number("115792089237316195423570985008687907853269984665640564039457584007913129639935")
	req := MarketsRequest{
		ID: []string{"opaque/market", ""}, Slug: []string{"opaque slug"}, TagID: stringPtr("not-a-number"), GameID: stringPtr("game:x"),
		CLOBTokenIDs: []Number{"0", maxToken}, ConditionIDs: []string{hash}, QuestionIDs: []string{hash}, MarketMakerAddress: []string{address},
		StartDateMin: &date, StartDateMax: &date, EndDateMin: &date, EndDateMax: &date,
		LiquidityNumMin: &decimal, LiquidityNumMax: &decimal, VolumeNumMin: &decimal, VolumeNumMax: &decimal, RewardsMinSize: &decimal,
	}
	for _, value := range []any{req, &req} {
		q, err := queryValues(value)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(q["id"], req.ID) || !reflect.DeepEqual(q["clob_token_ids"], []string{"0", string(maxToken)}) || q.Get("condition_ids") != hash || q.Get("market_maker_address") != address || q.Get("question_ids") != hash || q.Get("end_date_max") != date || q.Get("rewards_min_size") != string(decimal) {
			t.Fatalf("query: %v", q)
		}
	}
	events := EventsRequest{ID: []string{"not-an-integer"}, TagID: stringPtr("opaque"), ExcludeTagID: []string{"tag:1"}, StartDateMin: &date, StartDateMax: &date, EndDateMin: &date, EndDateMax: &date, LiquidityMin: &decimal, LiquidityMax: &decimal, VolumeMin: &decimal, VolumeMax: &decimal}
	if _, err := queryValues(events); err != nil {
		t.Fatal(err)
	}
	if _, err := queryValues((*MarketsRequest)(nil)); err != nil {
		t.Fatalf("nil request: %v", err)
	}
}

func TestRequestTypedFieldValidation(t *testing.T) {
	badDate := "2025-02-29T12:00:00Z"
	badDecimal := Number("abc")
	goodHash := "0x" + strings.Repeat("ab", 32)
	goodAddress := "0x" + strings.Repeat("12", 20)
	cases := []struct {
		name  string
		req   any
		field string
	}{
		{"event start min", EventsRequest{StartDateMin: &badDate}, "start_date_min"},
		{"event start max", EventsRequest{StartDateMax: &badDate}, "start_date_max"},
		{"event end min", EventsRequest{EndDateMin: &badDate}, "end_date_min"},
		{"event end max", EventsRequest{EndDateMax: &badDate}, "end_date_max"},
		{"market start min", MarketsRequest{StartDateMin: &badDate}, "start_date_min"},
		{"market start max", MarketsRequest{StartDateMax: &badDate}, "start_date_max"},
		{"market end min", MarketsRequest{EndDateMin: &badDate}, "end_date_min"},
		{"market end max", MarketsRequest{EndDateMax: &badDate}, "end_date_max"},
		{"event liquidity min", EventsRequest{LiquidityMin: &badDecimal}, "liquidity_min"},
		{"event liquidity max", EventsRequest{LiquidityMax: &badDecimal}, "liquidity_max"},
		{"event volume min", EventsRequest{VolumeMin: &badDecimal}, "volume_min"},
		{"event volume max", EventsRequest{VolumeMax: &badDecimal}, "volume_max"},
		{"market liquidity min", MarketsRequest{LiquidityNumMin: &badDecimal}, "liquidity_num_min"},
		{"market liquidity max", MarketsRequest{LiquidityNumMax: &badDecimal}, "liquidity_num_max"},
		{"market volume min", MarketsRequest{VolumeNumMin: &badDecimal}, "volume_num_min"},
		{"market volume max", MarketsRequest{VolumeNumMax: &badDecimal}, "volume_num_max"},
		{"market rewards", MarketsRequest{RewardsMinSize: &badDecimal}, "rewards_min_size"},
		{"fractional token", MarketsRequest{CLOBTokenIDs: []Number{"1.2"}}, "clob_token_ids[0]"},
		{"negative token", MarketsRequest{CLOBTokenIDs: []Number{"-1"}}, "clob_token_ids[0]"},
		{"overflow token", MarketsRequest{CLOBTokenIDs: []Number{"115792089237316195423570985008687907853269984665640564039457584007913129639936"}}, "clob_token_ids[0]"},
		{"empty hash", MarketsRequest{ConditionIDs: []string{goodHash, ""}}, "condition_ids[1]"},
		{"short hash", MarketsRequest{QuestionIDs: []string{"0x12"}}, "question_ids[0]"},
		{"invalid hex", MarketsRequest{ConditionIDs: []string{"0x" + strings.Repeat("zz", 32)}}, "condition_ids[0]"},
		{"empty maker", MarketsRequest{MarketMakerAddress: []string{goodAddress, ""}}, "market_maker_address[1]"},
		{"missing prefix", MarketsRequest{MarketMakerAddress: []string{strings.Repeat("12", 20)}}, "market_maker_address[0]"},
		{"missing profile address", PublicProfileRequest{}, "address"},
		{"malformed path address", CommentsByUserAddressRequest{UserAddress: "a"}, "user_address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := queryValues(tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected error mentioning %s; got %v", tc.field, err)
			}
		})
	}
}

func TestRequestDecimalBoundaries(t *testing.T) {
	valid := []string{"0", "-0.00000000000000000000000000000", "1e-28", "0.00000000000000000000000000010", "79228162514264337593543950335", "7.9228162514264337593543950335e28", "7922816251426433759354395033.5", "79228162514264337593543950335.0", "-12.3400", "+1"}

	invalid := []string{"", "NaN", "1x", "1e-29", "0.00000000000000000000000000001", "79228162514264337593543950336", "7922816251426433759354395033.6", "79228162514264337593543950335.1", "1e29", "1e999999999999999999999", "1e-999999999999999999999"}
	for _, s := range valid {
		if err := validateRequestDecimal(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range invalid {
		if err := validateRequestDecimal(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestRequestTimestamps(t *testing.T) {
	for _, s := range []string{"2024-02-29T12:34:56Z", "2025-01-01T00:00:00.000000001-05:30"} {
		if err := validateRequestTimestamp(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "2025-01-01", "2025-01-01T00:00:00", "2025-02-29T00:00:00Z", "2025-01-01T00:00:00+24:00", "2025-01-01T00:00:00+02:60", "2025-01-01T1:00:00Z"} {
		if err := validateRequestTimestamp(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestRequestValidationBeforeTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasPrefix(r.URL.Path, "/comments/") {
			fmt.Fprint(w, `[]`)
		} else {
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.CommentsByUserAddress(ctx, CommentsByUserAddressRequest{UserAddress: "bad"}); err == nil {
		t.Fatal("accepted invalid path address")
	}
	if _, err := client.PublicProfile(ctx, PublicProfileRequest{Address: ""}); err == nil {
		t.Fatal("accepted invalid query address")
	}
	if _, err := client.Markets(ctx, MarketsRequest{ConditionIDs: []string{"bad"}}); err == nil {
		t.Fatal("accepted invalid hash")
	}
	if calls != 0 {
		t.Fatalf("sent %d invalid requests", calls)
	}
	address := "0x" + strings.Repeat("12", 20)
	if _, err := client.CommentsByUserAddress(ctx, CommentsByUserAddressRequest{UserAddress: address}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PublicProfile(ctx, PublicProfileRequest{Address: address}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("sent %d valid requests", calls)
	}
}

func stringPtr(s string) *string { return &s }
