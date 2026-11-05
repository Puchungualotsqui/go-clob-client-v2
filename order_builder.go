package clob

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// OrderParams supplies the market data and wallet configuration needed to build an order.
// Decimal values are strings to avoid floating-point rounding. Version 3 is for position IDs.
// Salt and TimestampMS may be injected for deterministic testing; nil uses secure randomness and the clock.
type OrderParams struct {
	Version             int
	ChainID             uint64 // required to derive an unspecified proxy or Safe funder
	AssetID             string
	Signer              Address
	Funder              *Address
	SignatureType       SignatureType
	Side                Side
	Price, TickSize     string
	OrderType           OrderType
	Expiration          uint64
	PostOnly, DeferExec *bool
	Salt, TimestampMS   *uint64
	Taker               Address
	Nonce, FeeRateBPS   uint64
	Metadata, Builder   Hash
	MarketBuyFees       *MarketBuyFees
	UserUSDCBalance     *string // client-assisted fee resolution for USDC market buys
}

// MarketBuyFees enables balance-aware sizing. Rates are fractions, not basis points.
// Exponent is the integer fee exponent supplied by the market.
type MarketBuyFees struct {
	Balance, PlatformRate, BuilderRate string
	Exponent                           uint32
}
type BookLevel struct{ Price, Size string }
type MarketAmount struct {
	Value  string
	InUSDC bool
}

func decimal(s string) (*big.Rat, int, error) {
	if s == "" {
		return nil, 0, errors.New("missing decimal")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return nil, 0, fmt.Errorf("invalid decimal %q", s)
	}
	digits := parts[0]
	scale := 0
	if len(parts) == 2 {
		if parts[1] == "" {
			return nil, 0, fmt.Errorf("invalid decimal %q", s)
		}
		digits += parts[1]
		scale = len(parts[1])
	}
	if len(digits) > 100 || scale > 28 {
		return nil, 0, errors.New("decimal precision exceeds supported range")
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return nil, 0, fmt.Errorf("invalid decimal %q", s)
		}
	}
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, 0, errors.New("invalid decimal")
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	return new(big.Rat).SetFrac(n, den), scale, nil
}
func pow10(scale int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
}
func truncate(x *big.Rat, scale int) *big.Rat {
	p := pow10(scale)
	n := new(big.Int).Mul(x.Num(), p)
	n.Quo(n, x.Denom())
	return new(big.Rat).SetFrac(n, p)
}
func fixed(x *big.Rat) *big.Int {
	n := new(big.Int).Mul(x.Num(), big.NewInt(1000000))
	return n.Quo(n, x.Denom())
}
func validPrice(price, tick *big.Rat) bool {
	if tick.Sign() <= 0 || price.Cmp(tick) < 0 || price.Cmp(new(big.Rat).Sub(big.NewRat(1, 1), tick)) > 0 {
		return false
	}
	q := new(big.Rat).Quo(price, tick)
	return q.IsInt()
}
func tickPrice(p OrderParams, market bool) (*big.Rat, int, error) {
	tick, scale, err := decimal(p.TickSize)
	if err != nil {
		return nil, 0, err
	}
	switch p.TickSize {
	case "0.1", "0.01", "0.005", "0.0025", "0.001", "0.0001":
	default:
		return nil, 0, errors.New("unsupported tick size")
	}
	price, ps, err := decimal(p.Price)
	if err != nil {
		return nil, 0, err
	}
	if !market && ps > scale {
		return nil, 0, errors.New("price exceeds tick size precision")
	}
	if !validPrice(price, tick) {
		return nil, 0, errors.New("price outside bounds or not aligned to tick")
	}
	return price, scale, nil
}
func buildPayload(p OrderParams, maker, taker *big.Int, typ OrderType, post *bool) (SignableOrder, error) {
	if p.Version < 1 || p.Version > 3 {
		return SignableOrder{}, errors.New("unsupported order version")
	}
	if p.Side > Sell || p.SignatureType > SignaturePoly1271 {
		return SignableOrder{}, errors.New("invalid side or signature type")
	}
	if p.Version == 1 && p.SignatureType == SignaturePoly1271 {
		return SignableOrder{}, errors.New("Poly1271 is not supported for V1")
	}
	if p.Version != 1 && (p.Taker != (Address{}) || p.Nonce != 0 || p.FeeRateBPS != 0) {
		return SignableOrder{}, errors.New("taker, nonce and fee rate are only supported for V1 orders")
	}
	id, ok := new(big.Int).SetString(p.AssetID, 10)
	if !ok || id.Sign() < 0 || id.BitLen() > 256 {
		return SignableOrder{}, errors.New("invalid asset ID")
	}
	if maker.Sign() <= 0 || taker.Sign() <= 0 || maker.BitLen() > 256 || taker.BitLen() > 256 {
		return SignableOrder{}, errors.New("order amounts must be positive uint256 values")
	}
	salt := uint64(0)
	if p.Salt != nil {
		salt = *p.Salt
	} else {
		n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 53))
		if err != nil {
			return SignableOrder{}, err
		}
		salt = n.Uint64()
	}
	salt &= (1 << 53) - 1
	ms := uint64(0)
	if p.TimestampMS != nil {
		ms = *p.TimestampMS
	} else {
		now := time.Now().UnixMilli()
		if now < 0 {
			return SignableOrder{}, errors.New("clock before Unix epoch")
		}
		ms = uint64(now)
	}
	funder := p.Signer
	if p.Funder != nil {
		if p.SignatureType == SignatureEOA {
			return SignableOrder{}, errors.New("EOA signature type cannot have a funder")
		}
		funder = *p.Funder
		if funder == (Address{}) {
			return SignableOrder{}, errors.New("funder cannot be zero")
		}
	} else if p.SignatureType == SignatureProxy || p.SignatureType == SignatureSafe {
		var err error
		funder, err = DeriveWallet(p.Signer, p.ChainID, p.SignatureType)
		if err != nil {
			return SignableOrder{}, err
		}
	}
	orderSigner := p.Signer
	if p.SignatureType == SignaturePoly1271 {
		if p.Funder == nil || funder == (Address{}) {
			return SignableOrder{}, errors.New("Poly1271 requires a deposit wallet funder")
		}
		orderSigner = funder
	}
	o := Order{Version: p.Version, Salt: salt, Maker: funder, Signer: orderSigner, Taker: p.Taker, TokenID: id, MakerAmount: maker, TakerAmount: taker, Expiration: new(big.Int).SetUint64(p.Expiration), Nonce: new(big.Int).SetUint64(p.Nonce), FeeRateBPS: new(big.Int).SetUint64(p.FeeRateBPS), Timestamp: new(big.Int).SetUint64(ms), Side: p.Side, SignatureType: p.SignatureType, Metadata: p.Metadata, Builder: p.Builder}
	return SignableOrder{Order: o, OrderType: typ, PostOnly: post, DeferExec: p.DeferExec}, nil
}

