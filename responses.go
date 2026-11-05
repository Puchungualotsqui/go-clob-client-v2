package clob

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// UnixSeconds and UnixMilliseconds decode numeric or string Unix timestamps.
type UnixSeconds struct{ time.Time }

func (t *UnixSeconds) UnmarshalJSON(b []byte) error {
	n, err := parseTimestamp(b)
	if err != nil {
		return err
	}
	t.Time = time.Unix(n, 0).UTC()
	return nil
}
func (t UnixSeconds) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(t.Unix(), 10))
}

type UnixMilliseconds struct{ time.Time }

func (t *UnixMilliseconds) UnmarshalJSON(b []byte) error {
	n, err := parseTimestamp(b)
	if err != nil {
		return err
	}
	t.Time = time.UnixMilli(n).UTC()
	return nil
}
func (t UnixMilliseconds) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(t.UnixMilli(), 10))
}
func parseTimestamp(b []byte) (int64, error) {
	s := string(b)
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return 0, err
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp %q: %w", s, err)
	}
	return n, nil
}

type MidpointResponse struct {
	Mid Decimal `json:"mid"`
}
type MidpointsResponse map[U256]Decimal
type PriceResponse struct {
	Price Decimal `json:"price"`
}
type PricesResponse map[U256]map[Side]Decimal
type SpreadResponse struct {
	Spread Decimal `json:"spread"`
}
type SpreadsResponse struct {
	Spreads map[U256]Decimal `json:"spreads"`
}
type PricePoint struct {
	T int64   `json:"t"`
	P Decimal `json:"p"`
}
type PriceHistoryResponse struct {
	History []PricePoint `json:"history"`
}
type TickSizeResponse struct {
	MinimumTickSize TickSize `json:"minimum_tick_size"`
}
type NegRiskResponse struct {
	NegRisk bool `json:"neg_risk"`
}

func (r *NegRiskResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, nil, "neg_risk"); err != nil {
		return err
	}
	var v struct {
		NegRisk *bool `json:"neg_risk"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.NegRisk == nil {
		return fmt.Errorf("neg_risk: required non-null field")
	}
	r.NegRisk = *v.NegRisk
	return nil
}

type FeeRateResponse struct {
	BaseFee uint32 `json:"base_fee"`
}

func (r *FeeRateResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, nil, "base_fee"); err != nil {
		return err
	}
	var v struct {
		BaseFee *uint32 `json:"base_fee"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.BaseFee == nil {
		return fmt.Errorf("base_fee: required non-null field")
	}
	r.BaseFee = *v.BaseFee
	return nil
}

type GeoblockResponse struct {
	Blocked bool   `json:"blocked"`
	IP      string `json:"ip"`
	Country string `json:"country"`
	Region  string `json:"region"`
}

func (r *GeoblockResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, []string{"ip", "country", "region"}, "blocked"); err != nil {
		return err
	}
	type plain GeoblockResponse
	var v struct {
		*plain
		Blocked *bool `json:"blocked"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Blocked == nil {
		return fmt.Errorf("blocked: required non-null field")
	}
	p.Blocked = *v.Blocked
	*r = GeoblockResponse(p)
	return nil
}

type OrderSummary struct {
	Price Decimal `json:"price"`
	Size  Decimal `json:"size"`
}

func (r *OrderSummary) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, nil, "price", "size"); err != nil {
		return err
	}
	type plain OrderSummary
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = OrderSummary(v)
	return nil
}

type OrderBookSummaryResponse struct {
	Market         B256             `json:"market"`
	AssetID        U256             `json:"asset_id"`
	Timestamp      UnixMilliseconds `json:"timestamp"`
	Hash           *string          `json:"hash"`
	Bids           []OrderSummary   `json:"bids"`
	Asks           []OrderSummary   `json:"asks"`
	MinOrderSize   Decimal          `json:"min_order_size"`
	NegRisk        bool             `json:"neg_risk"`
	TickSize       TickSize         `json:"tick_size"`
	LastTradePrice *Decimal         `json:"last_trade_price"`
}

func (r *OrderBookSummaryResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, []string{"market", "asset_id", "timestamp", "hash", "bids", "asks", "min_order_size", "tick_size", "last_trade_price"}, "neg_risk"); err != nil {
		return err
	}
	type plain OrderBookSummaryResponse
	var v struct {
		*plain
		LastTradePrice json.RawMessage `json:"last_trade_price"`
		NegRisk        *bool           `json:"neg_risk"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.NegRisk == nil {
		return fmt.Errorf("neg_risk: required non-null field")
	}
	p.NegRisk = *v.NegRisk
	p.LastTradePrice = nil
	if len(v.LastTradePrice) > 0 && string(v.LastTradePrice) != "null" {
		var d Decimal
		if json.Unmarshal(v.LastTradePrice, &d) == nil {
			p.LastTradePrice = &d
		}
	}
	if p.Bids == nil {
		p.Bids = []OrderSummary{}
	}
	if p.Asks == nil {
		p.Asks = []OrderSummary{}
	}
	*r = OrderBookSummaryResponse(p)
	return nil
}

