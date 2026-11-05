package clob

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"testing"
)

type vectorSigner struct {
	address   Address
	signature []byte
	digest    Hash
}

func (s *vectorSigner) Address() Address                { return s.address }
func (s *vectorSigner) SignHash(h Hash) ([]byte, error) { s.digest = h; return s.signature, nil }
func mustAddress(t *testing.T, s string) Address {
	t.Helper()
	a, e := ParseAddress(s)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestAuthenticationVectors(t *testing.T) {
	addr := mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	signature := "f62319a987514da40e57e2f4d7529f7bac38f0355bd88bb5adbb3768d80de6c1682518e0af677d5260366425f4361e7b70c25ae232aff0ab2331e2b164a1aedc1b"
	local, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil || local.Address() != addr {
		t.Fatalf("local signer: %v", err)
	}
	localHeaders, err := L1Headers(local, 80002, 10000000, 23)
	if err != nil || localHeaders.Get("POLY_SIGNATURE") != "0x"+signature {
		t.Fatalf("local L1: %v %v", localHeaders, err)
	}
	signer := &vectorSigner{address: addr, signature: mustHex(t, signature)}
	h, err := L1Headers(signer, 80002, 10000000, 23)
	if err != nil {
		t.Fatal(err)
	}

	// Digest independently verified against the upstream secp256k1 signature.
	if fmt.Sprintf("%x", signer.digest) != "8f442df8073c9cf2e36d7b20ca9d64d1a9352e982cc82a293b7de2df57920610" {
		t.Fatalf("L1 digest %x", signer.digest)
	}
	if h.Get("POLY_SIGNATURE") != "0x"+signature || h.Get("POLY_ADDRESS") != addr.String() || h.Get("POLY_NONCE") != "23" {
		t.Fatal(h)
	}
	u, _ := url.Parse("http://localhost/?ignored=1")
	creds := Credentials{Key: "00000000-0000-0000-0000-000000000000", Secret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", Passphrase: "secret passphrase"}
	h, err = L2Headers(addr, creds, 1, "GET", u, nil)
	if err != nil {
		t.Fatal(err)
	}
	if h.Get("POLY_SIGNATURE") != "eHaylCwqRSOa2LFD77Nt_SaTpbsxzN8eTEI3LryhEj4=" || h.Get("POLY_ADDRESS") != "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
		t.Fatal(h)
	}
	u, _ = url.Parse("http://localhost/orders")
	msg, _ := L2Message(1000000, "test-sign", u, []byte(`{"hash":"0x123"}`))
	sig, err := L2Signature(creds.Secret, msg)
	if err != nil || sig != "4gJVbox-R6XlDK4nlaicig0_ANVL1qdcahiL8CXfXLM=" {
		t.Fatalf("%s %s %v", msg, sig, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", creds), creds.Secret) || strings.Contains(fmt.Sprint(creds), creds.Passphrase) {
		t.Fatal("credentials leaked")
	}
}
func TestOrderVersionDomainsAndExpiration(t *testing.T) {
	wallet := mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	o := Order{Version: 1, Maker: wallet, Signer: wallet, TokenID: big.NewInt(1234), MakerAmount: big.NewInt(5), TakerAmount: big.NewInt(10), Expiration: big.NewInt(0), Nonce: big.NewInt(0), FeeRateBPS: big.NewInt(0), Timestamp: big.NewInt(123)}
	v1, err := OrderDigest(o, 137, false)
	if err != nil {
		t.Fatal(err)
	}
	o.Expiration = big.NewInt(1)
	v1Exp, err := OrderDigest(o, 137, false)
	if err != nil || v1 == v1Exp {
		t.Fatal("V1 must sign expiration")
	}
	neg, err := OrderDigest(o, 137, true)
	if err != nil || neg == v1Exp {
		t.Fatal("V1 neg-risk must use a different exchange")
	}
	o.Version = 2
	v2, err := OrderDigest(o, 137, false)
	if err != nil {
		t.Fatal(err)
	}
	o.Expiration = big.NewInt(2)
	v2Exp, err := OrderDigest(o, 137, false)
	if err != nil || v2 != v2Exp || v1 == v2 {
		t.Fatal("V2 expiration/domain mismatch")
	}
	o.Version = 3
	v3, err := OrderDigest(o, 137, false)
	if err != nil || v3 == v2 {
		t.Fatal("V3 domain mismatch")
	}
	v3Neg, err := OrderDigest(o, 137, true)
	if err != nil || v3 != v3Neg {
		t.Fatal("V3 must ignore neg-risk")
	}
}

func TestSignOrderValidationAndSnapshot(t *testing.T) {
	wallet := mustAddress(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
	other := mustAddress(t, "0x1111111111111111111111111111111111111111")
	signer := &vectorSigner{address: wallet, signature: make([]byte, 65)}
	post, deferExec := true, true
	order := SignableOrder{Order: Order{Version: 2, Maker: wallet, Signer: wallet, TokenID: big.NewInt(1), MakerAmount: big.NewInt(2), TakerAmount: big.NewInt(3), Expiration: big.NewInt(42), Timestamp: big.NewInt(4)}, OrderType: GTD, PostOnly: &post, DeferExec: &deferExec}
	signed, err := SignOrder(signer, order, 137, false, "owner")
	if err != nil {
		t.Fatal(err)
	}
	order.Order.MakerAmount.SetInt64(999)
	order.Order.Expiration.SetInt64(999)
	post, deferExec = false, false
	if signed.Order.MakerAmount.Int64() != 2 || signed.Order.Expiration.Int64() != 42 || !*signed.PostOnly || !*signed.DeferExec {
		t.Fatal("signed order aliases mutable input")
	}
	for _, change := range []struct {
		name string
		edit func(*Order)
	}{
		{"signer mismatch", func(o *Order) { o.Signer = other }},
		{"EOA maker mismatch", func(o *Order) { o.Maker = other }},
		{"V2 private taker", func(o *Order) { o.Taker = other }},
		{"V2 nonce", func(o *Order) { o.Nonce = big.NewInt(1) }},
		{"V2 fee", func(o *Order) { o.FeeRateBPS = big.NewInt(1) }},
		{"unsafe salt", func(o *Order) { o.Salt = 1 << 53 }},
		{"missing expiration", func(o *Order) { o.Expiration = nil }},
	} {
		t.Run(change.name, func(t *testing.T) {
			bad := order
			change.edit(&bad.Order)
			if _, err := SignOrder(signer, bad, 137, false, "owner"); err == nil {
				t.Fatal("accepted invalid order")
			}
		})
	}
}

func TestWalletVectors(t *testing.T) {
	owner := mustAddress(t, "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	for _, v := range []struct {
		kind  SignatureType
		chain uint64
		want  string
	}{{SignatureSafe, 137, "0xd93b25Cb943D14d0d34FBAf01fc93a0F8b5f6e47"}, {SignatureSafe, 80002, "0xd93b25Cb943D14d0d34FBAf01fc93a0F8b5f6e47"}, {SignatureProxy, 137, "0x365f0cA36ae1F641E02Fe3b7743673DA42A13a70"}} {
		got, e := DeriveWallet(owner, v.chain, v.kind)
		if e != nil || got != mustAddress(t, v.want) {
			t.Fatalf("%v %s", e, got.Checksum())
		}
	}
	if _, e := DeriveWallet(owner, 80002, SignatureProxy); e == nil {
		t.Fatal("expected unsupported proxy")
	}
}
func TestPoly1271Vector(t *testing.T) {
	wallet := mustAddress(t, "0x1111111111111111111111111111111111111111")
	expected := "a3a093c83b6c20c83355c16ce94c92e6e9fcbdeb840618cc74f6c57a42ad145b" + "2b98db73d2c73cbf1f2b6af288566ae81960ddbc3a13921027358a8bff3be6ff1c" + "a440cbd865bc0c6243d7a8df9a8bf48a8827b0a4abbb61c30e96d305423af148" + "d23d42d3ad94e65d78258cecaf8dcbaddac0f73dc085040f2c12bb595dd83804" + "4f726465722875696e743235362073616c742c61646472657373206d616b65722c" + "61646472657373207369676e65722c75696e7432353620746f6b656e49642c75" + "696e74323536206d616b6572416d6f756e742c75696e743235362074616b6572" + "416d6f756e742c75696e743820736964652c75696e7438207369676e61747572" + "65547970652c75696e743235362074696d657374616d702c6279746573333220" + "6d657461646174612c62797465733332206275696c6465722900ba"
	signer := &vectorSigner{signature: mustHex(t, expected[:130])}
	o := Order{Version: 2, Salt: 479249096354, Maker: wallet, Signer: wallet, TokenID: big.NewInt(1234), MakerAmount: big.NewInt(100000000), TakerAmount: big.NewInt(50000000), Expiration: big.NewInt(0), Timestamp: big.NewInt(1710000000000), SignatureType: SignaturePoly1271}
	signed, err := SignOrder(signer, SignableOrder{Order: o, OrderType: GTC}, 80002, false, "owner")
	if err != nil || signed.Signature != "0x"+expected {
		t.Fatalf("%v\n%s", err, signed.Signature)
	}
	// Digest independently verified against the upstream secp256k1 signature.
	if fmt.Sprintf("%x", signer.digest) != "c31d535831b64decf4687570628e5765c218fc3c4a7b582c7bd3105525f76a8c" {
		t.Fatalf("Poly1271 digest %x", signer.digest)
	}
	local, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	localSigned, err := SignOrder(local, SignableOrder{Order: o, OrderType: GTC}, 80002, false, "owner")
	if err != nil || localSigned.Signature != "0x"+expected {
		t.Fatalf("local Poly1271: %v %s", err, localSigned.Signature)
	}
	b, err := json.Marshal(signed)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err = json.Unmarshal(b, &parsed); err != nil {
		t.Fatal(err)
	}
	body := parsed["order"].(map[string]any)
	if body["tokenId"] != "1234" || body["side"] != "BUY" || body["timestamp"] != "1710000000000" {
		t.Fatal(string(b))
	}
}