// BuildLimitOrder rounds shares down to the two-decimal lot size.
func BuildLimitOrder(p OrderParams, size string) (SignableOrder, error) {
	if p.UserUSDCBalance != nil {
		return SignableOrder{}, errors.New("user USDC balance is only supported for client-assisted market buys")
	}
	price, scale, err := tickPrice(p, false)
	if err != nil {
		return SignableOrder{}, err
	}
	shares, _, err := decimal(size)
	if err != nil {
		return SignableOrder{}, err
	}
	shares = truncate(shares, 2)
	if shares.Sign() <= 0 {
		return SignableOrder{}, errors.New("size rounds down to zero")
	}
	typ := p.OrderType
	if typ == "" {
		typ = GTC
	}
	if typ != GTC && typ != GTD && typ != FAK && typ != FOK {
		return SignableOrder{}, errors.New("invalid order type")
	}
	if p.Expiration > 0 && typ != GTD {
		return SignableOrder{}, errors.New("only GTD orders may expire")
	}
	post := false
	if p.PostOnly != nil {
		post = *p.PostOnly
	}
	if post && typ != GTC && typ != GTD {
		return SignableOrder{}, errors.New("post-only requires GTC or GTD")
	}
	usdc := truncate(new(big.Rat).Mul(shares, price), scale+2)
	maker, taker := fixed(usdc), fixed(shares)
	if p.Side == Sell {
		maker, taker = taker, maker
	}
	return buildPayload(p, maker, taker, typ, &post)
}

// CalculateMarketPrice walks levels in the CLOB wire order (worst first).
func CalculateMarketPrice(levels []BookLevel, side Side, amount MarketAmount, typ OrderType) (string, error) {
	if side > Sell || (side == Sell && amount.InUSDC) || (typ != FAK && typ != FOK) {
		return "", errors.New("invalid market side, amount, or order type")
	}
	target, _, err := decimal(amount.Value)
	if err != nil || target.Sign() <= 0 {
		return "", errors.New("invalid market amount")
	}
	if len(levels) == 0 {
		return "", errors.New("no opposing orders")
	}
	prices := make([]*big.Rat, len(levels))
	sizes := make([]*big.Rat, len(levels))
	for i, level := range levels {
		price, _, e := decimal(level.Price)
		if e != nil || price.Sign() <= 0 || price.Cmp(big.NewRat(1, 1)) >= 0 {
			return "", fmt.Errorf("invalid book price at level %d", i)
		}
		size, _, e := decimal(level.Size)
		if e != nil || size.Sign() <= 0 {
			return "", fmt.Errorf("invalid book size at level %d", i)
		}
		if i > 0 && ((side == Buy && prices[i-1].Cmp(price) < 0) || (side == Sell && prices[i-1].Cmp(price) > 0)) {
			return "", errors.New("book levels must be ordered worst first")
		}
		prices[i], sizes[i] = price, size
	}
	total := new(big.Rat)
	for i := len(levels) - 1; i >= 0; i-- {
		value := sizes[i]
		if amount.InUSDC {
			value = new(big.Rat).Mul(value, prices[i])
		}
		total.Add(total, value)
		if total.Cmp(target) >= 0 {
			return levels[i].Price, nil
		}
	}
	if typ == FOK {
		return "", errors.New("insufficient liquidity")
	}
	return levels[0].Price, nil
}

