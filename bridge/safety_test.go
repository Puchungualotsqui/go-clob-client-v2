package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testEVM = "0x56687bf447db6ffa42ffe2204a05edaa20f55839"
const testFees = `{"appFeeLabel":"fee","appFeePercent":0,"appFeeUsd":0,"fillCostPercent":0,"fillCostUsd":0,"gasUsd":0,"maxSlippage":0,"minReceived":0,"swapImpact":0,"swapImpactUsd":0,"totalImpact":0,"totalImpactUsd":0}`

func TestInvalidWalletNeverSent(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"", "0xabc", "0x" + strings.Repeat("g", 40), " " + testEVM, testEVM + "00"} {
		t.Run(address, func(t *testing.T) {
			if _, err := c.Deposit(context.Background(), DepositRequest{Address: address}); err == nil {
				t.Fatal("accepted invalid deposit wallet")
			}
			if _, err := c.Withdraw(context.Background(), WithdrawRequest{Address: address, ToTokenAddress: "sol", RecipientAddr: "bc1qexample"}); err == nil {
				t.Fatal("accepted invalid withdraw wallet")
			}
		})
	}
	if calls != 0 {
		t.Fatalf("sent %d invalid requests", calls)
	}
}

func TestRequiredResponseFields(t *testing.T) {
	addresses := `{"evm":"` + testEVM + `","svm":"So11111111111111111111111111111111111111112","btc":"bc1qexample"}`
	tests := []struct {
		name, body string
		fields     []string
		target     func() any
	}{
		{"supported assets", `{"supportedAssets":[]}`, []string{"supportedAssets"}, func() any { return new(SupportedAssetsResponse) }},
		{"status", `{"transactions":[]}`, []string{"transactions"}, func() any { return new(StatusResponse) }},
		{"supported asset", `{"chainId":"137","chainName":"Polygon","token":{"name":"USD Coin","symbol":"USDC","address":"sol","decimals":6},"minCheckoutUsd":"1.25"}`, []string{"chainId", "chainName", "token", "minCheckoutUsd"}, func() any { return new(SupportedAsset) }},
		{"token", `{"name":"USD Coin","symbol":"USDC","address":"sol","decimals":0}`, []string{"name", "symbol", "address", "decimals"}, func() any { return new(Token) }},
		{"deposit", `{"address":` + addresses + `}`, []string{"address"}, func() any { return new(DepositResponse) }},
		{"withdraw", `{"address":` + addresses + `,"note":""}`, []string{"address", "note"}, func() any { return new(WithdrawResponse) }},
		{"addresses", addresses, []string{"evm", "svm", "btc"}, func() any { return new(DepositAddresses) }},
		{"withdraw addresses", addresses, []string{"evm", "svm", "btc"}, func() any { return new(WithdrawalAddresses) }},
		{"quote", `{"estCheckoutTimeMs":0,"estFeeBreakdown":` + testFees + `,"estInputUsd":0,"estOutputUsd":0,"estToTokenBaseUnit":"0","quoteId":""}`, []string{"estCheckoutTimeMs", "estFeeBreakdown", "estInputUsd", "estOutputUsd", "estToTokenBaseUnit", "quoteId"}, func() any { return new(QuoteResponse) }},
		{"fees", testFees, []string{"appFeeLabel", "appFeePercent", "appFeeUsd", "fillCostPercent", "fillCostUsd", "gasUsd", "maxSlippage", "minReceived", "swapImpact", "swapImpactUsd", "totalImpact", "totalImpactUsd"}, func() any { return new(EstimatedFeeBreakdown) }},
		{"transaction", `{"fromChainId":"1","fromTokenAddress":"So11111111111111111111111111111111111111112","fromAmountBaseUnit":"0","toChainId":"137","toTokenAddress":"` + testEVM + `","status":"COMPLETED"}`, []string{"fromChainId", "fromTokenAddress", "fromAmountBaseUnit", "toChainId", "toTokenAddress", "status"}, func() any { return new(DepositTransaction) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tt.body), tt.target()); err != nil {
				t.Fatalf("valid response: %v", err)
			}
			for _, field := range tt.fields {
				for _, null := range []bool{false, true} {
					var obj map[string]json.RawMessage
					if err := json.Unmarshal([]byte(tt.body), &obj); err != nil {
						t.Fatal(err)
					}
					if null {
						obj[field] = json.RawMessage("null")
					} else {
						delete(obj, field)
					}
					b, _ := json.Marshal(obj)
					if err := json.Unmarshal(b, tt.target()); err == nil {
						t.Errorf("accepted missing/null %s: %s", field, b)
					}
				}
			}
		})
	}
}

