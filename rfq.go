package clob

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RFQState string
type RFQSortBy string
type RFQSortDir string

const (
	RFQActive   RFQState   = "active"
	RFQInactive RFQState   = "inactive"
	RFQPrice    RFQSortBy  = "price"
	RFQExpiry   RFQSortBy  = "expiry"
	RFQSize     RFQSortBy  = "size"
	RFQCreated  RFQSortBy  = "created"
	RFQAsc      RFQSortDir = "asc"
	RFQDesc     RFQSortDir = "desc"
)

// Asset is a token ID; zero denotes USDC.
type Asset = U256
type CreateRFQRequestRequest struct {
	AssetIn   Asset         `json:"assetIn"`
	AssetOut  Asset         `json:"assetOut"`
	AmountIn  Decimal       `json:"amountIn"`
	AmountOut Decimal       `json:"amountOut"`
	UserType  SignatureType `json:"userType"`
}
type CancelRFQRequestRequest struct {
	RequestID string `json:"requestId"`
}
type CreateRFQQuoteRequest struct {
	RequestID string        `json:"requestId"`
	AssetIn   Asset         `json:"assetIn"`
	AssetOut  Asset         `json:"assetOut"`
	AmountIn  Decimal       `json:"amountIn"`
	AmountOut Decimal       `json:"amountOut"`
	UserType  SignatureType `json:"userType"`
}
type CancelRFQQuoteRequest struct {
	QuoteID string `json:"quoteId"`
}
type AcceptRFQQuoteRequest struct {
	RequestID   string  `json:"requestId"`
	QuoteID     string  `json:"quoteId"`
	MakerAmount Decimal `json:"makerAmount"`
	TakerAmount Decimal `json:"takerAmount"`
	TokenID     U256    `json:"tokenId"`
	Maker       Address `json:"maker"`
	Signer      Address `json:"signer"`
	Taker       Address `json:"taker"`
	Nonce       uint64  `json:"nonce"`
	Expiration  int64   `json:"expiration"`
	Side        Side    `json:"side"`
	FeeRateBPS  uint64  `json:"feeRateBps"`
	Signature   string  `json:"signature"`
	Salt        string  `json:"salt"`
	Owner       APIKey  `json:"owner"`
}
type ApproveRFQOrderRequest = AcceptRFQQuoteRequest
type CreateRFQRequestResponse struct {
	RequestID string `json:"requestId"`
	Expiry    int64  `json:"expiry"`
}
type CreateRFQQuoteResponse struct {
	QuoteID string `json:"quoteId"`
}
type AcceptRFQQuoteResponse struct{}
type ApproveRFQOrderResponse struct {
	TradeIDs []string `json:"tradeIds"`
}
type RFQRequest struct {
	RequestID    string  `json:"requestId"`
	UserAddress  Address `json:"userAddress"`
	ProxyAddress Address `json:"proxyAddress"`
	Condition    B256    `json:"condition"`
	Token        U256    `json:"token"`
	Complement   U256    `json:"complement"`
	Side         Side    `json:"side"`
	SizeIn       Decimal `json:"sizeIn"`
	SizeOut      Decimal `json:"sizeOut"`
	Price        Decimal `json:"price"`
	Expiry       int64   `json:"expiry"`
}

// RFQ query filters use repeated request/quote IDs and comma-separated markets.
type RFQRequestsRequest struct {
	Offset      *string     `json:"offset,omitempty"`
	Limit       *uint32     `json:"limit,omitempty"`
	State       *RFQState   `json:"state,omitempty"`
	RequestIDs  []string    `json:"requestIds,omitempty"`
	Markets     []B256      `json:"-"`
	SizeMin     *Decimal    `json:"sizeMin,omitempty"`
	SizeMax     *Decimal    `json:"sizeMax,omitempty"`
	SizeUSDCMin *Decimal    `json:"sizeUsdcMin,omitempty"`
	SizeUSDCMax *Decimal    `json:"sizeUsdcMax,omitempty"`
	PriceMin    *Decimal    `json:"priceMin,omitempty"`
	PriceMax    *Decimal    `json:"priceMax,omitempty"`
	SortBy      *RFQSortBy  `json:"sortBy,omitempty"`
	SortDir     *RFQSortDir `json:"sortDir,omitempty"`
}

func rfqMarkets(markets []B256) ([]string, error) {
	result := make([]string, 0, len(markets))
	for i, market := range markets {
		wire := market.wireString()
		if _, err := ParseB256(wire); err != nil {
			return nil, fmt.Errorf("markets[%d]: %w", i, err)
		}
		result = append(result, wire)
	}
	return result, nil
}

func (r RFQRequestsRequest) MarshalJSON() ([]byte, error) {
	type plain RFQRequestsRequest
	markets, err := rfqMarkets(r.Markets)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		Markets string `json:"markets,omitempty"`
	}{plain(r), strings.Join(markets, ",")})
}

