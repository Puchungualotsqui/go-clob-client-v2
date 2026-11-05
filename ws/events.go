package ws

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Event values retain decimals and timestamps as json.Number. IDs and enums use
// strings; unknown enum strings are retained. Typed validates Rust identifier
// formats and quoted i64 timestamps while Decode remains permissive. Optional
// scalar fields use pointers to distinguish zero from missing or null values.

type OrderBookLevel struct {
	Price json.Number `json:"price"`
	Size  json.Number `json:"size"`
}

type BookUpdate struct {
	AssetID   string           `json:"asset_id"`
	Market    string           `json:"market"`
	Timestamp json.Number      `json:"timestamp"`
	Bids      []OrderBookLevel `json:"bids"`
	Asks      []OrderBookLevel `json:"asks"`
	Hash      *string          `json:"hash"`
}

type PriceChange struct {
	Market       string                  `json:"market"`
	Timestamp    json.Number             `json:"timestamp"`
	PriceChanges []PriceChangeBatchEntry `json:"price_changes"`
}

type PriceChangeBatchEntry struct {
	AssetID string       `json:"asset_id"`
	Price   json.Number  `json:"price"`
	Size    *json.Number `json:"size"`
	Side    string       `json:"side"`
	Hash    *string      `json:"hash"`
	BestBid *json.Number `json:"best_bid"`
	BestAsk *json.Number `json:"best_ask"`
}

type TickSizeChange struct {
	AssetID     string      `json:"asset_id"`
	Market      string      `json:"market"`
	OldTickSize json.Number `json:"old_tick_size"`
	NewTickSize json.Number `json:"new_tick_size"`
	Timestamp   json.Number `json:"timestamp"`
}

type LastTradePrice struct {
	AssetID    string       `json:"asset_id"`
	Market     string       `json:"market"`
	Price      json.Number  `json:"price"`
	Side       *string      `json:"side"`
	Size       *json.Number `json:"size"`
	FeeRateBps *json.Number `json:"fee_rate_bps"`
	Timestamp  json.Number  `json:"timestamp"`
}

type BestBidAsk struct {
	Market    string      `json:"market"`
	AssetID   string      `json:"asset_id"`
	BestBid   json.Number `json:"best_bid"`
	BestAsk   json.Number `json:"best_ask"`
	Spread    json.Number `json:"spread"`
	Timestamp json.Number `json:"timestamp"`
}

type EventMessage struct {
	ID          string `json:"id"`
	Ticker      string `json:"ticker"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type NewMarket struct {
	ID           string        `json:"id"`
	Question     string        `json:"question"`
	Market       string        `json:"market"`
	Slug         string        `json:"slug"`
	Description  string        `json:"description"`
	AssetIDs     []string      `json:"assets_ids"`
	Outcomes     []string      `json:"outcomes"`
	EventMessage *EventMessage `json:"event_message"`
	Timestamp    json.Number   `json:"timestamp"`
}

// UnmarshalJSON accepts both spellings used by the CLOB API. If both are
// non-null, assets_ids takes precedence.
func (e *NewMarket) UnmarshalJSON(data []byte) error {
	type plain NewMarket
	var v struct {
		plain
		AssetIDsAlias []string `json:"asset_ids"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.AssetIDs == nil {
		v.AssetIDs = v.AssetIDsAlias
	}
	*e = NewMarket(v.plain)
	return nil
}

type MarketResolved struct {
	ID             string        `json:"id"`
	Question       *string       `json:"question"`
	Market         string        `json:"market"`
	Slug           *string       `json:"slug"`
	Description    *string       `json:"description"`
	AssetIDs       []string      `json:"assets_ids"`
	Outcomes       []string      `json:"outcomes"`
	WinningAssetID string        `json:"winning_asset_id"`
	WinningOutcome string        `json:"winning_outcome"`
	EventMessage   *EventMessage `json:"event_message"`
	Timestamp      json.Number   `json:"timestamp"`
}