type LastTradePriceResponse struct {
	Price Decimal `json:"price"`
	Side  Side    `json:"side"`
}
type LastTradesPricesResponse struct {
	TokenID U256    `json:"token_id"`
	Price   Decimal `json:"price"`
	Side    Side    `json:"side"`
}
type MarketByTokenResponse struct {
	ConditionID      B256 `json:"condition_id"`
	PrimaryTokenID   U256 `json:"primary_token_id"`
	SecondaryTokenID U256 `json:"secondary_token_id"`
}
type Token struct {
	TokenID U256    `json:"token_id"`
	Outcome string  `json:"outcome"`
	Price   Decimal `json:"price"`
	Winner  bool    `json:"winner"`
}

func (r *Token) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"winner"}, "token_id", "outcome", "price")
	if err != nil {
		return err
	}
	if raw, ok := fields["winner"]; ok && string(raw) == "null" {
		return fmt.Errorf("winner: expected bool, got null")
	}
	type plain Token
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Token(v)
	return nil
}

type RewardRate struct {
	AssetAddress     Address `json:"asset_address"`
	RewardsDailyRate Decimal `json:"rewards_daily_rate"`
}

func (r *RewardRate) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "asset_address", "rewards_daily_rate")
	if err != nil {
		return err
	}
	if err := responseAddress(fields, "asset_address"); err != nil {
		return err
	}
	type plain RewardRate
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = RewardRate(v)
	return nil
}

type Rewards struct {
	Rates     []RewardRate `json:"rates"`
	MinSize   Decimal      `json:"min_size"`
	MaxSpread Decimal      `json:"max_spread"`
}

func (r *Rewards) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, []string{"rates"}, "min_size", "max_spread"); err != nil {
		return err
	}
	type plain Rewards
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Rates == nil {
		v.Rates = []RewardRate{}
	}
	*r = Rewards(v)
	return nil
}

type MarketResponse struct {
	EnableOrderBook         bool       `json:"enable_order_book"`
	Active                  bool       `json:"active"`
	Closed                  bool       `json:"closed"`
	Archived                bool       `json:"archived"`
	AcceptingOrders         bool       `json:"accepting_orders"`
	AcceptingOrderTimestamp *time.Time `json:"accepting_order_timestamp"`
	MinimumOrderSize        Decimal    `json:"minimum_order_size"`
	MinimumTickSize         Decimal    `json:"minimum_tick_size"`
	ConditionID             *B256      `json:"condition_id"`
	QuestionID              *B256      `json:"question_id"`
	Question                string     `json:"question"`
	Description             string     `json:"description"`
	MarketSlug              string     `json:"market_slug"`
	EndDateISO              *time.Time `json:"end_date_iso"`
	GameStartTime           *time.Time `json:"game_start_time"`
	SecondsDelay            uint64     `json:"seconds_delay"`
	FPMM                    *Address   `json:"fpmm"`
	MakerBaseFee            Decimal    `json:"maker_base_fee"`
	TakerBaseFee            Decimal    `json:"taker_base_fee"`
	NotificationsEnabled    bool       `json:"notifications_enabled"`
	NegRisk                 bool       `json:"neg_risk"`
	NegRiskMarketID         *B256      `json:"neg_risk_market_id"`
	NegRiskRequestID        *B256      `json:"neg_risk_request_id"`
	Icon                    string     `json:"icon"`
	Image                   string     `json:"image"`
	Rewards                 Rewards    `json:"rewards"`
	Is5050Outcome           bool       `json:"is_50_50_outcome"`
	Tokens                  []Token    `json:"tokens"`
	Tags                    []string   `json:"tags"`
}