type RFQQuotesRequest struct {
	Offset      *string     `json:"offset,omitempty"`
	Limit       *uint32     `json:"limit,omitempty"`
	State       *RFQState   `json:"state,omitempty"`
	QuoteIDs    []string    `json:"quoteIds,omitempty"`
	RequestIDs  []string    `json:"requestIds,omitempty"`
	Markets     []B256      `json:"-"`
	SizeMin     *Decimal    `json:"sizeMin,omitempty"`
	SizeMax     *Decimal    `json:"sizeMax,omitempty"`
	SizeUSDCMin *Decimal    `json:"sizeUsdcMin,omitempty"`
	SizeUSDCMax *Decimal    `json:"sizeUsdcMax,omitempty"`
	PriceMin    *Decimal    `json:"priceMin,omitempty"`
	PriceMax    *Decimal    `json:"priceMax,omitempty"`
	SortBy      *RFQSortBy  `json:"sortBy,omitempty"`
	SortDir     *RFQSortDir `json:"sortDir,omitempty"`
}

func (r RFQQuotesRequest) MarshalJSON() ([]byte, error) {
	type plain RFQQuotesRequest
	markets, err := rfqMarkets(r.Markets)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		Markets string `json:"markets,omitempty"`
	}{plain(r), strings.Join(markets, ",")})
}

// rfqDecodeExact prevents encoding/json's case-insensitive matching from
// allowing unknown keys to overwrite serde's case-sensitive wire fields.
func rfqDecodeExact(fields map[string]json.RawMessage, dst any, names ...string) error {
	exact := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		if raw, ok := fields[name]; ok {
			exact[name] = raw
		}
	}
	b, err := json.Marshal(exact)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func (r *CreateRFQRequestResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFieldsWithCaseCheck(b, false, nil, "requestId", "expiry")
	if err != nil {
		return err
	}
	type plain CreateRFQRequestResponse
	var v plain
	if err := rfqDecodeExact(fields, &v, "requestId", "expiry"); err != nil {
		return err
	}
	*r = CreateRFQRequestResponse(v)
	return nil
}

func (r *CreateRFQQuoteResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFieldsWithCaseCheck(b, false, nil, "quoteId")
	if err != nil {
		return err
	}
	type plain CreateRFQQuoteResponse
	var v plain
	if err := rfqDecodeExact(fields, &v, "quoteId"); err != nil {
		return err
	}
	*r = CreateRFQQuoteResponse(v)
	return nil
}

func (r *ApproveRFQOrderResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFieldsWithCaseCheck(b, false, nil, "tradeIds")
	if err != nil {
		return err
	}
	if err := responseStringList(fields, "tradeIds", false); err != nil {
		return err
	}
	type plain ApproveRFQOrderResponse
	var v plain
	if err := rfqDecodeExact(fields, &v, "tradeIds"); err != nil {
		return err
	}
	*r = ApproveRFQOrderResponse(v)
	return nil
}

func validateRFQRecord(b []byte, quote bool) (map[string]json.RawMessage, []string, error) {
	required := []string{"requestId", "userAddress", "proxyAddress", "condition", "token", "complement", "side", "sizeIn", "sizeOut", "price"}
	if quote {
		required = append(required, "quoteId")
	} else {
		required = append(required, "expiry")
	}
	fields, err := responseFieldsWithCaseCheck(b, false, nil, required...)
	if err != nil {
		return nil, nil, err
	}
	for _, name := range []string{"userAddress", "proxyAddress"} {
		if err := responseAddress(fields, name); err != nil {
			return nil, nil, err
		}
	}
	if err := responseHash(fields, "condition"); err != nil {
		return nil, nil, err
	}
	return fields, required, nil
}

func (r *RFQRequest) UnmarshalJSON(b []byte) error {
	fields, names, err := validateRFQRecord(b, false)
	if err != nil {
		return err
	}
	type plain RFQRequest
	var v plain
	if err := rfqDecodeExact(fields, &v, names...); err != nil {
		return err
	}
	*r = RFQRequest(v)
	return nil
}

func (r *RFQQuote) UnmarshalJSON(b []byte) error {
	fields, names, err := validateRFQRecord(b, true)
	if err != nil {
		return err
	}
	type plain RFQQuote
	var v plain
	if err := rfqDecodeExact(fields, &v, names...); err != nil {
		return err
	}
	*r = RFQQuote(v)
	return nil
}

type RFQQuote struct {
	QuoteID      string  `json:"quoteId"`
	RequestID    string  `json:"requestId"`
	UserAddress  Address `json:"userAddress"`
	ProxyAddress Address `json:"proxyAddress"`
	Condition    B256    `json:"condition"`
	Token        U256    `json:"token"`
	Complement   U256    `json:"complement"`
	Side         Side    `json:"side"`
	SizeIn       Decimal `json:"sizeIn"`
	SizeOut      Decimal `json:"sizeOut"`
	Price        Decimal `json:"price"`
}
