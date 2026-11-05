package clob

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatedOrderIdentity(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tick-size":
			fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
		case "/neg-risk":
			fmt.Fprint(w, `{"neg_risk":false}`)
		default:
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewLocalSigner("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "authenticated-owner"}
	deposit := Address{19}
	for _, typ := range []SignatureType{SignatureEOA, SignatureProxy, SignatureSafe, SignaturePoly1271} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			opts := AuthenticationOptions{Credentials: &creds, SignatureType: typ}
			if typ == SignaturePoly1271 {
				opts.Funder = &deposit
			}
			a, err := c.Authenticate(ctx, signer, Polygon, opts)
			if err != nil {
				t.Fatal(err)
			}
			maker := signer.Address()
			if typ == SignatureProxy || typ == SignatureSafe {
				maker, err = DeriveWallet(signer.Address(), Polygon, typ)
				if err != nil {
					t.Fatal(err)
				}
			} else if typ == SignaturePoly1271 {
				maker = deposit
			}
			p := OrderParams{Version: 2, AssetID: "123", Price: "0.5"}
			o, err := a.BuildLimitOrder(ctx, p, "10")
			if err != nil {
				t.Fatal(err)
			}
			expectedSigner := signer.Address()
			if typ == SignaturePoly1271 {
				expectedSigner = maker
			}
			if o.Order.Maker != maker || o.Order.Signer != expectedSigner || o.Order.SignatureType != typ {
				t.Fatalf("wrong identity: %+v", o.Order)
			}
			signed, err := a.SignOrder(ctx, signer, o)
			if err != nil {
				t.Fatal(err)
			}
			want, err := SignOrder(signer, o, Polygon, false, creds.Key)
			if err != nil || signed.Signature != want.Signature || signed.Owner != creds.Key {
				t.Fatalf("wrong signature or owner: %v", err)
			}
			market, err := a.BuildAuthenticatedMarketOrder(ctx, p, MarketAmount{Value: "2"})
			if err != nil || market.Order.Maker != maker || market.Order.SignatureType != typ {
				t.Fatalf("market identity: %v", err)
			}
			if _, err := a.SignOrder(ctx, other, o); err == nil {
				t.Error("accepted wrong signer")
			}
			for _, mutate := range []func(*Order){
				func(o *Order) { o.Maker = Address{99} },
				func(o *Order) { o.Signer = Address{99} },
				func(o *Order) { o.SignatureType = SignatureType((int(typ) + 1) % 4) },
			} {
				bad := o
				mutate(&bad.Order)
				if _, err := a.SignOrder(ctx, signer, bad); err == nil {
					t.Error("accepted mismatched order")
				}
			}
			for _, mutate := range []func(*OrderParams){
				func(p *OrderParams) { p.Signer = other.Address() },
				func(p *OrderParams) { p.ChainID = Amoy },
				func(p *OrderParams) { f := Address{99}; p.Funder = &f },
				func(p *OrderParams) { p.SignatureType = SignatureType(99) },
			} {
				bad := p
				mutate(&bad)
				if _, err := a.BuildLimitOrder(ctx, bad, "10"); err == nil {
					t.Error("accepted mismatched params")
				}
			}
		})
	}
}