func (r *MarketResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, nil,
		"enable_order_book", "active", "closed", "archived", "accepting_orders",
		"minimum_order_size", "minimum_tick_size", "question", "description", "market_slug",
		"seconds_delay", "maker_base_fee", "taker_base_fee", "notifications_enabled", "neg_risk",
		"icon", "image", "rewards", "is_50_50_outcome",
	); err != nil {
		return err
	}
	type plain MarketResponse
	var v struct {
		*plain
		FPMM    json.RawMessage `json:"fpmm"`
		NegRisk *bool           `json:"neg_risk"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.NegRisk == nil {
		return fmt.Errorf("neg_risk: required non-null field")
	}
	p.NegRisk = *v.NegRisk
	if len(v.FPMM) > 0 && string(v.FPMM) != "null" && string(v.FPMM) != `""` {
		var address Address
		if err := json.Unmarshal(v.FPMM, &address); err != nil {
			return err
		}
		p.FPMM = &address
	}
	for _, field := range []struct {
		name  string
		value **B256
	}{
		{"condition_id", &p.ConditionID}, {"question_id", &p.QuestionID}, {"neg_risk_request_id", &p.NegRiskRequestID},
	} {
		if *field.value != nil {
			if **field.value == "" {
				*field.value = nil
				continue
			}
			if _, err := ParseB256(string(**field.value)); err != nil {
				return fmt.Errorf("%s: %w", field.name, err)
			}
		}
	}
	if p.NegRiskMarketID != nil {
		if _, err := ParseB256(string(*p.NegRiskMarketID)); err != nil {
			p.NegRiskMarketID = nil
		}
	}
	if p.Tokens == nil {
		p.Tokens = []Token{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	*r = MarketResponse(p)
	return nil
}

type SimplifiedMarketResponse struct {
	ConditionID     *B256   `json:"condition_id"`
	Tokens          []Token `json:"tokens"`
	Rewards         Rewards `json:"rewards"`
	Active          bool    `json:"active"`
	Closed          bool    `json:"closed"`
	Archived        bool    `json:"archived"`
	AcceptingOrders bool    `json:"accepting_orders"`
}

func (r *SimplifiedMarketResponse) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, []string{"condition_id", "tokens"}, "rewards", "active", "closed", "archived", "accepting_orders"); err != nil {
		return err
	}
	type plain SimplifiedMarketResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.ConditionID != nil {
		if *v.ConditionID == "" {
			v.ConditionID = nil
		} else if _, err := ParseB256(string(*v.ConditionID)); err != nil {
			return fmt.Errorf("condition_id: %w", err)
		}
	}
	if v.Tokens == nil {
		v.Tokens = []Token{}
	}
	*r = SimplifiedMarketResponse(v)
	return nil
}

type APIKeysResponse struct {
	Keys []APIKey `json:"apiKeys"`
}
type BanStatusResponse struct {
	ClosedOnly bool `json:"closed_only"`
}
type PostOrderResponse struct {
	ErrorMsg          *string         `json:"errorMsg"`
	MakingAmount      EmptyDecimal    `json:"makingAmount"`
	TakingAmount      EmptyDecimal    `json:"takingAmount"`
	OrderID           string          `json:"orderID"`
	Status            OrderStatusType `json:"status"`
	Success           bool            `json:"success"`
	TransactionHashes []B256          `json:"transactionHashes"`
	TradeIDs          []string        `json:"tradeIds"`
}

func (r *PostOrderResponse) UnmarshalJSON(b []byte) error {
	if err := validatePostOrderResponse(b); err != nil {
		return err
	}
	type plain PostOrderResponse
	var v struct {
		*plain
		TransactionsHashes []B256   `json:"transactionsHashes"`
		TradeIDsAlias      []string `json:"tradeIDs"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if p.TransactionHashes == nil {
		p.TransactionHashes = v.TransactionsHashes
	}
	if p.TradeIDs == nil {
		p.TradeIDs = v.TradeIDsAlias
	}
	if p.TransactionHashes == nil {
		p.TransactionHashes = []B256{}
	}
	if p.TradeIDs == nil {
		p.TradeIDs = []string{}
	}
	*r = PostOrderResponse(p)
	return nil
}

type OpenOrderResponse struct {
	ID              string          `json:"id"`
	Status          OrderStatusType `json:"status"`
	Owner           APIKey          `json:"owner"`
	MakerAddress    Address         `json:"maker_address"`
	Market          B256            `json:"market"`
	AssetID         U256            `json:"asset_id"`
	Side            Side            `json:"side"`
	OriginalSize    Decimal         `json:"original_size"`
	SizeMatched     Decimal         `json:"size_matched"`
	Price           Decimal         `json:"price"`
	AssociateTrades []string        `json:"associate_trades"`
	Outcome         string          `json:"outcome"`
	CreatedAt       UnixSeconds     `json:"created_at"`
	Expiration      UnixSeconds     `json:"expiration"`
	OrderType       OrderType       `json:"order_type"`
}

