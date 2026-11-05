package clob

import (
	"encoding/hex"
	"fmt"
	"math/big"
)

// OrderV1 and OrderV2 are the wire-model representations of the signed fields.
type OrderV1 struct {
	Salt          uint64  `json:"salt"`
	Maker         Address `json:"maker"`
	Signer        Address `json:"signer"`
	Taker         Address `json:"taker"`
	TokenID       U256    `json:"tokenId"`
	MakerAmount   U256    `json:"makerAmount"`
	TakerAmount   U256    `json:"takerAmount"`
	Expiration    U256    `json:"expiration"`
	Nonce         U256    `json:"nonce"`
	FeeRateBPS    U256    `json:"feeRateBps"`
	Side          uint8   `json:"side"`
	SignatureType uint8   `json:"signatureType"`
}
type OrderV2 struct {
	Salt          uint64  `json:"salt"`
	Maker         Address `json:"maker"`
	Signer        Address `json:"signer"`
	TokenID       U256    `json:"tokenId"`
	MakerAmount   U256    `json:"makerAmount"`
	TakerAmount   U256    `json:"takerAmount"`
	Side          uint8   `json:"side"`
	SignatureType uint8   `json:"signatureType"`
	Timestamp     U256    `json:"timestamp"`
	Metadata      B256    `json:"metadata"`
	Builder       B256    `json:"builder"`
}
type OrderPayloadV1 struct{ Order OrderV1 }
type OrderPayloadV2 struct {
	Order      OrderV2
	Expiration U256
}
type OrderPayloadV3 = OrderPayloadV2

// OrderPayload must contain exactly one payload. V3 uses the V2 structure.
type OrderPayload struct {
	V1 *OrderPayloadV1
	V2 *OrderPayloadV2
	V3 *OrderPayloadV3
}

func (p OrderPayload) Version() int {
	n, v := 0, 0
	if p.V1 != nil {
		n++
		v = 1
	}
	if p.V2 != nil {
		n++
		v = 2
	}
	if p.V3 != nil {
		n++
		v = 3
	}
	if n != 1 {
		return 0
	}
	return v
}

func payloadInteger(v U256) (*big.Int, error) {
	n, err := ParseU256(v.String())
	if err != nil {
		return nil, err
	}
	x, _ := new(big.Int).SetString(string(n), 10)
	return x, nil
}

func payloadHash(v B256) (Hash, error) {
	var h Hash
	if v == "" {
		return h, nil
	}
	if _, err := ParseB256(string(v)); err != nil {
		return h, err
	}
	b, _ := hex.DecodeString(string(v)[2:])
	copy(h[:], b)
	return h, nil
}

// ToOrder converts the model payload to the canonical crypto order for signing.
func (p OrderPayload) ToOrder() (Order, error) {
	version := p.Version()
	if version == 0 {
		return Order{}, fmt.Errorf("order payload must contain exactly one version")
	}
	o := Order{Version: version}
	var token, maker, taker, expiration, nonce, fee, timestamp U256
	var side, sig uint8
	if version == 1 {
		v := p.V1.Order
		o.Salt, o.Maker, o.Signer, o.Taker = v.Salt, v.Maker, v.Signer, v.Taker
		token, maker, taker, expiration, nonce, fee = v.TokenID, v.MakerAmount, v.TakerAmount, v.Expiration, v.Nonce, v.FeeRateBPS
		side, sig = v.Side, v.SignatureType
	} else {
		v := p.V2
		if version == 3 {
			v = p.V3
		}
		o.Salt, o.Maker, o.Signer = v.Order.Salt, v.Order.Maker, v.Order.Signer
		token, maker, taker, expiration, timestamp = v.Order.TokenID, v.Order.MakerAmount, v.Order.TakerAmount, v.Expiration, v.Order.Timestamp
		side, sig = v.Order.Side, v.Order.SignatureType
		var err error
		if o.Metadata, err = payloadHash(v.Order.Metadata); err != nil {
			return Order{}, fmt.Errorf("metadata: %w", err)
		}
		if o.Builder, err = payloadHash(v.Order.Builder); err != nil {
			return Order{}, fmt.Errorf("builder: %w", err)
		}
	}
	o.Side, o.SignatureType = Side(side), SignatureType(sig)
	fields := []struct {
		name string
		src  U256
		dst  **big.Int
	}{
		{"tokenId", token, &o.TokenID}, {"makerAmount", maker, &o.MakerAmount}, {"takerAmount", taker, &o.TakerAmount},
		{"expiration", expiration, &o.Expiration}, {"nonce", nonce, &o.Nonce}, {"feeRateBps", fee, &o.FeeRateBPS}, {"timestamp", timestamp, &o.Timestamp},
	}
	for _, f := range fields {
		v, err := payloadInteger(f.src)
		if err != nil {
			return Order{}, fmt.Errorf("%s: %w", f.name, err)
		}
		*f.dst = v
	}
	if _, err := o.structHash(); err != nil {
		return Order{}, err
	}
	return o, nil
}

// NewOrderPayload converts a crypto order into the corresponding model payload.
func NewOrderPayload(o Order) (OrderPayload, error) {
	if _, err := o.structHash(); err != nil {
		return OrderPayload{}, err
	}
	if _, err := wordUint(o.Expiration); err != nil {
		return OrderPayload{}, fmt.Errorf("expiration: %w", err)
	}
	if o.Version == 1 {
		return OrderPayload{V1: &OrderPayloadV1{Order: OrderV1{
			Salt: o.Salt, Maker: o.Maker, Signer: o.Signer, Taker: o.Taker,
			TokenID: U256(o.TokenID.String()), MakerAmount: U256(o.MakerAmount.String()), TakerAmount: U256(o.TakerAmount.String()),
			Expiration: U256(o.Expiration.String()), Nonce: U256(o.Nonce.String()), FeeRateBPS: U256(o.FeeRateBPS.String()),
			Side: uint8(o.Side), SignatureType: uint8(o.SignatureType),
		}}}, nil
	}
	p := &OrderPayloadV2{Order: OrderV2{
		Salt: o.Salt, Maker: o.Maker, Signer: o.Signer, TokenID: U256(o.TokenID.String()),
		MakerAmount: U256(o.MakerAmount.String()), TakerAmount: U256(o.TakerAmount.String()),
		Side: uint8(o.Side), SignatureType: uint8(o.SignatureType), Timestamp: U256(o.Timestamp.String()),
		Metadata: B256("0x" + hex.EncodeToString(o.Metadata[:])), Builder: B256("0x" + hex.EncodeToString(o.Builder[:])),
	}, Expiration: U256(o.Expiration.String())}
	if o.Version == 2 {
		return OrderPayload{V2: p}, nil
	}
	return OrderPayload{V3: p}, nil
}
