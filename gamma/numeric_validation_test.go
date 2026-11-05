package gamma

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestDuplicateResponseFields(t *testing.T) {
	for _, tc := range []struct {
		name, body, field string
		dst               any
	}{
		{"market aliases", `{"id":"1","submittedBy":"a","submitted_by":"b"}`, "submittedBy", &Market{}},
		{"market aliases reversed with null", `{"id":"1","submitted_by":null,"submittedBy":"b"}`, "submittedBy", &Market{}},
		{"event aliases", `{"id":"1","publishedAt":"a","published_at":"b"}`, "publishedAt", &Event{}},
		{"event aliases reversed", `{"id":"1","published_at":"a","publishedAt":null}`, "publishedAt", &Event{}},
		{"same field", `{"id":"1","id":"2"}`, "id", &Market{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.body), tc.dst)
			if err == nil || !strings.Contains(err.Error(), "duplicate field") || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected duplicate %s error, got %v", tc.field, err)
			}
		})
	}
}

func TestUint256Boundaries(t *testing.T) {
	max := "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	for _, tc := range []struct{ input, canonical string }{
		{"0", "0"}, {"00042", "42"}, {"0x0", "0"}, {"0xAbCd", "43981"},
		{max, max}, {"0x" + strings.Repeat("f", 64), max},
	} {
		t.Run(tc.input, func(t *testing.T) {
			n, err := parseUint256(tc.input)
			if err != nil || n.String() != tc.canonical {
				t.Fatalf("parse: %v, %v", n, err)
			}
			q, err := queryValues(MarketsRequest{CLOBTokenIDs: []Number{Number(tc.input)}})
			if err != nil || !reflect.DeepEqual(q["clob_token_ids"], []string{tc.canonical}) {
				t.Fatalf("query: %v, %v", q, err)
			}
			for _, body := range []string{
				fmt.Sprintf(`{"id":"1","denominationToken":%q}`, tc.input),
				fmt.Sprintf(`{"id":"1","clobTokenIds":[%q]}`, tc.input),
			} {
				var market Market
				if err := json.Unmarshal([]byte(body), &market); err != nil {
					t.Fatalf("decode %s: %v", body, err)
				}
			}
			var position CommentPosition
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"tokenId":%q}`, tc.input)), &position); err != nil {
				t.Fatalf("position: %v", err)
			}
		})
	}
	for _, input := range []string{"", "-1", "+1", "1.0", "1e0", " 1", "0x", "0X1", "0xgg", "1_0", "115792089237316195423570985008687907853269984665640564039457584007913129639936", "0x1" + strings.Repeat("0", 64)} {
		t.Run("invalid/"+input, func(t *testing.T) {
			if _, err := parseUint256(input); err == nil {
				t.Fatal("accepted invalid U256")
			}
			if _, err := queryValues(MarketsRequest{CLOBTokenIDs: []Number{"1", Number(input)}}); err == nil || !strings.Contains(err.Error(), "clob_token_ids[1]") {
				t.Fatalf("query error: %v", err)
			}
			for _, body := range []string{
				fmt.Sprintf(`{"id":"1","denominationToken":%q}`, input),
				fmt.Sprintf(`{"id":"1","clobTokenIds":["1",%q]}`, input),
				fmt.Sprintf(`{"tokenId":%q}`, input),
			} {
				var dst any = &Market{}
				if strings.Contains(body, "tokenId") {
					dst = &CommentPosition{}
				}
				if err := json.Unmarshal([]byte(body), dst); err == nil {
					t.Fatalf("accepted %s", body)
				}
			}
		})
	}
}

func TestUint256LongInput(t *testing.T) {
	zeros := strings.Repeat("0", 300000)
	for _, value := range []string{zeros + "42", "0x" + zeros + "2a"} {
		if n, err := parseUint256(value); err != nil || n.String() != "42" {
			t.Fatalf("leading zeroes: %v, %v", n, err)
		}
	}
	if n, err := parseUint256(zeros); err != nil || n.Sign() != 0 {
		t.Fatalf("all zeroes: %v, %v", n, err)
	}
	tooLarge := strings.Repeat("9", 300000)
	if _, err := queryValues(MarketsRequest{CLOBTokenIDs: []Number{Number(tooLarge)}}); err == nil {
		t.Fatal("accepted oversized request token")
	}
	for _, body := range []string{
		`{"id":"1","denominationToken":"` + tooLarge + `"}`,
		`{"id":"1","clobTokenIds":["` + tooLarge + `"]}`,
		`{"tokenId":"` + tooLarge + `"}`,
	} {
		var dst any = &Market{}
		if strings.Contains(body, "tokenId") {
			dst = &CommentPosition{}
		}
		if err := json.Unmarshal([]byte(body), dst); err == nil {
			t.Fatal("accepted oversized U256")
		}
	}
}

func TestNumericResponseRoundTrip(t *testing.T) {
	const max = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	const decimal = "0.123456789012345678901234567890123456789"
	body := `{"id":"1","denominationToken":` + max + `,"clobTokenIds":"[\"0x2a\",\"` + max + `\"]","volume":` + decimal + `,"outcomePrices":"[\"` + decimal + `\"]"}`
	var market Market
	if err := json.Unmarshal([]byte(body), &market); err != nil {
		t.Fatal(err)
	}
	if *market.DenominationToken != Number(max) || (*market.CLOBTokenIDs)[0] != "0x2a" || (*market.CLOBTokenIDs)[1] != Number(max) || *market.Volume != Number(decimal) || (*market.OutcomePrices)[0] != Number(decimal) {
		t.Fatalf("numeric precision lost: %+v", market)
	}
	encoded, err := json.Marshal(market)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Market
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("round trip %s: %v", encoded, err)
	}
	if !reflect.DeepEqual(market, decoded) {
		t.Fatalf("round trip changed market: %s", encoded)
	}
}

func TestResponseDecimalMagnitudeAgainstRational(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	max, ok := new(big.Rat).SetString("79228162514264337593543950335")
	if !ok {
		t.Fatal("invalid test maximum")
	}
	for i := 0; i < 5000; i++ {
		digits := make([]byte, 1+rng.Intn(40))
		for j := range digits {
			digits[j] = byte('0' + rng.Intn(10))
		}
		point := rng.Intn(len(digits) + 1)
		value := fmt.Sprintf("%s.%se%d", digits[:point], digits[point:], rng.Intn(101)-50)
		rational, ok := new(big.Rat).SetString(value)
		if !ok {
			t.Fatalf("invalid generated decimal %q", value)
		}
		want := rational.Cmp(max) <= 0
		if err := validateResponseDecimal(value); (err == nil) != want {
			t.Fatalf("%s: want valid=%t, got %v", value, want, err)
		}
	}
}

func TestResponseDecimalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"0", true}, {"-0", true}, {".123456789012345678901234567890123456789", true},
		{"1e-29", true}, {"1e-999999999999999999999", true}, {"0e999999999999999999999", true},
		{"79228162514264337593543950335", true}, {"-79228162514264337593543950335.000", true},
		{"7.9228162514264337593543950335e28", true}, {"79228162514264337593543950334.9999", true},
		{"79228162514264337593543950335.0001", false}, {"-79228162514264337593543950336", false},
		{"7.9228162514264337593543950336e28", false}, {"1e29", false}, {"1e999999999999999999999", false},
		{"NaN", false}, {"0x10", false}, {"1_0", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if err := validateResponseDecimal(tc.value); (err == nil) != tc.valid {
				t.Fatalf("validation error: %v", err)
			}
			for _, body := range []string{
				fmt.Sprintf(`{"id":"1","volume":%q}`, tc.value),
				fmt.Sprintf(`{"id":"1","outcomePrices":[%q]}`, tc.value),
				fmt.Sprintf(`{"id":"1","markets":[{"id":"2","volume":%q}]}`, tc.value),
			} {
				var dst any = &Market{}
				if strings.Contains(body, "markets") {
					dst = &Event{}
				}
				err := json.Unmarshal([]byte(body), dst)
				if (err == nil) != tc.valid {
					t.Fatalf("decode %s: %v", body, err)
				}
			}
		})
	}
}
