// Package ws streams CLOB market and authenticated user events.
package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultEndpoint = "wss://ws-subscriptions-clob.polymarket.com"

// Credentials are sent in plaintext inside the WebSocket protocol. Use wss in production.
type Credentials struct {
	Key        string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

func (c Credentials) String() string   { return "Credentials{[REDACTED]}" }
func (c Credentials) GoString() string { return c.String() }

// SubscriptionRequest is the CLOB wire request. IDs are decimal asset IDs or
// hex market IDs, represented as strings to avoid losing precision.
type SubscriptionRequest struct {
	Type                 string       `json:"type"`
	Operation            string       `json:"operation,omitempty"`
	Markets              []string     `json:"markets"`
	AssetIDs             []string     `json:"assets_ids"`
	InitialDump          *bool        `json:"initial_dump,omitempty"`
	CustomFeatureEnabled *bool        `json:"custom_feature_enabled,omitempty"`
	Auth                 *Credentials `json:"auth,omitempty"`
}

// Message retains the complete event JSON so callers can decode event-specific
// fields without losing precision or forward-compatible fields. Metadata is set
// only on stream-delivered messages, not by ParseMessages.
type Message struct {
	EventType string
	Raw       json.RawMessage
	// ConnectionID identifies the physical connection within this process, until
	// the uint64 counter wraps. It is not stable across process restarts.
	ConnectionID uint64 `json:"-"`
	// Generation is the group's successful full-subscription-write generation.
	Generation uint64 `json:"-"`
	// ReceivedAt is the local time the complete WebSocket message was read,
	// before parsing or routing. Events in one batch share the same timestamp.
	ReceivedAt time.Time `json:"-"`
}

// Decode unmarshals the complete event into an event-specific value.
func (m Message) Decode(v any) error { return json.Unmarshal(m.Raw, v) }

// Client shares market connections and user connections with identical credentials.
type Client struct {
	endpoint string
	config   Config
	mu       sync.Mutex
	groups   map[groupKey]*connectionGroup
}

// NewClient accepts the base URL or a URL ending in /ws, /ws/market, or /ws/user.
// An empty endpoint selects DefaultEndpoint. It does not initiate a connection.
func NewClient(endpoint string, config Config) (*Client, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid CLOB WebSocket base URL")
	}
	base := strings.TrimRight(endpoint, "/")
	for _, suffix := range []string{"/ws/market", "/ws/user", "/ws"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimSuffix(base, suffix)
			break
		}
	}
	return &Client{endpoint: base, config: config}, nil
}

// SubscribeMarket subscribes to asset IDs. Valid U256 identifiers are sent and
// matched in decimal form; other nonempty IDs retain exact-string matching for
// compatibility. customFeatures enables best bid/ask, new market and market
// resolved events. Close unsubscribes the entire stream.
func (c *Client) SubscribeMarket(ctx context.Context, assetIDs []string, customFeatures bool) (*Stream[Message], error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if len(assetIDs) == 0 {
		return nil, errors.New("at least one asset ID is required")
	}
	ids := append([]string(nil), assetIDs...)
	for i, id := range ids {
		if id == "" {
			return nil, errors.New("empty asset ID")
		}
		if canonical, err := canonicalU256(id); err == nil {
			ids[i] = canonical
		}
	}
	return c.subscribe(ctx, groupKey{channel: "market"}, ids, customFeatures)
}

// SubscribeUser subscribes to order and trade events. An empty markets slice
// subscribes to all markets. Valid B256 identifiers are sent and matched as
// lowercase 0x-prefixed hex; other nonempty IDs match exactly for compatibility.
// Credentials must be protected in transit and logs.
func (c *Client) SubscribeUser(ctx context.Context, markets []string, credentials Credentials) (*Stream[Message], error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if credentials.Key == "" || credentials.Secret == "" || credentials.Passphrase == "" {
		return nil, errors.New("user subscription requires credentials")
	}
	ids := append([]string{}, markets...)
	for i, id := range ids {
		if id == "" {
			return nil, errors.New("empty market ID")
		}
		encoded, _ := json.Marshal(id)
		if canonical, err := routingMarketID(encoded); err == nil {
			ids[i] = canonical
		}
	}
	return c.subscribe(ctx, groupKey{channel: "user", credentials: credentials}, ids, false)
}

// ParseMessages accepts a CLOB object or batch. Objects without event_type are ignored.
// For a syntactically valid batch, malformed elements are reported in the error
// while valid messages are returned. Invalid JSON rejects the entire input.
func ParseMessages(data []byte) ([]Message, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	var values []json.RawMessage
	if data[0] == '[' {
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, err
		}
	} else {
		values = []json.RawMessage{data}
	}
	var messages []Message
	var parseErr error
	for i, raw := range values {
		var h struct {
			EventType string `json:"event_type"`
		}
		err := json.Unmarshal(raw, &h)
		if err == nil && isNull(raw) {
			err = errors.New("expected event object, got null")
		}
		if err != nil {
			if data[0] == '[' {
				err = fmt.Errorf("batch element %d: %w", i, err)
			}
			parseErr = errors.Join(parseErr, err)
			continue
		}
		if h.EventType != "" {
			messages = append(messages, Message{EventType: h.EventType, Raw: append(json.RawMessage(nil), raw...)})
		}
	}
	return messages, parseErr
}