func (r *OpenOrderResponse) UnmarshalJSON(b []byte) error {
	if err := validateOpenOrderResponse(b); err != nil {
		return err
	}
	type plain OpenOrderResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.AssociateTrades == nil {
		v.AssociateTrades = []string{}
	}
	*r = OpenOrderResponse(v)
	return nil
}

type CancelOrdersResponse struct {
	Canceled    []string          `json:"canceled"`
	NotCanceled map[string]string `json:"notCanceled"`
}

func (r *CancelOrdersResponse) UnmarshalJSON(b []byte) error {
	type plain CancelOrdersResponse
	var v struct {
		*plain
		Alias map[string]string `json:"not_canceled"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if p.NotCanceled == nil {
		p.NotCanceled = v.Alias
	}
	if p.NotCanceled == nil {
		p.NotCanceled = map[string]string{}
	}
	if p.Canceled == nil {
		p.Canceled = []string{}
	}
	*r = CancelOrdersResponse(p)
	return nil
}

type MakerOrder struct {
	OrderID       string  `json:"order_id"`
	Owner         APIKey  `json:"owner"`
	MakerAddress  Address `json:"maker_address"`
	MatchedAmount Decimal `json:"matched_amount"`
	Price         Decimal `json:"price"`
	FeeRateBPS    Decimal `json:"fee_rate_bps"`
	AssetID       U256    `json:"asset_id"`
	Outcome       string  `json:"outcome"`
	Side          Side    `json:"side"`
}
type TradeResponse struct {
	ID              string          `json:"id"`
	TakerOrderID    string          `json:"taker_order_id"`
	Market          B256            `json:"market"`
	AssetID         U256            `json:"asset_id"`
	Side            Side            `json:"side"`
	Size            Decimal         `json:"size"`
	FeeRateBPS      Decimal         `json:"fee_rate_bps"`
	Price           Decimal         `json:"price"`
	Status          TradeStatusType `json:"status"`
	MatchTime       UnixSeconds     `json:"match_time"`
	LastUpdate      UnixSeconds     `json:"last_update"`
	Outcome         string          `json:"outcome"`
	BucketIndex     uint32          `json:"bucket_index"`
	Owner           APIKey          `json:"owner"`
	MakerAddress    Address         `json:"maker_address"`
	MakerOrders     []MakerOrder    `json:"maker_orders"`
	TransactionHash B256            `json:"transaction_hash"`
	TraderSide      TraderSide      `json:"trader_side"`
	ErrorMsg        *string         `json:"error_msg"`
}

func (r *TradeResponse) UnmarshalJSON(b []byte) error {
	if err := validateTradeResponse(b); err != nil {
		return err
	}
	type plain TradeResponse
	var v struct {
		*plain
		TransactionHash json.RawMessage `json:"transaction_hash"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v.TransactionHash) > 0 {
		var hash string
		if string(v.TransactionHash) == "null" {
			return fmt.Errorf("transaction_hash: expected string")
		}
		if err := json.Unmarshal(v.TransactionHash, &hash); err != nil {
			return fmt.Errorf("transaction_hash: %w", err)
		}
		p.TransactionHash = B256(hash)
	}
	if strings.TrimSpace(string(p.TransactionHash)) == "" {
		p.TransactionHash = B256("0x0000000000000000000000000000000000000000000000000000000000000000")
	} else if _, err := ParseB256(string(p.TransactionHash)); err != nil {
		return fmt.Errorf("transaction_hash: %w", err)
	}
	if p.MakerOrders == nil {
		p.MakerOrders = []MakerOrder{}
	}
	*r = TradeResponse(p)
	return nil
}

type BalanceAllowanceResponse struct {
	Balance    Decimal            `json:"balance"`
	Allowances map[Address]string `json:"allowances"`
}

func (r *BalanceAllowanceResponse) UnmarshalJSON(b []byte) error {
	if err := validateBalanceAllowanceResponse(b); err != nil {
		return err
	}
	type plain BalanceAllowanceResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Allowances == nil {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			return err
		}
		if string(fields["allowances"]) == "null" {
			return fmt.Errorf("allowances: expected map, got null")
		}
		v.Allowances = map[Address]string{}
	}
	*r = BalanceAllowanceResponse(v)
	return nil
}

type OrderScoringResponse struct {
	Scoring bool `json:"scoring"`
}
type OrdersScoringResponse map[string]bool
type PriceSideResponse struct {
	Side  Side    `json:"side"`
	Price Decimal `json:"price"`
}
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor"`
	Limit      uint64 `json:"limit"`
	Count      uint64 `json:"count"`
}

