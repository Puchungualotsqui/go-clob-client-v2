package clob

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
)

// Order contains both legacy and V2/V3 fields. Expiration is not signed in V2/V3.
// Integer fields are unsigned 256-bit integers; callers must not mutate them while signing.
type Order struct {
	Version                                                                     int
	Salt                                                                        uint64
	Maker, Signer, Taker                                                        Address
	TokenID, MakerAmount, TakerAmount, Expiration, Nonce, FeeRateBPS, Timestamp *big.Int
	Side                                                                        Side
	SignatureType                                                               SignatureType
	Metadata, Builder                                                           Hash
}
type SignableOrder struct {
	Order               Order
	OrderType           OrderType
	PostOnly, DeferExec *bool
}
type SignedOrder struct {
	SignableOrder
	Signature string
	Owner     string
}

const orderTypeV1 = "Order(uint256 salt,address maker,address signer,address taker,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint256 expiration,uint256 nonce,uint256 feeRateBps,uint8 side,uint8 signatureType)"
const orderTypeV2 = "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"
const soladyType = "TypedDataSign(Order contents,string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)" + orderTypeV2

func (o Order) structHash() (Hash, error) {
	var zero Hash
	if o.Version < 1 || o.Version > 3 {
		return zero, errors.New("unsupported order version")
	}
	if o.Side > Sell || o.SignatureType > SignaturePoly1271 || (o.Version == 1 && o.SignatureType == SignaturePoly1271) {
		return zero, errors.New("invalid side or signature type")
	}
	typ := orderTypeV2
	if o.Version == 1 {
		typ = orderTypeV1
	}
	parts := [][]byte{hashString(typ), uintWord(o.Salt), addressWord(o.Maker), addressWord(o.Signer)}
	if o.Version == 1 {
		parts = append(parts, addressWord(o.Taker))
	}
	values := []*big.Int{o.TokenID, o.MakerAmount, o.TakerAmount}
	if o.Version == 1 {
		values = append(values, o.Expiration, o.Nonce, o.FeeRateBPS)
	}
	for _, v := range values {
		w, err := wordUint(v)
		if err != nil {
			return zero, err
		}
		parts = append(parts, w)
	}
	parts = append(parts, uintWord(uint64(o.Side)), uintWord(uint64(o.SignatureType)))
	if o.Version != 1 {
		w, err := wordUint(o.Timestamp)
		if err != nil {
			return zero, err
		}
		parts = append(parts, w, o.Metadata[:], o.Builder[:])
	}
	return keccak(parts...), nil
}

// ExchangeAddress selects the upstream contract for a chain, version, and risk mode.
func ExchangeAddress(chainID uint64, version int, negRisk bool) (Address, error) {
	var v string
	switch version {
	case 1:
		if negRisk {
			v = "0xC5d563A36AE78145C45a50134d48A1215220f80a"
		} else if chainID == 137 {
			v = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
		} else {
			v = "0xdFE02Eb6733538f8Ea35D585af8DE5958AD99E40"
		}
	case 2:
		if negRisk {
			v = "0xe2222d279d744050d28e00520010520000310F59"
		} else {
			v = "0xE111180000d2663C0091e4f400237545B87B996B"
		}
	case 3:
		if chainID == 137 {
			v = "0xe3333700cA9d93003F00f0F71f8515005F6c00Aa"
		} else {
			v = "0x9fE6e61422AdB6F610d8597F9684b16912D50C3D"
		}
	default:
		return Address{}, errors.New("unsupported order version")
	}
	if chainID != 137 && chainID != 80002 {
		return Address{}, errors.New("unsupported chain")
	}
	return ParseAddress(v)
}

// OrderDigest returns the standard EIP-712 order digest (not the Poly1271 wrapped digest).
func OrderDigest(order Order, chainID uint64, negRisk bool) (Hash, error) {
	content, err := order.structHash()
	if err != nil {
		return Hash{}, err
	}
	exchange, err := ExchangeAddress(chainID, order.Version, negRisk)
	if err != nil {
		return Hash{}, err
	}
	return typedDigest(domainHash("Polymarket CTF Exchange", strconv.Itoa(order.Version), chainID, &exchange), content), nil
}

