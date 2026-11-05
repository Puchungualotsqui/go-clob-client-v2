// Package clob contains transport-independent Polymarket CLOB types.
package clob

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Decimal preserves the exact base-10 representation, including trailing zeroes.
// Its zero value represents zero. Use ParseDecimal for values from untrusted sources.
type Decimal string

var decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

func ParseDecimal(s string) (Decimal, error) {
	if !decimalPattern.MatchString(s) {
		return "", fmt.Errorf("invalid decimal %q", s)
	}
	return Decimal(s), nil
}
func (d Decimal) String() string {
	if d == "" {
		return "0"
	}
	return string(d)
}
func (d Decimal) MarshalJSON() ([]byte, error) {
	if _, err := ParseDecimal(d.String()); err != nil {
		return nil, err
	}
	return json.Marshal(d.String())
}
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else {
		s = string(b)
	}
	v, err := ParseDecimal(s)
	if err == nil {
		*d = v
	}
	return err
}
func (d Decimal) normalized() Decimal {
	s := d.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	p := strings.SplitN(s, ".", 2)
	whole := strings.TrimLeft(p[0], "0")
	if whole == "" {
		whole = "0"
	}
	frac := ""
	if len(p) == 2 {
		frac = strings.TrimRight(p[1], "0")
	}
	if frac != "" {
		whole += "." + frac
	}
	if neg && whole != "0" {
		whole = "-" + whole
	}
	return Decimal(whole)
}

// U256 stores an unsigned 256-bit integer as a decimal string. Its zero value is zero.
type U256 string

func ParseU256(s string) (U256, error) {
	if s == "" {
		return "", fmt.Errorf("invalid uint256 %q", s)
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("invalid uint256 %q", s)
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.BitLen() > 256 {
		return "", fmt.Errorf("uint256 out of range: %q", s)
	}
	return U256(n.String()), nil
}
func (u U256) String() string {
	if u == "" {
		return "0"
	}
	return string(u)
}
func (u U256) MarshalJSON() ([]byte, error) {
	v, err := ParseU256(u.String())
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(v))
}
func (u *U256) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else {
		s = string(b)
	}
	v, err := ParseU256(s)
	if err == nil {
		*u = v
	}
	return err
}

// B256 retains the hex wire representation. Its zero value represents the zero hash.
// ParseB256 validates values from untrusted input.
type B256 string

const zeroB256 = "0x0000000000000000000000000000000000000000000000000000000000000000"

func (b B256) wireString() string {
	if b == "" {
		return zeroB256
	}
	return string(b)
}

// MarshalJSON rejects malformed hashes, including in embedded request structs.
func (b B256) MarshalJSON() ([]byte, error) {
	s := b.wireString()
	if _, err := ParseB256(s); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

type APIKey string

func parseHex(s string, n int) error {
	if len(s) != 2+n*2 || !strings.HasPrefix(s, "0x") {
		return fmt.Errorf("expected 0x-prefixed %d-byte hex value", n)
	}
	for _, c := range s[2:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return fmt.Errorf("invalid hex value %q", s)
		}
	}
	return nil
}

func ParseB256(s string) (B256, error) {
	if err := parseHex(s, 32); err != nil {
		return "", err
	}
	return B256(s), nil
}

// Side is encoded as 0 or 1 in EIP-712 orders and as a string on the wire.
type Side uint8

const (
	Buy Side = iota
	Sell
	SideUnknown Side = 255
)

func (s Side) String() string {
	switch s {
	case Buy:
		return "BUY"
	case Sell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

func (s Side) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }
func (s Side) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
func (s *Side) UnmarshalText(b []byte) error {
	switch string(b) {
	case "BUY", "buy":
		*s = Buy
	case "SELL", "sell":
		*s = Sell
	default:
		*s = SideUnknown
	}
	return nil
}

func SideFromUint8(v uint8) (Side, error) {
	switch v {
	case 0:
		return Buy, nil
	case 1:
		return Sell, nil
	default:
		return SideUnknown, NewValidationError(fmt.Sprintf("Unable to create Side from %d", v))
	}
}
func (s *Side) UnmarshalJSON(b []byte) error {
	var x string
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	return s.UnmarshalText([]byte(x))
}

type OrderType string

const (
	GTC OrderType = "GTC"
	FOK OrderType = "FOK"
	GTD OrderType = "GTD"
	FAK OrderType = "FAK"
)

func (s *OrderType) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v {
	case "gtc", "fok", "gtd", "fak":
		v = strings.ToUpper(v)
	}
	*s = OrderType(v)
	return nil
}

type OrderStatusType string

