package clob

import (
	"math/big"
	"testing"
)

func TestLimitOrderVectors(t *testing.T) {
	p := OrderParams{Version: 2, AssetID: "15871154585880608648532107628464183779895785213830018178010423617714102767076", Signer: mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"), Price: "0.5", TickSize: "0.1", OrderType: GTD, Expiration: 50000}
	for _, v := range []struct {
		side         Side
		maker, taker string
	}{{Buy, "10520000", "21040000"}, {Sell, "21040000", "10520000"}} {
		p.Side = v.side
		o, e := BuildLimitOrder(p, "21.049")
		if e != nil {
			t.Fatal(e)
		}
		if o.Order.MakerAmount.String() != v.maker || o.Order.TakerAmount.String() != v.taker || o.Order.Expiration.Uint64() != 50000 {
			t.Fatalf("%+v", o.Order)
		}
	}
	p.TickSize = "0.005"
	p.Price = "0.007"
	if _, e := BuildLimitOrder(p, "10"); e == nil {
		t.Fatal("off-grid price accepted")
	}
	p.Price = "0.005"
	if _, e := BuildLimitOrder(p, "0.001"); e == nil {
		t.Fatal("zero lot accepted")
	}
}
func TestFunderAndFees(t *testing.T) {
	p := OrderParams{Version: 2, ChainID: 137, AssetID: "1234", Signer: mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"), SignatureType: SignatureSafe, Price: "0.50", TickSize: "0.01"}
	o, err := BuildLimitOrder(p, "10")
	if err != nil || o.Order.Maker != mustAddress(t, "0xd93b25Cb943D14d0d34FBAf01fc93a0F8b5f6e47") {
		t.Fatalf("safe funder: %v %+v", err, o)
	}
	p.SignatureType = SignatureEOA
	p.MarketBuyFees = &MarketBuyFees{Balance: "100", PlatformRate: "0", BuilderRate: "0.005", Exponent: 1}
	o, err = BuildMarketOrder(p, MarketAmount{Value: "100", InUSDC: true}, nil)
	if err != nil || o.Order.MakerAmount.String() != "99502450" || o.Order.TakerAmount.String() != "199004900" {
		t.Fatalf("fee-adjusted: %v %+v", err, o)
	}
	p.MarketBuyFees.Balance = "0.0000001"
	if _, err := BuildMarketOrder(p, MarketAmount{Value: "100", InUSDC: true}, nil); err == nil {
		t.Fatal("accepted insufficient balance")
	}
}

func TestMarketBuyRespectsLimitAfterRounding(t *testing.T) {
	for _, v := range []struct{ tick, price, amount string }{
		{"0.1", "0.3", "10"},
		{"0.01", "0.51", "0.001001"},
		{"0.005", "0.005", "1.234567"},
		{"0.0001", "0.1234", "2.345678"},
	} {
		t.Run(v.price+"/"+v.amount, func(t *testing.T) {
			p := OrderParams{Version: 2, AssetID: "1", Price: v.price, TickSize: v.tick}
			o, err := BuildMarketOrder(p, MarketAmount{Value: v.amount, InUSDC: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			price, _, _ := decimal(v.price)
			amount, _, _ := decimal(v.amount)
			if new(big.Rat).SetFrac(o.Order.MakerAmount, o.Order.TakerAmount).Cmp(price) > 0 || new(big.Rat).SetFrac(o.Order.MakerAmount, big.NewInt(1000000)).Cmp(amount) > 0 {
				t.Fatalf("price or spend exceeded: %+v", o.Order)
			}
		})
	}
}

func TestUnsafeOrderInputsRejected(t *testing.T) {
	p := OrderParams{Version: 2, AssetID: "1234", Price: "0.50", TickSize: "0.01"}
	for _, version := range []int{2, 3} {
		for _, field := range []string{"taker", "nonce", "fee"} {
			q := p
			q.Version = version
			switch field {
			case "taker":
				q.Taker = mustAddress(t, "0x1111111111111111111111111111111111111111")
			case "nonce":
				q.Nonce = 1
			case "fee":
				q.FeeRateBPS = 1
			}
			if _, err := BuildLimitOrder(q, "10"); err == nil {
				t.Fatalf("accepted V%d %s", version, field)
			}
			if _, err := BuildMarketOrder(q, MarketAmount{Value: "10", InUSDC: true}, nil); err == nil {
				t.Fatalf("accepted V%d market %s", version, field)
			}
			q.Version = 1
			if _, err := BuildLimitOrder(q, "10"); err != nil {
				t.Fatalf("rejected V1 %s: %v", field, err)
			}
		}
	}
	p.Price = "0.509"
	p.Side = Sell
	if _, err := BuildMarketOrder(p, MarketAmount{Value: "10"}, nil); err == nil {
		t.Fatal("accepted off-tick sell limit")
	}
	p.Price = "0.50"
	p.MarketBuyFees = &MarketBuyFees{Balance: "1", PlatformRate: "0", BuilderRate: "0"}
	if _, err := BuildMarketOrder(p, MarketAmount{Value: "10"}, nil); err == nil {
		t.Fatal("ignored fee sizing")
	}
	for _, levels := range [][]BookLevel{
		{{"0.5", "10"}, {"0.7", "10"}},
		{{"0.7", "0"}, {"0.5", "10"}},
		{{"1.1", "10"}, {"0.5", "10"}},
	} {
		if _, err := CalculateMarketPrice(levels, Buy, MarketAmount{Value: "1"}, FAK); err == nil {
			t.Fatalf("accepted invalid book: %v", levels)
		}
	}
}

func TestMarketOrders(t *testing.T) {
	p := OrderParams{Version: 2, AssetID: "1234", Price: "0.50", TickSize: "0.01", Side: Buy}
	o, e := BuildMarketOrder(p, MarketAmount{Value: "10", InUSDC: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if o.Order.MakerAmount.String() != "10000000" || o.Order.TakerAmount.String() != "20000000" {
		t.Fatal(o.Order)
	}
	p.Side = Sell
	if _, e = BuildMarketOrder(p, MarketAmount{Value: "10", InUSDC: true}, nil); e == nil {
		t.Fatal("sell USDC accepted")
	}
	o, e = BuildMarketOrder(p, MarketAmount{Value: "21.049"}, nil)
	if e != nil || o.Order.MakerAmount.String() != "21040000" || o.Order.TakerAmount.String() != "10520000" {
		t.Fatalf("%v %+v", e, o)
	}
	levels := []BookLevel{{"0.7", "10"}, {"0.6", "5"}, {"0.5", "10"}}
	price, e := CalculateMarketPrice(levels, Buy, MarketAmount{Value: "12"}, FOK)
	if e != nil || price != "0.6" {
		t.Fatalf("%s %v", price, e)
	}
	if _, e = CalculateMarketPrice(levels, Buy, MarketAmount{Value: "100"}, FOK); e == nil {
		t.Fatal("FOK accepted insufficient liquidity")
	}
	price, e = CalculateMarketPrice(levels, Buy, MarketAmount{Value: "100"}, FAK)
	if e != nil || price != "0.7" {
		t.Fatalf("%s %v", price, e)
	}
}
