package bridge

import (
	"encoding/json"
	"testing"
)

func TestBaseUnitCompatibility(t *testing.T) {
	const max = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
	cases := []struct {
		amount        string
		requestValid  bool
		responseValid bool
	}{
		{"0", true, true}, {max, true, true}, {"", false, false}, {"-1", false, false}, {"1.5", false, false}, {"1e3", false, false}, {"0x10", false, true}, {"115792089237316195423570985008687907853269984665640564039457584007913129639936", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.amount, func(t *testing.T) {
			b, err := json.Marshal(QuoteRequest{FromAmountBaseUnit: Number(tc.amount), FromTokenAddress: "sol", RecipientAddress: "btc", ToTokenAddress: testEVM})
			if (err == nil) != tc.requestValid {
				t.Fatalf("request %s: %s %v", tc.amount, b, err)
			}
			quoted, _ := json.Marshal(tc.amount)
			for _, decode := range []func([]byte) error{
				func(b []byte) error { var v QuoteResponse; return json.Unmarshal(b, &v) },
				func(b []byte) error { var v DepositTransaction; return json.Unmarshal(b, &v) },
			} {
				body := []byte(`{"estCheckoutTimeMs":0,"estFeeBreakdown":` + testFees + `,"estInputUsd":0,"estOutputUsd":0,"quoteId":"q","estToTokenBaseUnit":` + string(quoted) + `,"fromChainId":"1","fromTokenAddress":"sol","toChainId":"137","toTokenAddress":"` + testEVM + `","status":"COMPLETED","fromAmountBaseUnit":` + string(quoted) + `}`)
				if err := decode(body); (err == nil) != tc.responseValid {
					t.Fatalf("response %s: %v", body, err)
				}
			}
		})
	}
	for _, body := range []string{`{"estToTokenBaseUnit":null}`, `{"fromAmountBaseUnit":null}`} {
		var err error
		if body[2] == 'e' {
			var v QuoteResponse
			err = json.Unmarshal([]byte(body), &v)
		} else {
			var v DepositTransaction
			err = json.Unmarshal([]byte(body), &v)
		}
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