// SignOrder signs V1/V2/V3 EIP-712 orders, including the Solady Poly1271 wrapper.
// The result owns copies of the input's integer and boolean fields. Callers must
// not mutate the returned order after signing; V2/V3 expiration is not signed.
func SignOrder(s HashSigner, order SignableOrder, chainID uint64, negRisk bool, owner string) (SignedOrder, error) {
	if s == nil {
		return SignedOrder{}, errors.New("missing signer")
	}
	cloneInt := func(v *big.Int) *big.Int {
		if v == nil {
			return nil
		}
		return new(big.Int).Set(v)
	}
	o := order.Order
	o.TokenID, o.MakerAmount, o.TakerAmount = cloneInt(o.TokenID), cloneInt(o.MakerAmount), cloneInt(o.TakerAmount)
	o.Expiration, o.Nonce, o.FeeRateBPS, o.Timestamp = cloneInt(o.Expiration), cloneInt(o.Nonce), cloneInt(o.FeeRateBPS), cloneInt(o.Timestamp)
	order.Order = o
	if order.PostOnly != nil {
		v := *order.PostOnly
		order.PostOnly = &v
	}
	if order.DeferExec != nil {
		v := *order.DeferExec
		order.DeferExec = &v
	}
	if o.SignatureType != SignaturePoly1271 && s.Address() != o.Signer {
		return SignedOrder{}, errors.New("signer address does not match order signer")
	}
	if o.SignatureType == SignatureEOA && o.Maker != o.Signer {
		return SignedOrder{}, errors.New("EOA maker and signer must match")
	}
	if o.Version != 1 && (o.Taker != (Address{}) || (o.Nonce != nil && o.Nonce.Sign() != 0) || (o.FeeRateBPS != nil && o.FeeRateBPS.Sign() != 0)) {
		return SignedOrder{}, errors.New("taker, nonce and fee rate are only supported for V1 orders")
	}
	if o.Salt > (1<<53)-1 {
		return SignedOrder{}, errors.New("order salt exceeds JavaScript safe integer range")
	}
	if _, err := wordUint(o.Expiration); err != nil {
		return SignedOrder{}, fmt.Errorf("expiration: %w", err)
	}
	result := SignedOrder{SignableOrder: order, Owner: owner}
	content, err := o.structHash()
	if err != nil {
		return SignedOrder{}, err
	}
	exchange, err := ExchangeAddress(chainID, o.Version, negRisk)
	if err != nil {
		return SignedOrder{}, err
	}
	domain := domainHash("Polymarket CTF Exchange", strconv.Itoa(o.Version), chainID, &exchange)
	digest := typedDigest(domain, content)
	if o.SignatureType == SignaturePoly1271 {
		if o.Maker != o.Signer {
			return SignedOrder{}, errors.New("Poly1271 maker and signer must be the deposit wallet")
		}
		wrappedHash := keccak(hashString(soladyType), content[:], hashString("DepositWallet"), hashString("1"), uintWord(chainID), addressWord(o.Signer), make([]byte, 32))
		digest = typedDigest(domain, wrappedHash)
	}
	sig, err := signDigest(s, digest)
	if err != nil {
		return SignedOrder{}, err
	}
	if o.SignatureType == SignaturePoly1271 {
		sig = append(sig, domain[:]...)
		sig = append(sig, content[:]...)
		sig = append(sig, []byte(orderTypeV2)...)
		sig = append(sig, byte(len(orderTypeV2)>>8), byte(len(orderTypeV2)))
	}
	result.Signature = "0x" + hex.EncodeToString(sig)
	return result, nil
}

// MarshalJSON implements the CLOB order submission format.
func (s SignedOrder) MarshalJSON() ([]byte, error) {
	o := s.Order
	if _, err := o.structHash(); err != nil {
		return nil, err
	}
	if _, err := wordUint(o.Expiration); err != nil {
		return nil, fmt.Errorf("expiration: %w", err)
	}
	side := "BUY"
	if o.Side == Sell {
		side = "SELL"
	}
	body := map[string]any{"salt": o.Salt, "maker": o.Maker.String(), "signer": o.Signer.String(), "tokenId": o.TokenID.String(), "makerAmount": o.MakerAmount.String(), "takerAmount": o.TakerAmount.String(), "expiration": o.Expiration.String(), "side": side, "signatureType": o.SignatureType, "signature": s.Signature}
	if o.Version == 1 {
		body["taker"] = o.Taker.String()
		body["nonce"] = o.Nonce.String()
		body["feeRateBps"] = o.FeeRateBPS.String()
	} else {
		body["timestamp"] = o.Timestamp.String()
		body["metadata"] = "0x" + hex.EncodeToString(o.Metadata[:])
		body["builder"] = "0x" + hex.EncodeToString(o.Builder[:])
	}
	out := map[string]any{"order": body, "orderType": s.OrderType, "owner": s.Owner}
	if s.PostOnly != nil {
		out["postOnly"] = *s.PostOnly
	}
	if s.DeferExec != nil {
		out["deferExec"] = *s.DeferExec
	}
	return json.Marshal(out)
}
