package clob

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"testing"
)

type signingVectors struct {
	Schema     int    `json:"schema"`
	PrivateKey string `json:"public_private_key"`
	HMAC       []struct {
		Name, Secret, Method, Path, Body, Message, Signature string
		Timestamp                                            int64
	} `json:"hmac"`
	Orders []struct {
		Name          string `json:"name"`
		Version       int    `json:"version"`
		ChainID       uint64 `json:"chain_id"`
		NegRisk       bool   `json:"neg_risk"`
		Exchange      string `json:"exchange"`
		Salt          uint64 `json:"salt"`
		Maker         string `json:"maker"`
		Signer        string `json:"signer"`
		TokenID       string `json:"token_id"`
		MakerAmount   string `json:"maker_amount"`
		TakerAmount   string `json:"taker_amount"`
		Side          uint8  `json:"side"`
		SignatureType uint8  `json:"signature_type"`
		Timestamp     string `json:"timestamp"`
		Expiration    string `json:"expiration"`
		Metadata      string `json:"metadata"`
		Builder       string `json:"builder"`
		Digest        string `json:"digest"`
		Signature     string `json:"signature"`
	} `json:"orders"`
}

func TestSharedSigningConformance(t *testing.T) {
	raw, err := os.ReadFile("conformance/signing/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors signingVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.Schema != 1 {
		t.Fatalf("unsupported fixture schema %d", vectors.Schema)
	}
	if len(vectors.HMAC) == 0 || len(vectors.Orders) == 0 {
		t.Fatal("empty vectors")
	}
	for _, v := range vectors.HMAC {
		t.Run("hmac/"+v.Name, func(t *testing.T) {
			u, err := url.Parse("https://example.invalid" + v.Path + "?ignored=yes")
			if err != nil {
				t.Fatal(err)
			}
			msg, err := L2Message(v.Timestamp, v.Method, u, []byte(v.Body))
			if err != nil || msg != v.Message {
				t.Fatalf("message %q: %v", msg, err)
			}
			sig, err := L2Signature(v.Secret, msg)
			if err != nil || sig != v.Signature {
				t.Fatalf("signature %q: %v", sig, err)
			}
		})
	}
	signer, err := NewLocalSigner(vectors.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	number := func(t *testing.T, s string) *big.Int {
		t.Helper()
		n, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("invalid integer %q", s)
		}
		return n
	}
	hash := func(t *testing.T, s string) Hash {
		t.Helper()
		b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil || len(b) != 32 {
			t.Fatalf("invalid hash %q", s)
		}
		var h Hash
		copy(h[:], b)
		return h
	}
	for _, v := range vectors.Orders {
		t.Run("order/"+v.Name, func(t *testing.T) {
			maker, err := ParseAddress(v.Maker)
			if err != nil {
				t.Fatal(err)
			}
			address, err := ParseAddress(v.Signer)
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := ExchangeAddress(v.ChainID, v.Version, v.NegRisk)
			if err != nil || !strings.EqualFold(exchange.String(), v.Exchange) {
				t.Fatalf("exchange %s: %v", exchange, err)
			}
			o := Order{Version: v.Version, Salt: v.Salt, Maker: maker, Signer: address, TokenID: number(t, v.TokenID), MakerAmount: number(t, v.MakerAmount), TakerAmount: number(t, v.TakerAmount), Side: Side(v.Side), SignatureType: SignatureType(v.SignatureType), Timestamp: number(t, v.Timestamp), Expiration: number(t, v.Expiration), Metadata: hash(t, v.Metadata), Builder: hash(t, v.Builder)}
			digest, err := OrderDigest(o, v.ChainID, v.NegRisk)
			if err != nil {
				t.Fatal(err)
			}
			signed, err := SignOrder(signer, SignableOrder{Order: o, OrderType: GTC}, v.ChainID, v.NegRisk, "owner")
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("0x%x", digest); got != v.Digest {
				t.Errorf("digest got %s want %s", got, v.Digest)
			}
			if signed.Signature != v.Signature {
				t.Errorf("signature got %s want %s", signed.Signature, v.Signature)
			}
		})
	}
}