func userParser(data []byte) ([]Message, error) {
	messages, err := ParseMessages(data)
	out := messages[:0]
	for _, m := range messages {
		if m.EventType == "order" || m.EventType == "trade" {
			out = append(out, m)
		}
	}
	return out, err
}

// Routing keeps opaque legacy string IDs working while comparing valid typed
// identifiers by value. Non-string malformed wire identifiers are errors.
func routingAssetID(raw json.RawMessage) (string, error) {
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		if id, err := canonicalU256(s); err == nil {
			return id, nil
		}
		return s, nil
	}
	normalized, err := normalizeU256(raw)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(normalized, &s); err != nil {
		return "", err
	}
	return canonicalU256(s)
}

func routingMarketID(raw json.RawMessage) (string, error) {
	normalized, err := normalizeFixedBytes(raw, 32)
	if err != nil {
		var s string
		if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
			return s, nil
		}
		return "", err
	}
	var s string
	if err := json.Unmarshal(normalized, &s); err != nil {
		return "", err
	}
	return "0x" + strings.ToLower(strings.TrimPrefix(s, "0x")), nil
}

func knownMarketEvent(eventType string) bool {
	switch eventType {
	case "book", "price_change", "tick_size_change", "last_trade_price", "best_bid_ask", "new_market", "market_resolved":
		return true
	default:
		return false
	}
}

// marketRoutingIDs is shared by strict validation and permissive routing so
// that validation cannot approve a frame that the router would reject.
func marketRoutingIDs(m Message, strict bool) ([]string, error) {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(m.Raw, &event); err != nil {
		return nil, fmt.Errorf("%s: %w", m.EventType, err)
	}
	rawIDs := []json.RawMessage{event["asset_id"]}
	for _, key := range []string{"assets_ids", "asset_ids"} {
		var list []json.RawMessage
		if raw := event[key]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &list); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", m.EventType, key, err)
			}
		}
		if strict {
			for _, raw := range list {
				if isNull(raw) {
					return nil, fmt.Errorf("%s: %s: null asset ID", m.EventType, key)
				}
			}
		}
		rawIDs = append(rawIDs, list...)
	}
	var changes []map[string]json.RawMessage
	if raw := event["price_changes"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &changes); err != nil {
			return nil, fmt.Errorf("%s: price_changes: %w", m.EventType, err)
		}
	}
	for _, change := range changes {
		raw := change["asset_id"]
		if strict && (len(raw) == 0 || isNull(raw)) {
			return nil, fmt.Errorf("%s: price_changes entry missing asset_id", m.EventType)
		}
		rawIDs = append(rawIDs, raw)
	}
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		if len(raw) == 0 || isNull(raw) {
			continue
		}
		if strict && len(raw) > 0 && raw[0] == '"' {
			var text string
			if err := json.Unmarshal(raw, &text); err == nil && text == "" {
				return nil, fmt.Errorf("%s: empty asset ID", m.EventType)
			}
		}
		id, err := routingAssetID(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: asset_id: %w", m.EventType, err)
		}
		if strict && id == "" {
			return nil, fmt.Errorf("%s: empty asset ID", m.EventType)
		}
		ids = append(ids, id)
	}
	if strict && len(ids) == 0 {
		return nil, fmt.Errorf("%s: no routing asset ID", m.EventType)
	}
	return ids, nil
}

// strictMarketMessages validates the entire WebSocket message before routing.
func strictMarketMessages(data []byte) ([]Message, error) {
	data = bytes.Trim(data, " \t\r\n")
	if len(data) == 0 {
		return nil, errors.New("empty market message")
	}
	if err := validateStrictJSON(data); err != nil {
		return nil, err
	}
	messages, err := ParseMessages(data)
	if err != nil {
		return nil, err
	}
	count := 1
	if data[0] == '[' {
		var elements []json.RawMessage
		if err := json.Unmarshal(data, &elements); err != nil {
			return nil, err
		}
		count = len(elements)
	}
	if len(messages) != count {
		return nil, errors.New("market event missing event_type")
	}
	for i, m := range messages {
		if !knownMarketEvent(m.EventType) {
			return nil, fmt.Errorf("market event %d: unknown event_type", i)
		}
		if _, err := marketRoutingIDs(m, true); err != nil {
			return nil, fmt.Errorf("market event %d: %w", i, err)
		}
	}
	return messages, nil
}

func marketParser(ids []string) func([]byte) ([]Message, error) {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		if canonical, err := canonicalU256(id); err == nil {
			id = canonical
		}
		set[id] = true
	}
	return func(data []byte) ([]Message, error) {
		messages, err := ParseMessages(data)
		if err != nil {
			return nil, err
		}
		out := messages[:0]
		for _, m := range messages {
			if !knownMarketEvent(m.EventType) {
				continue
			}
			ids, err := marketRoutingIDs(m, false)
			if err != nil {
				return nil, err
			}
			matched := false
			for _, id := range ids {
				matched = matched || set[id]
			}
			if matched {
				out = append(out, m)
			}
		}
		return out, nil
	}
}
