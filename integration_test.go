package clob

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestPayloadCryptoRoundTrip(t *testing.T) {
	address := mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	for _, version := range []int{1, 2, 3} {
		o := Order{Version: version, Salt: 42, Maker: address, Signer: address, Taker: address,
			TokenID: new(big.Int).Lsh(big.NewInt(1), 255), MakerAmount: big.NewInt(12), TakerAmount: big.NewInt(34),
			Expiration: big.NewInt(56), Nonce: big.NewInt(78), FeeRateBPS: big.NewInt(90), Timestamp: big.NewInt(123), Side: Sell}
		o.Metadata[31], o.Builder[0] = 4, 5
		payload, err := NewOrderPayload(o)
		if err != nil {
			t.Fatal(err)
		}
		if payload.Version() != version {
			t.Fatalf("version %d: got %d", version, payload.Version())
		}
		converted, err := payload.ToOrder()
		if err != nil {
			t.Fatal(err)
		}
		before, err := OrderDigest(o, Polygon, false)
		if err != nil {
			t.Fatal(err)
		}
		after, err := OrderDigest(converted, Polygon, false)
		if err != nil || before != after || converted.Expiration.Cmp(o.Expiration) != 0 {
			t.Fatalf("version %d: digest or expiration changed: %v", version, err)
		}
		if version == 1 && (converted.Taker != address || converted.Nonce.Cmp(o.Nonce) != 0 || converted.FeeRateBPS.Cmp(o.FeeRateBPS) != 0) {
			t.Fatal("V1 fields changed")
		}
		if version != 1 && (converted.Metadata != o.Metadata || converted.Builder != o.Builder) {
			t.Fatal("V2 fields changed")
		}
	}
}

func TestPayloadRejectsInvalidValues(t *testing.T) {
	p := OrderPayload{V1: &OrderPayloadV1{}, V2: &OrderPayloadV2{}}
	if _, err := p.ToOrder(); err == nil {
		t.Fatal("accepted multiple versions")
	}
	p = OrderPayload{V2: &OrderPayloadV2{Order: OrderV2{Metadata: "0x01"}}}
	if _, err := p.ToOrder(); err == nil {
		t.Fatal("accepted invalid metadata")
	}
	p.V2.Order.Metadata = ""
	p.V2.Order.TokenID = "-1"
	if _, err := p.ToOrder(); err == nil {
		t.Fatal("accepted invalid uint256")
	}
	p.V2.Order.TokenID = "1"
	p.V2.Order.Side = 2
	if _, err := p.ToOrder(); err == nil {
		t.Fatal("accepted invalid side")
	}
}

func TestAddressAndSideWireModels(t *testing.T) {
	addr := mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	data := `{"allowances":{"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266":"100"},"balance":"1"}`
	var balance BalanceAllowanceResponse
	if err := json.Unmarshal([]byte(data), &balance); err != nil || balance.Allowances[addr] != "100" {
		t.Fatalf("%+v: %v", balance, err)
	}
	encoded, err := json.Marshal(balance)
	if err != nil || !strings.Contains(string(encoded), addr.String()) {
		t.Fatalf("%s: %v", encoded, err)
	}
	var prices PricesResponse
	if err := json.Unmarshal([]byte(`{"123":{"BUY":"0.5","SELL":"0.6"}}`), &prices); err != nil {
		t.Fatal(err)
	}
	if prices["123"][Buy] != "0.5" || prices["123"][Sell] != "0.6" {
		t.Fatal(prices)
	}
	encoded, err = json.Marshal(prices)
	if err != nil || !strings.Contains(string(encoded), `"BUY"`) || !strings.Contains(string(encoded), `"SELL"`) {
		t.Fatalf("%s: %v", encoded, err)
	}
	var market MarketResponse
	if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`,"fpmm":""}`), &market); err != nil || market.FPMM != nil {
		t.Fatalf("%+v: %v", market, err)
	}
	if err := json.Unmarshal([]byte(`{`+fixtureMarketFields+`,"fpmm":"0x0000000000000000000000000000000000000000"}`), &market); err != nil || market.FPMM == nil {
		t.Fatalf("%+v: %v", market, err)
	}
}
