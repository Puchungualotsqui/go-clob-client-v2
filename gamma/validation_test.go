package gamma

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidatedNestedResponse(t *testing.T) {
	body := `{"id":"e1","startDate":"2025-01-02T03:04:05.123Z","eventDate":"2025-01-02","parentEventId":"parent-alpha","turnProviderId":42,"liquidity":12345678901234567890.123456789,"markets":[{"id":"m1","conditionId":"0x` + strings.Repeat("ab", 32) + `","marketMakerAddress":"0x` + strings.Repeat("12", 20) + `","endDateIso":"2024-02-29","outcomePrices":"[\"0.1234567890123456789012345678\",\"0.9\"]","clobTokenIds":"[\"115792089237316195423570985008687907853269984665640564039457584007913129639935\"]","clobRewards":[{"assetAddress":"","startDate":"2025-01-01"}]}]}`
	var event Event
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatal(err)
	}
	if *event.Liquidity != "12345678901234567890.123456789" || *event.ParentEventID != "parent-alpha" || *event.TurnProviderID != "42" {
		t.Fatalf("event: %+v", event)
	}
	market := (*event.Markets)[0]
	if (*market.OutcomePrices)[0] != "0.1234567890123456789012345678" || (*market.CLOBRewards)[0].AssetAddress != nil {
		t.Fatalf("market: %+v", market)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var again Event
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatalf("round trip: %v: %s", err, encoded)
	}
}

func TestResponseValidationFailures(t *testing.T) {
	cases := []struct {
		name, body string
		dst        any
	}{
		{"missing event id", `{}`, &Event{}},
		{"null market id", `{"id":null}`, &Market{}},
		{"null team id", `{"id":null}`, &Team{}},
		{"missing sports field", `{"sport":"nba","tags":""}`, &SportsMetadata{}},
		{"null sports tags", `{"sport":"nba","image":"","resolution":"","ordering":"","tags":null,"series":""}`, &SportsMetadata{}},
		{"missing market types", `{}`, &SportsMarketTypesResponse{}},
		{"nested missing id", `{"id":"1","tags":[{}]}`, &Event{}},
		{"null nested market", `{"id":"1","markets":[null]}`, &Event{}},
		{"null subevent", `{"id":"1","subEvents":["1",null]}`, &Event{}},
		{"null reward", `{"id":"1","clobRewards":[null]}`, &Market{}},
		{"null profile user", `{"users":[null]}`, &PublicProfile{}},
		{"null search tag", `{"tags":[null]}`, &SearchResults{}},
		{"null positions", `{"positions":[null]}`, &CommentProfile{}},
		{"decimal text", `{"id":"1","volume":"not-a-number"}`, &Market{}},
		{"decimal array null", `{"id":"1","outcomePrices":"[null]"}`, &Market{}},
		{"string array null", `{"id":"1","outcomes":"[null]"}`, &Market{}},
		{"encoded null array", `{"id":"1","outcomes":"null"}`, &Market{}},

		{"bad hash", `{"id":"1","conditionId":"0x1234"}`, &Market{}},
		{"bad address", `{"proxyWallet":"0xgggggggggggggggggggggggggggggggggggggggg"}`, &PublicProfile{}},
		{"bad nested hash", `{"id":"1","markets":[{"id":"2","questionID":"garbage"}]}`, &Event{}},
		{"bad timestamp", `{"id":"1","createdAt":"2025-02-30T00:00:00Z"}`, &Tag{}},
		{"timestamp without zone", `{"id":"1","startDate":"2025-01-01T00:00:00"}`, &Event{}},
		{"bad date", `{"id":"1","eventDate":"2025-02-29"}`, &Event{}},
		{"date contains time", `{"startDate":"2025-01-01T00:00:00Z"}`, &ClobReward{}},
		{"negative token", `{"tokenId":"-1"}`, &CommentPosition{}},
		{"fractional token", `{"id":"1","clobTokenIds":"[\"1.2\"]"}`, &Market{}},
		{"overflow token", `{"id":"1","clobTokenIds":"[\"115792089237316195423570985008687907853269984665640564039457584007913129639936\"]"}`, &Market{}},
		{"float identifier", `{"id":1.5}`, &RelatedTag{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.body), tc.dst); err == nil {
				t.Fatalf("accepted %s", tc.body)
			}
		})
	}
}