func (r *Page[T]) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "data", "next_cursor", "limit", "count")
	if err != nil {
		return err
	}
	// encoding/json silently accepts null for non-nullable values. Preserve it
	// for pointers, interfaces, and custom unmarshalers that handle it.
	t := reflect.TypeOf((*T)(nil)).Elem()
	unmarshaler := reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	if t.Kind() != reflect.Pointer && !reflect.PointerTo(t).Implements(unmarshaler) {
		switch t.Kind() {
		case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64,
			reflect.Struct, reflect.Array, reflect.Map, reflect.Slice:
			var entries []json.RawMessage
			if err := json.Unmarshal(fields["data"], &entries); err != nil {
				return err
			}
			for i, entry := range entries {
				if string(entry) == "null" {
					return fmt.Errorf("data[%d]: unexpected null", i)
				}
			}
		}
	}
	type plain Page[T]
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Page[T](v)
	return nil
}

type ReadonlyAPIKeyResponse struct {
	APIKey string `json:"apiKey"`
}
type HeartbeatResponse struct {
	HeartbeatID string  `json:"heartbeat_id"`
	Error       *string `json:"error"`
}

func (r *HeartbeatResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"error"}, "heartbeat_id")
	if err != nil {
		return err
	}
	if err := responseUUID(fields, "heartbeat_id"); err != nil {
		return err
	}
	type plain HeartbeatResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = HeartbeatResponse(v)
	return nil
}

type FeeInfo struct {
	Rate     Decimal
	Exponent uint32
}
type FeeDetails struct {
	Rate      Decimal `json:"r"`
	Exponent  uint32  `json:"e"`
	TakerOnly bool    `json:"to"`
}

func (r *FeeDetails) UnmarshalJSON(b []byte) error {
	if _, err := responsePresentFields(b, "r", "e", "to"); err != nil {
		return err
	}
	type plain FeeDetails
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = FeeDetails(v)
	return nil
}

type ClobToken struct {
	TokenID U256   `json:"t"`
	Outcome string `json:"o"`
}

func (r *ClobToken) UnmarshalJSON(b []byte) error {
	if _, err := responseFields(b, nil, "t", "o"); err != nil {
		return err
	}
	type plain ClobToken
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = ClobToken(v)
	return nil
}

type ClobMarketInfoResponse struct {
	ConditionID    B256         `json:"c"`
	Tokens         []*ClobToken `json:"t"`
	MinTickSize    TickSize     `json:"mts"`
	MinOrderSize   Decimal      `json:"mos"`
	NegRisk        bool         `json:"nr"`
	FeeDetails     *FeeDetails  `json:"fd"`
	MakerBaseFee   *Decimal     `json:"mbf"`
	TakerBaseFee   *Decimal     `json:"tbf"`
	RFQEnabled     bool         `json:"rfqe"`
	negRiskPresent bool
}

func (r *ClobMarketInfoResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFieldsWithCaseCheck(b, false, []string{"t", "mos", "nr", "fd", "mbf", "tbf", "rfqe"}, "c", "mts")
	if err != nil {
		return err
	}
	if err := validatePresentResponseFields(fields, nil, nil, "mos", "nr", "rfqe"); err != nil {
		return err
	}
	var condition string
	if err := json.Unmarshal(fields["c"], &condition); err != nil {
		return fmt.Errorf("c: %w", err)
	}
	if _, err := ParseB256(condition); err != nil {
		return fmt.Errorf("c: %w", err)
	}
	// Serde matches wire names exactly; do not let case variants override them.
	for key := range fields {
		switch key {
		case "c", "t", "mts", "mos", "nr", "fd", "mbf", "tbf", "rfqe":
		default:
			delete(fields, key)
		}
	}
	filtered, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	type plain ClobMarketInfoResponse
	var v struct {
		*plain
		NegRisk json.RawMessage `json:"nr"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(filtered, &v); err != nil {
		return err
	}
	if len(v.NegRisk) != 0 {
		if string(v.NegRisk) == "null" {
			return fmt.Errorf("nr: expected bool, got null")
		}
		if err := json.Unmarshal(v.NegRisk, &p.NegRisk); err != nil {
			return fmt.Errorf("nr: %w", err)
		}
		p.negRiskPresent = true
	}
	if v.Tokens == nil {
		v.Tokens = []*ClobToken{}
	}
	*r = ClobMarketInfoResponse(p)
	return nil
}

type BuilderFeeRateResponse struct {
	BuilderMakerFeeRateBPS uint32 `json:"builderMakerFeeRateBps"`
	BuilderTakerFeeRateBPS uint32 `json:"builderTakerFeeRateBps"`
}
