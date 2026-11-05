package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// decodeResponse matches Serde's exact field names and rejects duplicate known fields.
// encoding/json otherwise lets case variants override fields after validation.
func decodeResponse(b []byte, dst any, fields ...string) error {
	typ := reflect.TypeOf(dst).Elem()
	known := make(map[string]bool, typ.NumField())
	quotedUint := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")
		known[tag[0]] = true
		if field.Type.Kind() == reflect.Uint64 && len(tag) > 1 && tag[1] == "string" {
			quotedUint[tag[0]] = true
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("expected JSON object")
	}
	raw := make(map[string]json.RawMessage)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return fmt.Errorf("expected JSON field name")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		if !known[name] {
			continue
		}
		if _, exists := raw[name]; exists {
			return fmt.Errorf("duplicate field %s", name)
		}
		if quotedUint[name] && string(value) != "null" {
			var text string
			if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &text) != nil {
				return fmt.Errorf("%s: expected unsigned integer string", name)
			}
			digits := strings.TrimPrefix(text, "+")
			if digits == "" || strings.Trim(digits, "0123456789") != "" {
				return fmt.Errorf("%s: invalid unsigned integer %q", name, text)
			}
			n, err := strconv.ParseUint(digits, 10, 64)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			value = json.RawMessage(strconv.Quote(strconv.FormatUint(n, 10)))
		}
		raw[name] = value
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	for _, field := range fields {
		value, ok := raw[field]
		if !ok || string(value) == "null" {
			return fmt.Errorf("required field %s missing or null", field)
		}
	}
	filtered, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	decoded := reflect.New(typ)
	if err := json.Unmarshal(filtered, decoded.Interface()); err != nil {
		return err
	}
	reflect.ValueOf(dst).Elem().Set(decoded.Elem())
	return nil
}

// DisplayFromStr accepts a JSON string, not a JSON number.
func requireStringField(b []byte, field string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if value := raw[field]; len(value) == 0 || value[0] != '"' {
		return fmt.Errorf("%s: expected string", field)
	}
	return nil
}

// parseResponseBaseUnit follows ruint's FromStr, used by DisplayFromStr.
// Unlike the request representation, it accepts radix prefixes and underscores.
// Digitless values are rejected rather than silently becoming a zero amount.
func parseResponseBaseUnit(n Number) (Number, error) {
	s := string(n)
	base := 10
	if len(s) >= 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			base, s = 16, s[2:]
		case 'o', 'O':
			base, s = 8, s[2:]
		case 'b', 'B':
			base, s = 2, s[2:]
		}
	}
	s = strings.ReplaceAll(s, "_", "")
	if s == "" {
		return "", fmt.Errorf("invalid uint256 %q: no digits", n)
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", fmt.Errorf("invalid uint256 %q", n)
		}
	}
	v, ok := new(big.Int).SetString(s, base)
	if !ok || v.BitLen() > 256 {
		return "", fmt.Errorf("invalid uint256 %q", n)
	}
	return Number(v.String()), nil
}

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

type DepositResponse struct {
	Address DepositAddresses `json:"address"`
	Note    *string          `json:"note,omitempty"`
}

func (v *DepositResponse) UnmarshalJSON(b []byte) error {
	type plain DepositResponse
	return decodeResponse(b, (*plain)(v), "address")
}

type DepositAddresses struct {
	EVM string `json:"evm"`
	SVM string `json:"svm"`
	BTC string `json:"btc"`
}

func (v *DepositAddresses) UnmarshalJSON(b []byte) error {
	type plain DepositAddresses
	var decoded plain
	if err := decodeResponse(b, &decoded, "evm", "svm", "btc"); err != nil {
		return err
	}
	if err := validateEVMAddress(decoded.EVM); err != nil {
		return fmt.Errorf("evm: %w", err)
	}
	*v = DepositAddresses(decoded)
	return nil
}

type SupportedAssetsResponse struct {
	SupportedAssets []SupportedAsset `json:"supportedAssets"`
	Note            *string          `json:"note,omitempty"`
}

func (v *SupportedAssetsResponse) UnmarshalJSON(b []byte) error {
	type plain SupportedAssetsResponse
	return decodeResponse(b, (*plain)(v), "supportedAssets")
}

type SupportedAsset struct {
	ChainID        uint64 `json:"chainId,string"`
	ChainName      string `json:"chainName"`
	Token          Token  `json:"token"`
	MinCheckoutUSD Number `json:"minCheckoutUsd"`
}

func (v *SupportedAsset) UnmarshalJSON(b []byte) error {
	type plain SupportedAsset
	var decoded plain
	if err := decodeResponse(b, &decoded, "chainId", "chainName", "token", "minCheckoutUsd"); err != nil {
		return err
	}
	if !decimalPattern.MatchString(string(decoded.MinCheckoutUSD)) {
		return fmt.Errorf("minCheckoutUsd: invalid decimal %q", decoded.MinCheckoutUSD)
	}
	*v = SupportedAsset(decoded)
	return nil
}

type Token struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address"`
	Decimals uint8  `json:"decimals"`
}