// adjustedMarketBuy applies platform and builder fees using exact arithmetic for
// the integer exponents returned by the market API.
func adjustedMarketBuy(amount, price *big.Rat, fees MarketBuyFees) (*big.Rat, error) {
	balance, _, err := decimal(fees.Balance)
	if err != nil {
		return nil, fmt.Errorf("balance: %w", err)
	}
	rate, _, err := decimal(fees.PlatformRate)
	if err != nil {
		return nil, fmt.Errorf("platform rate: %w", err)
	}
	builder, _, err := decimal(fees.BuilderRate)
	if err != nil {
		return nil, fmt.Errorf("builder rate: %w", err)
	}
	if fees.Exponent > 100 {
		return nil, errors.New("fee exponent too large")
	}
	base := new(big.Rat).Mul(price, new(big.Rat).Sub(big.NewRat(1, 1), price))
	factor := big.NewRat(1, 1)
	for i := uint32(0); i < fees.Exponent; i++ {
		factor.Mul(factor, base)
	}
	platform := new(big.Rat).Mul(rate, factor)
	divisor := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).Quo(platform, price))
	divisor.Add(divisor, builder)
	if balance.Cmp(new(big.Rat).Mul(amount, divisor)) <= 0 {
		amount = new(big.Rat).Quo(balance, divisor)
	}
	amount = truncate(amount, 6)
	if amount.Sign() <= 0 {
		return nil, errors.New("fee-adjusted market buy amount rounds to zero")
	}
	return amount, nil
}

// BuildMarketOrder uses a tick-aligned explicit price, or calculates one from worst-first book levels.
// A USDC buy may spend slightly less than requested to keep the signed price at or below the limit.
func BuildMarketOrder(p OrderParams, amount MarketAmount, levels []BookLevel) (SignableOrder, error) {
	if p.UserUSDCBalance != nil {
		return SignableOrder{}, errors.New("user USDC balance requires client-assisted fee resolution")
	}
	typ := p.OrderType
	if typ == "" {
		typ = FAK
	}
	if typ != FAK && typ != FOK {
		return SignableOrder{}, errors.New("market order requires FAK or FOK")
	}
	if p.PostOnly != nil && *p.PostOnly {
		return SignableOrder{}, errors.New("market order cannot be post-only")
	}
	if p.Expiration != 0 {
		return SignableOrder{}, errors.New("market order cannot expire")
	}
	if p.Side > Sell || (p.Side == Sell && amount.InUSDC) {
		return SignableOrder{}, errors.New("invalid market side or amount")
	}
	if p.MarketBuyFees != nil && (p.Side != Buy || !amount.InUSDC) {
		return SignableOrder{}, errors.New("market buy fees require a USDC-denominated buy")
	}
	raw, scaleAmount, err := decimal(amount.Value)
	if err != nil {
		return SignableOrder{}, err
	}
	if amount.InUSDC {
		trimmed := strings.TrimRight(amount.Value, "0")
		if strings.Contains(trimmed, ".") {
			scaleAmount = len(strings.Split(trimmed, ".")[1])
		} else {
			scaleAmount = 0
		}
		if scaleAmount > 6 {
			return SignableOrder{}, errors.New("USDC amount exceeds six decimal places")
		}
	} else {
		raw = truncate(raw, 2)
	}
	if raw.Sign() <= 0 {
		return SignableOrder{}, errors.New("amount must be positive after rounding")
	}
	if p.Price == "" { // Use the rounded amount for order book depth, matching the upstream Amount constructor.
		normalized := raw.FloatString(6)
		p.Price, err = CalculateMarketPrice(levels, p.Side, MarketAmount{Value: normalized, InUSDC: amount.InUSDC}, typ)
		if err != nil {
			return SignableOrder{}, err
		}
	}
	price, scale, err := tickPrice(p, true)
	if err != nil {
		return SignableOrder{}, err
	}
	if p.Side == Buy && amount.InUSDC && p.MarketBuyFees != nil {
		raw, err = adjustedMarketBuy(raw, price, *p.MarketBuyFees)
		if err != nil {
			return SignableOrder{}, err
		}
	}
	var maker, taker *big.Int
	if amount.InUSDC {
		shares := truncate(new(big.Rat).Quo(raw, price), scale+2)
		taker = fixed(shares)
		// Truncating shares alone would raise the signed price above the limit.
		maker = fixed(new(big.Rat).Mul(shares, price))
	} else {
		shares := fixed(raw)
		usdc := fixed(truncate(new(big.Rat).Mul(raw, price), scale+2))
		if p.Side == Buy {
			maker, taker = usdc, shares
		} else {
			maker, taker = shares, usdc
		}
	}
	return buildPayload(p, maker, taker, typ, nil)
}