func TestNestedCollectionsAndOptionalFields(t *testing.T) {
	for _, body := range []string{
		`{"supportedAssets":[null]}`,
		`{"supportedAssets":[{}]}`,
		`{"supportedAssets":[{"chainId":"137","chainName":"Polygon","token":{"name":"USD Coin","symbol":"USDC","address":"sol"},"minCheckoutUsd":1}]}`,
	} {
		var v SupportedAssetsResponse
		if err := json.Unmarshal([]byte(body), &v); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, body := range []string{`{"transactions":[null]}`, `{"transactions":[{}]}`} {
		var v StatusResponse
		if err := json.Unmarshal([]byte(body), &v); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	for _, body := range []string{`{"supportedAssets":[]}`, `{"supportedAssets":[],"note":null}`} {
		var v SupportedAssetsResponse
		if err := json.Unmarshal([]byte(body), &v); err != nil || v.SupportedAssets == nil || v.Note != nil {
			t.Errorf("%s: %+v %v", body, v, err)
		}
	}
	var v StatusResponse
	if err := json.Unmarshal([]byte(`{"transactions":[]}`), &v); err != nil || v.Transactions == nil {
		t.Fatalf("empty status: %+v %v", v, err)
	}
}

func TestEmptyRequestAddressesNeverSent(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", " \t\n"} {
		for _, field := range []string{"fromTokenAddress", "toTokenAddress", "recipientAddress"} {
			r := QuoteRequest{FromAmountBaseUnit: "1", FromTokenAddress: "So11111111111111111111111111111111111111112", ToTokenAddress: "btc", RecipientAddress: "bc1qexample"}
			switch field {
			case "fromTokenAddress":
				r.FromTokenAddress = invalid
			case "toTokenAddress":
				r.ToTokenAddress = invalid
			case "recipientAddress":
				r.RecipientAddress = invalid
			}
			if _, err := c.Quote(context.Background(), r); err == nil || !strings.Contains(err.Error(), field) {
				t.Errorf("quote %s: %v", field, err)
			}
		}
		for _, field := range []string{"toTokenAddress", "recipientAddr"} {
			r := WithdrawRequest{Address: testEVM, ToTokenAddress: "So11111111111111111111111111111111111111112", RecipientAddr: "bc1qexample"}
			if field == "toTokenAddress" {
				r.ToTokenAddress = invalid
			} else {
				r.RecipientAddr = invalid
			}
			if _, err := c.Withdraw(context.Background(), r); err == nil || !strings.Contains(err.Error(), field) {
				t.Errorf("withdraw %s: %v", field, err)
			}
		}
	}
	if calls != 0 {
		t.Fatalf("sent %d invalid requests", calls)
	}
}

func TestReturnedEVMAddresses(t *testing.T) {
	for _, address := range []string{"", "0x1234", "0x" + strings.Repeat("z", 40)} {
		for _, body := range []string{
			`{"address":{"evm":"` + address + `","svm":"sol","btc":"btc"}}`,
			`{"address":{"evm":"` + address + `","svm":"sol","btc":"btc"},"note":"ok"}`,
		} {
			var v any = new(DepositResponse)
			if strings.Contains(body, `"note"`) {
				v = new(WithdrawResponse)
			}
			if err := json.Unmarshal([]byte(body), v); err == nil {
				t.Errorf("accepted %s", body)
			}
		}
		body := `{"fromChainId":"1","fromTokenAddress":"btc","fromAmountBaseUnit":"1","toChainId":"137","toTokenAddress":"` + address + `","status":"COMPLETED"}`
		var tx DepositTransaction
		if err := json.Unmarshal([]byte(body), &tx); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestResponseFieldConflicts(t *testing.T) {
	other := "0x1111111111111111111111111111111111111111"
	addresses := `{"evm":"` + testEVM + `","svm":"sol","btc":"btc"}`
	for _, body := range []string{
		`{"address":` + addresses + `,"Address":{"evm":"` + other + `","svm":"bad","btc":"bad"}}`,
		`{"address":{"evm":"` + testEVM + `","EVM":"` + other + `","svm":"sol","btc":"btc"}}`,
		`{"Address":{"evm":"` + other + `","svm":"bad","btc":"bad"},"address":` + addresses + `}`,
	} {
		var v DepositResponse
		if err := json.Unmarshal([]byte(body), &v); err != nil || v.Address.EVM != testEVM || v.Address.SVM != "sol" {
			t.Errorf("case variant changed deposit: %+v, %v", v, err)
		}
	}
	var assets SupportedAssetsResponse
	body := `{"supportedAssets":[{"chainId":"137","chainName":"Polygon","token":{"name":"USDC","symbol":"USDC","address":"good","Address":"bad","decimals":6,"Decimals":18},"minCheckoutUsd":"1","MinCheckoutUsd":"999"}],"SupportedAssets":null}`
	if err := json.Unmarshal([]byte(body), &assets); err != nil || len(assets.SupportedAssets) != 1 || assets.SupportedAssets[0].Token.Address != "good" || assets.SupportedAssets[0].Token.Decimals != 6 || assets.SupportedAssets[0].MinCheckoutUSD != "1" {
		t.Fatalf("case variant changed asset: %+v, %v", assets, err)
	}
	var quote QuoteResponse
	body = `{"estCheckoutTimeMs":1,"estFeeBreakdown":` + strings.Replace(testFees, `"minReceived":0`, `"minReceived":1,"MinReceived":999`, 1) + `,"estInputUsd":1,"EstInputUsd":999,"estOutputUsd":1,"estToTokenBaseUnit":"1","EstToTokenBaseUnit":"999","quoteId":"good","QuoteId":"bad"}`
	if err := json.Unmarshal([]byte(body), &quote); err != nil || quote.EstInputUSD != 1 || quote.EstToTokenBaseUnit != "1" || quote.EstFeeBreakdown.MinReceived != 1 || quote.QuoteID != "good" {
		t.Fatalf("case variant changed quote: %+v, %v", quote, err)
	}
	var status StatusResponse
	body = `{"transactions":[{"fromChainId":"1","fromTokenAddress":"good","fromAmountBaseUnit":"1","FromAmountBaseUnit":"999","toChainId":"137","toTokenAddress":"` + testEVM + `","status":"PROCESSING","Status":"COMPLETED"}]}`
	if err := json.Unmarshal([]byte(body), &status); err != nil || len(status.Transactions) != 1 || status.Transactions[0].Status != Processing || status.Transactions[0].FromAmountBaseUnit != "1" {
		t.Fatalf("case variant changed transaction: %+v, %v", status, err)
	}
	for _, tc := range []struct {
		body string
		dst  any
	}{
		{`{"address":` + addresses + `,"address":` + addresses + `}`, new(DepositResponse)},
		{`{"address":{"evm":"` + testEVM + `","evm":"` + other + `","svm":"sol","btc":"btc"}}`, new(DepositResponse)},
		{`{"name":"USDC","symbol":"USDC","address":"good","decimals":6,"decimals":18}`, new(Token)},
		{`{"supportedAssets":[],"note":null,"note":"x"}`, new(SupportedAssetsResponse)},
		{`{"Address":` + addresses + `}`, new(DepositResponse)},
	} {
		if err := json.Unmarshal([]byte(tc.body), tc.dst); err == nil {
			t.Errorf("accepted duplicate or missing exact field: %s", tc.body)
		}
	}
}

func TestNonEVMRequestFields(t *testing.T) {
	req := WithdrawRequest{Address: testEVM, ToChainID: 1151111081099710, ToTokenAddress: "So11111111111111111111111111111111111111112", RecipientAddr: "bc1qexample"}
	b, err := json.Marshal(req)
	if err != nil || !strings.Contains(string(b), req.RecipientAddr) || !strings.Contains(string(b), req.ToTokenAddress) {
		t.Fatalf("withdraw: %s %v", b, err)
	}
	q := QuoteRequest{FromAmountBaseUnit: "1", FromTokenAddress: req.ToTokenAddress, RecipientAddress: req.RecipientAddr, ToTokenAddress: "btc"}
	if _, err := json.Marshal(q); err != nil {
		t.Fatal(err)
	}
	var dep DepositResponse
	if err := json.Unmarshal([]byte(`{"address":{"evm":"`+testEVM+`","svm":"sol","btc":"btc"},"note":null}`), &dep); err != nil || dep.Note != nil {
		t.Fatalf("optional note: %+v %v", dep, err)
	}
}