const (
	OrderLive      OrderStatusType = "LIVE"
	OrderMatched   OrderStatusType = "MATCHED"
	OrderCanceled  OrderStatusType = "CANCELED"
	OrderDelayed   OrderStatusType = "DELAYED"
	OrderUnmatched OrderStatusType = "UNMATCHED"
)

func (s *OrderStatusType) UnmarshalJSON(b []byte) error {
	v, err := knownUpper(b, "live", "matched", "canceled", "delayed", "unmatched")
	*s = OrderStatusType(v)
	return err
}

type TradeStatusType string

const (
	TradeMatched   TradeStatusType = "MATCHED"
	TradeMined     TradeStatusType = "MINED"
	TradeConfirmed TradeStatusType = "CONFIRMED"
	TradeRetrying  TradeStatusType = "RETRYING"
	TradeFailed    TradeStatusType = "FAILED"
)

func (s *TradeStatusType) UnmarshalJSON(b []byte) error {
	v, err := knownUpper(b, "matched", "mined", "confirmed", "retrying", "failed")
	*s = TradeStatusType(v)
	return err
}
func knownUpper(b []byte, known ...string) (string, error) {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", err
	}
	for _, v := range known {
		if s == v {
			return strings.ToUpper(v), nil
		}
	}
	return s, nil
}

type AssetType string

const (
	Collateral  AssetType = "COLLATERAL"
	Conditional AssetType = "CONDITIONAL"
)

type TraderSide string

const (
	Taker TraderSide = "TAKER"
	Maker TraderSide = "MAKER"
)

type SignatureType uint8

const (
	EOA SignatureType = iota
	Proxy
	GnosisSafe
	Poly1271

	SignatureEOA      = EOA
	SignatureProxy    = Proxy
	SignatureSafe     = GnosisSafe
	SignaturePoly1271 = Poly1271
)

type Interval string

const (
	OneMinute Interval = "1m"
	OneHour   Interval = "1h"
	SixHours  Interval = "6h"
	OneDay    Interval = "1d"
	OneWeek   Interval = "1w"
	Max       Interval = "max"
)

type TickSize string

const (
	Tenth         TickSize = "0.1"
	Hundredth     TickSize = "0.01"
	HalfCent      TickSize = "0.005"
	QuarterCent   TickSize = "0.0025"
	Thousandth    TickSize = "0.001"
	TenThousandth TickSize = "0.0001"
)

func ParseTickSize(d Decimal) (TickSize, error) {
	if _, err := ParseDecimal(d.String()); err != nil {
		return "", err
	}
	v := TickSize(d.normalized())
	switch v {
	case Tenth, Hundredth, HalfCent, QuarterCent, Thousandth, TenThousandth:
		return v, nil
	}
	return "", NewValidationError(fmt.Sprintf("Unknown tick size: %s. Expected one of: 0.1, 0.01, 0.005, 0.0025, 0.001, 0.0001", d))
}
func (t *TickSize) UnmarshalJSON(b []byte) error {
	var d Decimal
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	v, err := ParseTickSize(d)
	if err == nil {
		*t = v
	}
	return err
}
func (t TickSize) MarshalJSON() ([]byte, error) {
	v, err := ParseTickSize(Decimal(t))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(v))
}

type Amount struct {
	value Decimal
	usdc  bool
}

func NewUSDCAmount(d Decimal) (Amount, error) {
	if _, err := ParseDecimal(d.String()); err != nil {
		return Amount{}, err
	}
	d = d.normalized()
	parts := strings.Split(d.String(), ".")
	if len(parts) > 1 && len(parts[1]) > 6 {
		return Amount{}, NewValidationError(fmt.Sprintf("Unable to build Amount with %d decimal points, must be <= 6", len(parts[1])))
	}
	return Amount{d, true}, nil
}
func NewSharesAmount(d Decimal) (Amount, error) {
	if _, err := ParseDecimal(d.String()); err != nil {
		return Amount{}, err
	}
	s := d.String()
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 && len(parts[1]) > 2 {
		s = parts[0] + "." + parts[1][:2]
	}
	v := Decimal(s).normalized()
	if v == "0" && d.normalized() != "0" {
		return Amount{}, NewValidationError(fmt.Sprintf("Size %s rounds down to zero at lot size 2", d))
	}
	return Amount{v, false}, nil
}
func (a Amount) Value() Decimal { return a.value }
func (a Amount) IsUSDC() bool   { return a.usdc }
func (a Amount) IsShares() bool { return !a.usdc }

// EmptyDecimal is used only where the API treats blank strings as zero.
type EmptyDecimal struct{ Decimal }

func (d *EmptyDecimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if strings.TrimSpace(s) == "" {
		d.Decimal = "0"
		return nil
	}
	return json.Unmarshal(bytes.TrimSpace(b), &d.Decimal)
}
