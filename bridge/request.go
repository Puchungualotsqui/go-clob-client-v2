package bridge

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

func validateBaseUnit(n Number) error {
	s := string(n)
	if len(s) == 0 {
		return fmt.Errorf("base unit amount must be an unsigned 256-bit decimal integer")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return fmt.Errorf("base unit amount must be an unsigned 256-bit decimal integer")
		}
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok || v.BitLen() > 256 {
		return fmt.Errorf("base unit amount must be an unsigned 256-bit decimal integer")
	}
	return nil
}

func requireAddress(field, address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("%s: address must not be empty", field)
	}
	return nil
}

func validateEVMAddress(address string) error {
	if !common.IsHexAddress(address) {
		return fmt.Errorf("invalid EVM address %q", address)
	}
	return nil
}

type DepositRequest struct {
	Address string `json:"address"`
}

func (r DepositRequest) MarshalJSON() ([]byte, error) {
	if err := validateEVMAddress(r.Address); err != nil {
		return nil, fmt.Errorf("address: %w", err)
	}
	type plain DepositRequest
	return json.Marshal(plain(r))
}

type StatusRequest struct {
	Address string `json:"address"`
}

type QuoteRequest struct {
	FromAmountBaseUnit Number `json:"fromAmountBaseUnit"`
	FromChainID        uint64 `json:"fromChainId,string"`
	FromTokenAddress   string `json:"fromTokenAddress"`
	RecipientAddress   string `json:"recipientAddress"`
	ToChainID          uint64 `json:"toChainId,string"`
	ToTokenAddress     string `json:"toTokenAddress"`
}

func (r QuoteRequest) MarshalJSON() ([]byte, error) {
	if err := validateBaseUnit(r.FromAmountBaseUnit); err != nil {
		return nil, fmt.Errorf("fromAmountBaseUnit: %w", err)
	}
	for _, field := range []struct{ name, value string }{
		{"fromTokenAddress", r.FromTokenAddress},
		{"recipientAddress", r.RecipientAddress},
		{"toTokenAddress", r.ToTokenAddress},
	} {
		if err := requireAddress(field.name, field.value); err != nil {
			return nil, err
		}
	}
	type plain QuoteRequest
	return json.Marshal(plain(r))
}

type WithdrawRequest struct {
	Address        string `json:"address"`
	ToChainID      uint64 `json:"toChainId,string"`
	ToTokenAddress string `json:"toTokenAddress"`
	RecipientAddr  string `json:"recipientAddr"`
}

func (r WithdrawRequest) MarshalJSON() ([]byte, error) {
	if err := validateEVMAddress(r.Address); err != nil {
		return nil, fmt.Errorf("address: %w", err)
	}
	for _, field := range []struct{ name, value string }{
		{"toTokenAddress", r.ToTokenAddress},
		{"recipientAddr", r.RecipientAddr},
	} {
		if err := requireAddress(field.name, field.value); err != nil {
			return nil, err
		}
	}
	type plain WithdrawRequest
	return json.Marshal(plain(r))
}