func TestNumberAndFlexibleID(t *testing.T) {
	for _, input := range []string{`"12.3400"`, `1.25e-9`, `"-0.1"`, `"+12"`} {
		var n Number
		if err := json.Unmarshal([]byte(input), &n); err != nil {
			t.Errorf("%s: %v", input, err)
		}
	}
	for _, input := range []string{`""`, `"NaN"`, `"12oops"`, `true`, `null`, `" 1"`} {
		var n Number
		if err := json.Unmarshal([]byte(input), &n); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	if _, err := json.Marshal(Number("not-a-number")); err == nil {
		t.Fatal("marshaled invalid decimal")
	}
	var tag RelatedTag
	if err := json.Unmarshal([]byte(`{"id":"sports-tag","tagID":123,"relatedTagID":"other"}`), &tag); err != nil {
		t.Fatal(err)
	}
	if tag.ID != "sports-tag" || *tag.TagID != "123" || *tag.RelatedTagID != "other" {
		t.Fatalf("%+v", tag)
	}
}

func TestExactResponseFields(t *testing.T) {
	good := "0x" + strings.Repeat("12", 32)
	other := "0x" + strings.Repeat("ab", 32)
	var event Event
	body := `{"id":"1","negRiskMarketID":"` + good + `","negriskmarketid":"` + other + `"}`
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatal(err)
	}
	if event.NegRiskMarketID == nil || *event.NegRiskMarketID != good {
		t.Fatalf("case variant overwrote identifier: %+v", event)
	}
	var tag Tag
	if err := json.Unmarshal([]byte(`{"id":"1","ID":null,"CreatedAt":"garbage","Label":"ignored"}`), &tag); err != nil || tag.Label != nil || tag.CreatedAt != nil || tag.ID != "1" {
		t.Fatalf("case variant populated optional field: %+v, %v", tag, err)
	}
}

func TestSportsMarketTypesRoundTrip(t *testing.T) {
	for _, types := range [][]string{nil, {}, {"moneyline"}} {
		original := SportsMarketTypesResponse{MarketTypes: types}
		b, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded SportsMarketTypesResponse
		if err := json.Unmarshal(b, &decoded); err != nil || len(decoded.MarketTypes) != len(types) {
			t.Fatalf("round trip %s: %+v, %v", b, decoded, err)
		}
	}
	var decoded SportsMarketTypesResponse
	if err := json.Unmarshal([]byte(`{"marketTypes":[null]}`), &decoded); err == nil {
		t.Fatal("accepted null market type")
	}
}

func TestRepeatedResponseDecoding(t *testing.T) {
	var tag Tag
	var event Event
	var market Market
	cases := []struct {
		name, first, second, invalid string
		dst                          any
		check                        func() bool
	}{
		{
			"tag", `{"id":"1","label":"old"}`, `{"id":"2"}`, `{"id":"3","label":true}`, &tag, func() bool { return tag.ID == "2" && tag.Label == nil },
		},
		{
			"event", `{"id":"1","publishedAt":"old","markets":[{"id":"1"}]}`, `{"id":"2","published_at":"new"}`, `{"id":"3","markets":[{}]}`, &event, func() bool {
				return event.ID == "2" && event.PublishedAt != nil && *event.PublishedAt == "new" && event.Markets == nil
			},
		},
		{
			"market", `{"id":"1","submittedBy":"old","twitterCardLastRefreshed":"123","clobTokenIds":"[\"1\"]"}`, `{"id":"2","submitted_by":"new"}`, `{"id":"3","clobTokenIds":"[\"-1\"]"}`, &market, func() bool {
				return market.ID == "2" && market.SubmittedBy != nil && *market.SubmittedBy == "new" && market.TwitterCardLastRefreshed == nil && market.CLOBTokenIDs == nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.first), tc.dst); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.second), tc.dst); err != nil {
				t.Fatal(err)
			}
			if !tc.check() {
				t.Fatalf("stale fields after second decode: %+v", tc.dst)
			}
			if err := json.Unmarshal([]byte(tc.invalid), tc.dst); err == nil {
				t.Fatal("invalid response accepted")
			}
			if !tc.check() {
				t.Fatalf("receiver changed after failed decode: %+v", tc.dst)
			}
		})
	}
}

func TestOptionalNullAndEmpty(t *testing.T) {
	var c Comment
	if err := json.Unmarshal([]byte(`{"id":"","userAddress":"","replyAddress":null,"profile":{"baseAddress":""}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.UserAddress != nil || c.ReplyAddress != nil || c.Profile.BaseAddress != nil {
		t.Fatalf("%+v", c)
	}
}