func (e *MarketResolved) UnmarshalJSON(data []byte) error {
	type plain MarketResolved
	var v struct {
		plain
		AssetIDsAlias []string `json:"asset_ids"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.AssetIDs == nil {
		v.AssetIDs = v.AssetIDsAlias
	}
	*e = MarketResolved(v.plain)
	return nil
}

type MakerOrder struct {
	AssetID       string      `json:"asset_id"`
	MatchedAmount json.Number `json:"matched_amount"`
	OrderID       string      `json:"order_id"`
	Outcome       string      `json:"outcome"`
	Owner         string      `json:"owner"`
	Price         json.Number `json:"price"`
}

type TradeMessage struct {
	ID              string       `json:"id"`
	Market          string       `json:"market"`
	AssetID         string       `json:"asset_id"`
	Side            string       `json:"side"`
	Size            json.Number  `json:"size"`
	Price           json.Number  `json:"price"`
	Status          string       `json:"status"`
	Type            *string      `json:"type"`
	LastUpdate      *json.Number `json:"last_update"`
	Matchtime       *json.Number `json:"matchtime"`
	Timestamp       *json.Number `json:"timestamp"`
	Outcome         *string      `json:"outcome"`
	Owner           *string      `json:"owner"`
	TradeOwner      *string      `json:"trade_owner"`
	TakerOrderID    *string      `json:"taker_order_id"`
	MakerOrders     []MakerOrder `json:"maker_orders"`
	FeeRateBps      *json.Number `json:"fee_rate_bps"`
	TransactionHash *string      `json:"transaction_hash"`
	TraderSide      *string      `json:"trader_side"`
}

func (e *TradeMessage) UnmarshalJSON(data []byte) error {
	type plain TradeMessage
	var v struct {
		plain
		MatchTimeAlias *json.Number `json:"match_time"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Matchtime == nil {
		v.Matchtime = v.MatchTimeAlias
	}
	if v.TransactionHash != nil && *v.TransactionHash == "" {
		v.TransactionHash = nil
	}
	*e = TradeMessage(v.plain)
	return nil
}

type OrderMessage struct {
	ID              string       `json:"id"`
	Market          string       `json:"market"`
	AssetID         string       `json:"asset_id"`
	Side            string       `json:"side"`
	Price           json.Number  `json:"price"`
	Type            *string      `json:"type"`
	Outcome         *string      `json:"outcome"`
	Owner           *string      `json:"owner"`
	OrderOwner      *string      `json:"order_owner"`
	OriginalSize    *json.Number `json:"original_size"`
	SizeMatched     *json.Number `json:"size_matched"`
	Timestamp       *json.Number `json:"timestamp"`
	AssociateTrades []string     `json:"associate_trades"`
	Status          *string      `json:"status"`
}

type eventSchema struct {
	required       []string
	arrays         map[string]*eventSchema
	nullableArrays map[string]*eventSchema
	objects        map[string]*eventSchema
	numbers        []string
	assets         bool
}

var (
	levelSchema        = &eventSchema{required: []string{"price", "size"}, numbers: []string{"price", "size"}}
	changeSchema       = &eventSchema{required: []string{"asset_id", "price", "side"}, numbers: []string{"price", "size", "best_bid", "best_ask"}}
	eventMessageSchema = &eventSchema{required: []string{"id", "ticker", "slug", "title", "description"}}
	makerSchema        = &eventSchema{required: []string{"asset_id", "matched_amount", "order_id", "outcome", "owner", "price"}, numbers: []string{"matched_amount", "price"}}
	eventSchemas       = map[string]eventSchema{
		"book":             {required: []string{"asset_id", "market", "timestamp"}, arrays: map[string]*eventSchema{"bids": levelSchema, "asks": levelSchema}, numbers: []string{"timestamp"}},
		"price_change":     {required: []string{"market", "timestamp"}, arrays: map[string]*eventSchema{"price_changes": changeSchema}, numbers: []string{"timestamp"}},
		"tick_size_change": {required: []string{"asset_id", "market", "old_tick_size", "new_tick_size", "timestamp"}, numbers: []string{"old_tick_size", "new_tick_size", "timestamp"}},
		"last_trade_price": {required: []string{"asset_id", "market", "price", "timestamp"}, numbers: []string{"price", "size", "fee_rate_bps", "timestamp"}},
		"best_bid_ask":     {required: []string{"market", "asset_id", "best_bid", "best_ask", "spread", "timestamp"}, numbers: []string{"best_bid", "best_ask", "spread", "timestamp"}},
		"new_market":       {required: []string{"id", "question", "market", "slug", "description", "outcomes", "timestamp"}, arrays: map[string]*eventSchema{"assets_ids": nil, "asset_ids": nil, "outcomes": nil}, objects: map[string]*eventSchema{"event_message": eventMessageSchema}, numbers: []string{"timestamp"}, assets: true},
		"market_resolved":  {required: []string{"id", "market", "winning_asset_id", "winning_outcome", "timestamp"}, arrays: map[string]*eventSchema{"assets_ids": nil, "asset_ids": nil}, nullableArrays: map[string]*eventSchema{"outcomes": nil}, objects: map[string]*eventSchema{"event_message": eventMessageSchema}, numbers: []string{"timestamp"}, assets: true},
		"trade":            {required: []string{"id", "market", "asset_id", "side", "size", "price", "status"}, arrays: map[string]*eventSchema{"maker_orders": makerSchema}, numbers: []string{"size", "price", "last_update", "matchtime", "match_time", "timestamp", "fee_rate_bps"}},
		"order":            {required: []string{"id", "market", "asset_id", "side", "price"}, nullableArrays: map[string]*eventSchema{"associate_trades": nil}, numbers: []string{"price", "original_size", "size_matched", "timestamp"}},
	}
)

func validateEvent(eventType string, data json.RawMessage) error {
	schema := eventSchemas[eventType]
	return schema.validate(data)
}

func isNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func (schema *eventSchema) validate(data json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("expected object")
	}
	for _, key := range schema.required {
		if value, ok := fields[key]; !ok || isNull(value) {
			return fmt.Errorf("missing or null required field %q", key)
		}
	}
	if schema.assets && (len(fields["assets_ids"]) == 0 || isNull(fields["assets_ids"])) && (len(fields["asset_ids"]) == 0 || isNull(fields["asset_ids"])) {
		return fmt.Errorf("missing or null required field assets_ids")
	}
	for key, nested := range schema.nullableArrays {
		if value, ok := fields[key]; ok && !isNull(value) {
			if err := validateArray(key, value, nested); err != nil {
				return err
			}
		}
	}
	for key, nested := range schema.arrays {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if isNull(value) {
			if schema.assets && (key == "assets_ids" || key == "asset_ids") {
				continue
			}
			return fmt.Errorf("null array %q", key)
		}
		if err := validateArray(key, value, nested); err != nil {
			return err
		}
	}
	for key, nested := range schema.objects {
		if value, ok := fields[key]; ok && !isNull(value) {
			if err := nested.validate(value); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	for _, key := range schema.numbers {
		if value, ok := fields[key]; ok && !isNull(value) {
			var n json.Number
			if err := json.Unmarshal(value, &n); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if n == "" {
				return fmt.Errorf("%s: empty number", key)
			}
		}
	}
	return nil
}

func validateArray(key string, value json.RawMessage, nested *eventSchema) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(value, &elements); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	for i, element := range elements {
		if isNull(element) {
			return fmt.Errorf("%s[%d]: null element", key, i)
		}
		if nested != nil {
			if err := nested.validate(element); err != nil {
				return fmt.Errorf("%s[%d]: %w", key, i, err)
			}
		}
	}
	return nil
}

// canonicalEventJSON drops unknown fields before Go's case-insensitive struct
// decoder can mistake them for known fields. Like the Rust websocket parser's
// intermediate JSON Value, repeated identical keys use the last value; two
// distinct aliases for the same field are rejected by serde.
func canonicalEventJSON(data json.RawMessage, typ reflect.Type) (json.RawMessage, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid JSON")
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if isNull(data) {
		return data, nil
	}
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() != reflect.Uint8 {
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, err
		}
		for i := range items {
			item, err := canonicalEventJSON(items[i], typ.Elem())
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return json.Marshal(items)
	}
	if typ.Kind() != reflect.Struct {
		return data, nil
	}
	known := make(map[string]reflect.Type)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			known[name] = field.Type
		}
	}
	aliases := map[string]string{}
	switch typ {
	case reflect.TypeOf(NewMarket{}), reflect.TypeOf(MarketResolved{}):
		aliases["asset_ids"] = "assets_ids"
	case reflect.TypeOf(TradeMessage{}):
		aliases["match_time"] = "matchtime"
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("expected object")
	}
	fields := make(map[string]json.RawMessage)
	seen := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		name := key
		if alias, ok := aliases[key]; ok {
			name = alias
		}
		if _, ok := known[name]; !ok {
			continue
		}
		if previous, ok := seen[name]; ok && previous != key {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		seen[name] = key
		fields[name] = raw
	}
	for name, raw := range fields {
		raw, err := normalizeEventField(typ, name, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		value, err := canonicalEventJSON(raw, known[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		fields[name] = value
	}
	return json.Marshal(fields)
}

// normalizeEventField validates the non-string Rust wire types before decoding
// them into the public Go string fields. Unknown enum strings remain valid.
func normalizeEventField(typ reflect.Type, name string, raw json.RawMessage) (json.RawMessage, error) {
	if isNull(raw) {
		return raw, nil
	}
	if name == "asset_id" || (typ == reflect.TypeOf(MarketResolved{}) && name == "winning_asset_id") {
		return normalizeU256(raw)
	}
	if (typ == reflect.TypeOf(NewMarket{}) || typ == reflect.TypeOf(MarketResolved{})) && name == "assets_ids" {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for i, item := range items {
			var err error
			items[i], err = normalizeU256(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return json.Marshal(items)
	}
	if typ == reflect.TypeOf(TradeMessage{}) && name == "transaction_hash" {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("expected transaction hash string")
		}
		if s == "" {
			return raw, nil
		}
		return normalizeFixedBytes(raw, 32)
	}
	if name == "market" {
		return normalizeFixedBytes(raw, 32)
	}
	if (typ == reflect.TypeOf(MakerOrder{}) && name == "owner") ||
		(typ == reflect.TypeOf(TradeMessage{}) && (name == "owner" || name == "trade_owner")) ||
		(typ == reflect.TypeOf(OrderMessage{}) && (name == "owner" || name == "order_owner")) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		if _, err := uuid.Parse(s); err != nil {
			return nil, fmt.Errorf("invalid UUID")
		}
	}
	if name == "timestamp" || (typ == reflect.TypeOf(TradeMessage{}) && (name == "last_update" || name == "matchtime")) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("expected quoted i64 timestamp")
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid i64 timestamp")
		}
		return json.Marshal(strconv.FormatInt(n, 10))
	}
	return raw, nil
}