func (v *Token) UnmarshalJSON(b []byte) error {
	type plain Token
	return decodeResponse(b, (*plain)(v), "name", "symbol", "address", "decimals")
}

type StatusResponse struct {
	Transactions []DepositTransaction `json:"transactions"`
}

func (v *StatusResponse) UnmarshalJSON(b []byte) error {
	type plain StatusResponse
	return decodeResponse(b, (*plain)(v), "transactions")
}

type DepositTransaction struct {
	FromChainID        uint64                   `json:"fromChainId,string"`
	FromTokenAddress   string                   `json:"fromTokenAddress"`
	FromAmountBaseUnit Number                   `json:"fromAmountBaseUnit"`
	ToChainID          uint64                   `json:"toChainId,string"`
	ToTokenAddress     string                   `json:"toTokenAddress"`
	Status             DepositTransactionStatus `json:"status"`
	TxHash             *string                  `json:"txHash,omitempty"`
	CreatedTimeMs      *uint64                  `json:"createdTimeMs,omitempty"`
}

func (v *DepositTransaction) UnmarshalJSON(b []byte) error {
	type plain DepositTransaction
	var decoded plain
	if err := decodeResponse(b, &decoded, "fromChainId", "fromTokenAddress", "fromAmountBaseUnit", "toChainId", "toTokenAddress", "status"); err != nil {
		return err
	}
	if err := requireStringField(b, "fromAmountBaseUnit"); err != nil {
		return err
	}
	amount, err := parseResponseBaseUnit(decoded.FromAmountBaseUnit)
	if err != nil {
		return fmt.Errorf("fromAmountBaseUnit: %w", err)
	}
	decoded.FromAmountBaseUnit = amount
	if err := validateEVMAddress(decoded.ToTokenAddress); err != nil {
		return fmt.Errorf("toTokenAddress: %w", err)
	}
	*v = DepositTransaction(decoded)
	return nil
}

type QuoteResponse struct {
	EstCheckoutTimeMs  uint64                `json:"estCheckoutTimeMs"`
	EstFeeBreakdown    EstimatedFeeBreakdown `json:"estFeeBreakdown"`
	EstInputUSD        float64               `json:"estInputUsd"`
	EstOutputUSD       float64               `json:"estOutputUsd"`
	EstToTokenBaseUnit Number                `json:"estToTokenBaseUnit"`
	QuoteID            string                `json:"quoteId"`
}

func (v *QuoteResponse) UnmarshalJSON(b []byte) error {
	type plain QuoteResponse
	var decoded plain
	if err := decodeResponse(b, &decoded, "estCheckoutTimeMs", "estFeeBreakdown", "estInputUsd", "estOutputUsd", "estToTokenBaseUnit", "quoteId"); err != nil {
		return err
	}
	if err := requireStringField(b, "estToTokenBaseUnit"); err != nil {
		return err
	}
	amount, err := parseResponseBaseUnit(decoded.EstToTokenBaseUnit)
	if err != nil {
		return fmt.Errorf("estToTokenBaseUnit: %w", err)
	}
	decoded.EstToTokenBaseUnit = amount
	*v = QuoteResponse(decoded)
	return nil
}

type EstimatedFeeBreakdown struct {
	AppFeeLabel     string  `json:"appFeeLabel"`
	AppFeePercent   float64 `json:"appFeePercent"`
	AppFeeUSD       float64 `json:"appFeeUsd"`
	FillCostPercent float64 `json:"fillCostPercent"`
	FillCostUSD     float64 `json:"fillCostUsd"`
	GasUSD          float64 `json:"gasUsd"`
	MaxSlippage     float64 `json:"maxSlippage"`
	MinReceived     float64 `json:"minReceived"`
	SwapImpact      float64 `json:"swapImpact"`
	SwapImpactUSD   float64 `json:"swapImpactUsd"`
	TotalImpact     float64 `json:"totalImpact"`
	TotalImpactUSD  float64 `json:"totalImpactUsd"`
}

func (v *EstimatedFeeBreakdown) UnmarshalJSON(b []byte) error {
	type plain EstimatedFeeBreakdown
	return decodeResponse(b, (*plain)(v), "appFeeLabel", "appFeePercent", "appFeeUsd", "fillCostPercent", "fillCostUsd", "gasUsd", "maxSlippage", "minReceived", "swapImpact", "swapImpactUsd", "totalImpact", "totalImpactUsd")
}

type WithdrawResponse struct {
	Address WithdrawalAddresses `json:"address"`
	Note    string              `json:"note"`
}

func (v *WithdrawResponse) UnmarshalJSON(b []byte) error {
	type plain WithdrawResponse
	return decodeResponse(b, (*plain)(v), "address", "note")
}

type WithdrawalAddresses struct {
	EVM string `json:"evm"`
	SVM string `json:"svm"`
	BTC string `json:"btc"`
}

func (v *WithdrawalAddresses) UnmarshalJSON(b []byte) error {
	var addresses DepositAddresses
	if err := json.Unmarshal(b, &addresses); err != nil {
		return err
	}
	*v = WithdrawalAddresses(addresses)
	return nil
}