func TestAuthenticatedOrderSubmissionIdentity(t *testing.T) {
	ctx := context.Background()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/order":
			fmt.Fprint(w, `{"makingAmount":"0","takingAmount":"0","orderID":"id","status":"LIVE","success":true}`)
		case "/orders":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	hc := server.Client()
	originalTransport := hc.Transport
	c, err := NewClient(server.URL, DefaultConfig(), hc)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "owner", Secret: "c2VjcmV0"}
	funder := Address{42}
	for _, typ := range []SignatureType{SignatureEOA, SignatureProxy, SignatureSafe, SignaturePoly1271} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			opts := AuthenticationOptions{Credentials: &creds, SignatureType: typ}
			if typ == SignaturePoly1271 {
				opts.Funder = &funder
			}
			a, err := c.Authenticate(ctx, signer, Polygon, opts)
			if err != nil {
				t.Fatal(err)
			}
			if hc.Transport != originalTransport || a.httpClient.Transport != originalTransport {
				t.Fatal("authentication replaced provided transport")
			}
			p, err := a.bindOrderParams(OrderParams{Version: 2, AssetID: "1", Price: "0.5", TickSize: "0.01"})
			if err != nil {
				t.Fatal(err)
			}
			o, err := BuildLimitOrder(p, "2")
			if err != nil {
				t.Fatal(err)
			}
			signed, err := SignOrder(signer, o, Polygon, false, creds.Key)
			if err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*SignedOrder){
				func(o *SignedOrder) { o.Order.Maker = Address{99} },
				func(o *SignedOrder) { o.Order.Signer = Address{99} },
				func(o *SignedOrder) { o.Order.SignatureType = SignatureType((int(typ) + 1) % 4) },
				func(o *SignedOrder) { o.Owner = "wrong" },
			} {
				bad := signed
				mutate(&bad)
				before := requests
				if _, err := a.PostOrder(ctx, bad); err == nil {
					t.Error("accepted mismatched order")
				}
				if _, err := a.PostOrders(ctx, []SignedOrder{signed, bad}); err == nil {
					t.Error("accepted mismatched batch")
				}
				if requests != before {
					t.Fatal("invalid order reached network")
				}
			}
			before := requests
			if _, err := a.PostOrder(ctx, signed); err != nil {
				t.Fatal(err)
			}
			if _, err := a.PostOrders(ctx, []SignedOrder{signed}); err != nil {
				t.Fatal(err)
			}
			if requests != before+2 {
				t.Fatal("valid orders were not submitted")
			}
		})
	}
	standalone, err := NewAuthenticatedClient(server.URL, Address{99}, creds, AuthenticatedClientOptions{HTTPClient: hc})
	if err != nil {
		t.Fatal(err)
	}
	p := OrderParams{Version: 2, AssetID: "1", Price: "0.5", TickSize: "0.01", Signer: signer.Address()}
	o, err := BuildLimitOrder(p, "2")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignOrder(signer, o, Polygon, false, "different-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := standalone.PostOrder(ctx, signed); err != nil {
		t.Fatalf("standalone post: %v", err)
	}
	if _, err := standalone.PostOrders(ctx, []SignedOrder{signed}); err != nil {
		t.Fatalf("standalone batch: %v", err)
	}
}

func TestAuthenticatedOrderConfigValidation(t *testing.T) {
	c, err := NewClient("http://example.com", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	signer := &vectorSigner{address: Address{1}}
	creds := Credentials{Key: "key"}
	plainHTTP := &http.Client{}
	plainClient, err := NewClient("http://example.com", DefaultConfig(), plainHTTP)
	if err != nil {
		t.Fatal(err)
	}
	plainAuth, err := plainClient.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil || plainHTTP.Transport != nil || plainAuth.httpClient.Transport != nil {
		t.Fatalf("nil transport was changed: %v", err)
	}
	zero, funder := Address{}, Address{2}
	for _, o := range []AuthenticationOptions{
		{Credentials: &creds, Funder: &funder},
		{Credentials: &creds, SignatureType: SignaturePoly1271},
		{Credentials: &creds, SignatureType: SignaturePoly1271, Funder: &zero},
		{Credentials: &creds, SignatureType: SignatureProxy, Funder: &zero},
		{Credentials: &creds, SignatureType: SignatureSafe, Funder: &zero},
	} {
		if _, err := c.Authenticate(context.Background(), signer, Polygon, o); err == nil {
			t.Errorf("accepted config %+v", o)
		}
	}
	if _, err := c.Authenticate(context.Background(), signer, Amoy, AuthenticationOptions{Credentials: &creds, SignatureType: SignatureProxy}); err == nil {
		t.Fatal("accepted unsupported Amoy proxy derivation")
	}
	for _, typ := range []SignatureType{SignatureProxy, SignatureSafe, SignaturePoly1271} {
		a, err := c.Authenticate(context.Background(), signer, Amoy, AuthenticationOptions{Credentials: &creds, SignatureType: typ, Funder: &funder})
		if err != nil {
			t.Fatal(err)
		}
		funder = Address{3}
		b, err := a.orderBinding()
		if err != nil || b.funder != (Address{2}) || b.chainID != Amoy {
			t.Fatal("binding not copied")
		}
		funder = Address{2}
	}
	direct, err := NewAuthenticatedClient("http://example.com", signer.Address(), creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := direct.BuildLimitOrder(context.Background(), OrderParams{}, "1"); err == nil {
		t.Fatal("direct client accepted without order binding")
	}
}