func normalizeU256(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
	} else {
		if _, err := strconv.ParseUint(string(raw), 10, 64); err != nil {
			return nil, fmt.Errorf("expected unsigned integer or U256 string")
		}
		s = string(raw)
	}
	if _, err := canonicalU256(s); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// canonicalU256 returns a decimal key for the string forms accepted by ruint.
func canonicalU256(s string) (string, error) {
	digits, base := s, 10
	if len(digits) >= 2 && digits[0] == '0' {
		switch digits[1] {
		case 'x', 'X':
			base, digits = 16, digits[2:]
		case 'o', 'O':
			base, digits = 8, digits[2:]
		case 'b', 'B':
			base, digits = 2, digits[2:]
		}
	}
	digits = strings.ReplaceAll(digits, "_", "")
	if digits != "" {
		for _, c := range digits {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return "", fmt.Errorf("invalid U256")
			}
		}
		n, ok := new(big.Int).SetString(digits, base)
		if !ok || n.BitLen() > 256 {
			return "", fmt.Errorf("invalid U256")
		}
		return n.String(), nil
	}
	return "0", nil
}

func normalizeFixedBytes(raw json.RawMessage, size int) (json.RawMessage, error) {
	var s string
	if len(raw) > 0 && raw[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil || len(values) != size {
			return nil, fmt.Errorf("expected %d bytes", size)
		}
		data := make([]byte, size)
		for i, value := range values {
			n, err := strconv.ParseUint(string(value), 10, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid byte at index %d", i)
			}
			data[i] = byte(n)
		}
		s = "0x" + hex.EncodeToString(data)
		return json.Marshal(s)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	hexValue := strings.TrimPrefix(s, "0x")
	if len(hexValue) != size*2 {
		return nil, fmt.Errorf("expected %d-byte hex string", size)
	}
	if _, err := hex.DecodeString(hexValue); err != nil {
		return nil, fmt.Errorf("invalid hex string")
	}
	return raw, nil
}

// Typed decodes a known event into a pointer to its corresponding event type.
// Unknown event types return an error; Raw remains available for future events.
// Unlike Decode, Typed checks required fields and rejects null for non-optional
// fields, including defaulted arrays and nested objects. Raw remains permissive.
func (m Message) Typed() (any, error) {
	var event any
	switch m.EventType {
	case "book":
		event = &BookUpdate{}
	case "price_change":
		event = &PriceChange{}
	case "tick_size_change":
		event = &TickSizeChange{}
	case "last_trade_price":
		event = &LastTradePrice{}
	case "best_bid_ask":
		event = &BestBidAsk{}
	case "new_market":
		event = &NewMarket{}
	case "market_resolved":
		event = &MarketResolved{}
	case "trade":
		event = &TradeMessage{}
	case "order":
		event = &OrderMessage{}
	default:
		return nil, fmt.Errorf("unknown websocket event type %q", m.EventType)
	}
	data, err := canonicalEventJSON(m.Raw, reflect.TypeOf(event))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.EventType, err)
	}
	if err := validateEvent(m.EventType, data); err != nil {
		return nil, fmt.Errorf("%s: %w", m.EventType, err)
	}
	if err := json.Unmarshal(data, event); err != nil {
		return nil, fmt.Errorf("%s: %w", m.EventType, err)
	}
	switch e := event.(type) {
	case *BookUpdate:
		if e.Bids == nil {
			e.Bids = []OrderBookLevel{}
		}
		if e.Asks == nil {
			e.Asks = []OrderBookLevel{}
		}
	case *PriceChange:
		if e.PriceChanges == nil {
			e.PriceChanges = []PriceChangeBatchEntry{}
		}
	case *MarketResolved:
		if e.Outcomes == nil {
			e.Outcomes = []string{}
		}
	case *TradeMessage:
		if e.MakerOrders == nil {
			e.MakerOrders = []MakerOrder{}
		}
	}
	return event, nil
}
