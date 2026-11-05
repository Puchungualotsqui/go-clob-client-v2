package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaseUnitsRequireJSONString(t *testing.T) {
	for _, value := range []string{`1`, `1.0`, `null`, `true`, `"-1"`, `"1e3"`} {
		quote := `{"estCheckoutTimeMs":0,"estFeeBreakdown":` + testFees + `,"estInputUsd":0,"estOutputUsd":0,"estToTokenBaseUnit":` + value + `,"quoteId":"q"}`
		tx := `{"fromChainId":"1151111081099710","fromTokenAddress":"So11111111111111111111111111111111111111112","fromAmountBaseUnit":` + value + `,"toChainId":"137","toTokenAddress":"` + testEVM + `","status":"COMPLETED"}`
		for _, tc := range []struct {
			body string
			dst  any
		}{{quote, new(QuoteResponse)}, {tx, new(DepositTransaction)}} {
			if err := json.Unmarshal([]byte(tc.body), tc.dst); err == nil {
				t.Errorf("accepted base unit %s in %T", value, tc.dst)
			}
		}
	}
}

func TestResponseU256FromStr(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"0Xf_f", "255"}, {"0o17", "15"}, {"0B101_0", "10"},
		{"00_123", "123"},
	} {
		quote := `{"estCheckoutTimeMs":0,"estFeeBreakdown":` + testFees + `,"estInputUsd":0,"estOutputUsd":0,"estToTokenBaseUnit":"` + tc.input + `","quoteId":"q"}`
		tx := `{"fromChainId":"1","fromTokenAddress":"sol","fromAmountBaseUnit":"` + tc.input + `","toChainId":"137","toTokenAddress":"` + testEVM + `","status":"COMPLETED"}`
		var q QuoteResponse
		var d DepositTransaction
		if err := json.Unmarshal([]byte(quote), &q); err != nil || string(q.EstToTokenBaseUnit) != tc.want {
			t.Errorf("quote %q: %q, %v", tc.input, q.EstToTokenBaseUnit, err)
		}
		if err := json.Unmarshal([]byte(tx), &d); err != nil || string(d.FromAmountBaseUnit) != tc.want {
			t.Errorf("transaction %q: %q, %v", tc.input, d.FromAmountBaseUnit, err)
		}
	}
	for _, input := range []string{"", "_", "0x", "0b2", "0o8", "0xg", "+1", " 1", "0x1" + strings.Repeat("0", 64)} {
		if _, err := parseResponseBaseUnit(Number(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestChainIDDisplayFromStr(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  uint64
		valid bool
	}{
		{`"+00137"`, 137, true}, {`"0000"`, 0, true}, {`"18446744073709551615"`, ^uint64(0), true},
		{`"18446744073709551616"`, 0, false}, {`"-0"`, 0, false}, {`" 137"`, 0, false}, {`137`, 0, false},
	} {
		asset := `{"chainId":` + tc.input + `,"chainName":"Polygon","token":{"name":"x","symbol":"x","address":"x","decimals":6},"minCheckoutUsd":1}`
		tx := `{"fromChainId":` + tc.input + `,"fromTokenAddress":"sol","fromAmountBaseUnit":"1","toChainId":` + tc.input + `,"toTokenAddress":"` + testEVM + `","status":"PROCESSING"}`
		var a SupportedAsset
		var d DepositTransaction
		errA, errD := json.Unmarshal([]byte(asset), &a), json.Unmarshal([]byte(tx), &d)
		if (errA == nil) != tc.valid || (errD == nil) != tc.valid || tc.valid && (a.ChainID != tc.want || d.FromChainID != tc.want || d.ToChainID != tc.want) {
			t.Errorf("%s: asset=%+v %v transaction=%+v %v", tc.input, a, errA, d, errD)
		}
	}
}

func TestQuotedIntRequiresCompleteInteger(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`"+12"`, true}, {`-2147483648`, true}, {`2147483647`, true},
		{`"12junk"`, false}, {`"12 34"`, false}, {`" 12"`, false}, {`"1.0"`, false},
		{`null`, false}, {`2147483648`, false},
	} {
		got := QuotedInt(42)
		err := json.Unmarshal([]byte(tc.input), &got)
		if (err == nil) != tc.valid || err != nil && got != 42 {
			t.Errorf("%s: %v %v", tc.input, got, err)
		}
	}
}

func TestSupportedAssetDecimalValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{`"1.25"`, true}, {`1.234567890123456789`, true}, {`0`, true},
		{`"not a decimal"`, false}, {`"1/2"`, false}, {`""`, false}, {`true`, false}, {`null`, false},
	} {
		body := `{"chainId":"1151111081099710","chainName":"Solana","token":{"name":"Solana","symbol":"SOL","address":"So11111111111111111111111111111111111111112","decimals":9},"minCheckoutUsd":` + tc.value + `}`
		var v SupportedAsset
		err := json.Unmarshal([]byte(body), &v)
		if (err == nil) != tc.valid {
			t.Errorf("%s: %v", tc.value, err)
		}
	}
}

func TestStatusEnum(t *testing.T) {
	for _, status := range []string{"DEPOSIT_DETECTED", "PROCESSING", "ORIGIN_TX_CONFIRMED", "SUBMITTED", "COMPLETED", "FAILED"} {
		var got DepositTransactionStatus
		if err := json.Unmarshal([]byte(`"`+status+`"`), &got); err != nil || string(got) != status {
			t.Errorf("%s: %s %v", status, got, err)
		}
	}
	for _, value := range []string{`"FUTURE_STATUS"`, `"completed"`, `null`, `1`} {
		var got DepositTransactionStatus
		if err := json.Unmarshal([]byte(value), &got); err == nil {
			t.Errorf("accepted %s", value)
		}
	}
}

func TestReusedResponseIsReplaced(t *testing.T) {
	note := "old"
	v := SupportedAssetsResponse{Note: &note}
	if err := json.Unmarshal([]byte(`{"supportedAssets":[]}`), &v); err != nil || v.Note != nil || v.SupportedAssets == nil {
		t.Fatalf("response not replaced: %+v %v", v, err)
	}
	if err := json.Unmarshal([]byte(`{"supportedAssets":null}`), &v); err == nil || v.SupportedAssets == nil {
		t.Fatalf("invalid response changed value: %+v %v", v, err)
	}
}

func TestResponseRejectsTrailingJSON(t *testing.T) {
	for _, suffix := range []string{` {}`, ` null`, ` garbage`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"supportedAssets":[]}`+suffix)
		}))
		c, err := NewClient(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.SupportedAssets(context.Background())
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "after top-level value") {
			t.Errorf("suffix %q: %v", suffix, err)
		}
	}
}
