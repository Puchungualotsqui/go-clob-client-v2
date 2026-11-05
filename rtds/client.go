// Package rtds streams Polymarket real-time data socket topics.
package rtds

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultEndpoint = "wss://ws-live-data.polymarket.com"

// Credentials are sent in plaintext inside the WebSocket protocol. Use wss in production.
type Credentials struct {
	Key        string `json:"key"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

func (c Credentials) String() string   { return "Credentials{[REDACTED]}" }
func (c Credentials) GoString() string { return c.String() }

// Subscription describes one RTDS topic. Filters contains JSON text or a
// literal string. Chainlink filters are always sent as a JSON string; for other
// topics valid JSON is sent as a JSON value, matching the Rust client.
type Subscription struct {
	Topic    string       `json:"topic"`
	Type     string       `json:"type"`
	Filters  *string      `json:"-"`
	ClobAuth *Credentials `json:"clob_auth,omitempty"`
}

func (s Subscription) MarshalJSON() ([]byte, error) {
	type wire struct {
		Topic    string       `json:"topic"`
		Type     string       `json:"type"`
		Filters  any          `json:"filters,omitempty"`
		ClobAuth *Credentials `json:"clob_auth,omitempty"`
	}
	w := wire{Topic: s.Topic, Type: s.Type, ClobAuth: s.ClobAuth}
	if s.Filters != nil {
		if s.Topic != "crypto_prices_chainlink" && json.Valid([]byte(*s.Filters)) {
			w.Filters = json.RawMessage(*s.Filters)
		} else {
			w.Filters = *s.Filters
		}
	}
	return json.Marshal(w)
}

// UnmarshalJSON preserves filters in the representation expected by MarshalJSON.
func (s *Subscription) UnmarshalJSON(data []byte) error {
	var w struct {
		Topic    string          `json:"topic"`
		Type     string          `json:"type"`
		Filters  json.RawMessage `json:"filters"`
		ClobAuth *Credentials    `json:"clob_auth"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	decoded := Subscription{Topic: w.Topic, Type: w.Type, ClobAuth: w.ClobAuth}
	if w.Filters != nil {
		filter := string(w.Filters)
		if w.Topic == "crypto_prices_chainlink" {
			if bytes.Equal(bytes.TrimSpace(w.Filters), []byte("null")) {
				return errors.New("chainlink filters must be a string")
			}
			if err := json.Unmarshal(w.Filters, &filter); err != nil {
				return err
			}
		}
		decoded.Filters = &filter
	}
	*s = decoded
	return nil
}

type SubscriptionRequest struct {
	Action        string         `json:"action"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// CryptoPrices subscribes to Binance symbols; nil means all symbols.
func CryptoPrices(symbols []string) Subscription {
	s := Subscription{Topic: "crypto_prices", Type: "update"}
	if symbols != nil {
		data, _ := json.Marshal(symbols)
		filter := string(data)
		s.Filters = &filter
	}
	return s
}

// ChainlinkPrices subscribes to a Chainlink symbol; nil means all symbols.
func ChainlinkPrices(symbol *string) Subscription {
	s := Subscription{Topic: "crypto_prices_chainlink", Type: "*"}
	if symbol != nil {
		data, _ := json.Marshal(struct {
			Symbol string `json:"symbol"`
		}{*symbol})
		filter := string(data)
		s.Filters = &filter
	}
	return s
}

// Comments subscribes to a comment event type, or all types when empty.
func Comments(eventType string) Subscription {
	if eventType == "" {
		eventType = "*"
	}
	return Subscription{Topic: "comments", Type: eventType}
}

const (
	CommentCreated  = "comment_created"
	CommentRemoved  = "comment_removed"
	ReactionCreated = "reaction_created"
	ReactionRemoved = "reaction_removed"
)

// Message wraps the raw payload, preserving decimal precision and unknown fields.
type Message struct {
	Topic     string          `json:"topic"`
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// UnmarshalJSON requires the same envelope fields as the Rust RTDS message.
func (m *Message) UnmarshalJSON(data []byte) error {
	data, err := exactObject(data, true, "topic", "type", "timestamp", "payload")
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"topic", "type", "timestamp", "payload"} {
		value, ok := fields[key]
		if !ok || (key != "payload" && bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return errors.New("missing or null RTDS message field: " + key)
		}
	}
	type message Message
	var decoded message
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Message(decoded)
	return nil
}

func (m Message) DecodePayload(v any) error { return json.Unmarshal(m.Payload, v) }

// AsCryptoPrice, AsChainlinkPrice and AsComment decode only matching topics.
// A false result indicates a different topic or an invalid payload.
func (m Message) AsCryptoPrice() (CryptoPrice, bool) {
	var price CryptoPrice
	return priceForTopic(m, "crypto_prices", &price)
}

func (m Message) AsChainlinkPrice() (ChainlinkPrice, bool) {
	var price ChainlinkPrice
	return priceForTopic(m, "crypto_prices_chainlink", &price)
}

func priceForTopic(m Message, topic string, price *Price) (Price, bool) {
	data, err := exactObject(m.Payload, false, "symbol", "timestamp", "value")
	if m.Topic != topic || err != nil || !hasFields(data, "symbol", "timestamp", "value") || json.Unmarshal(data, price) != nil || price.Value == "" {
		return Price{}, false
	}
	return *price, true
}

func (m Message) AsComment() (Comment, bool) {
	var comment Comment
	if m.Topic != "comments" {
		return comment, false
	}
	data, err := exactObject(m.Payload, false, "id", "body", "createdAt", "parentCommentID", "parentEntityID", "parentEntityType", "profile", "reactionCount", "replyAddress", "reportCount", "userAddress")
	if err != nil || !hasFields(data, "id", "body", "createdAt", "parentEntityID", "parentEntityType", "profile", "userAddress") || !nonNullIfPresent(data, "reactionCount", "reportCount") {
		return comment, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return comment, false
	}
	profile, err := exactObject(fields["profile"], false, "baseAddress", "displayUsernamePublic", "name", "proxyWallet", "pseudonym")
	if err != nil || !hasFields(profile, "baseAddress", "name") || !nonNullIfPresent(profile, "displayUsernamePublic") {
		return comment, false
	}
	var profileFields map[string]json.RawMessage
	if json.Unmarshal(profile, &profileFields) != nil {
		return Comment{}, false
	}
	for _, field := range []struct {
		fields map[string]json.RawMessage
		name   string
	}{
		{fields, "userAddress"}, {fields, "replyAddress"},
		{profileFields, "baseAddress"}, {profileFields, "proxyWallet"},
	} {
		if value, ok := field.fields[field.name]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			address, err := normalizeAddress(value)
			if err != nil {
				return Comment{}, false
			}
			field.fields[field.name] = address
		}
	}
	fields["profile"], err = json.Marshal(profileFields)
	if err != nil {
		return Comment{}, false
	}
	data, err = json.Marshal(fields)
	if err != nil || json.Unmarshal(data, &comment) != nil {
		return Comment{}, false
	}
	return comment, true
}

// normalizeAddress accepts the string and byte-array forms of alloy's Address.
func normalizeAddress(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > 0 && raw[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil || len(values) != 20 {
			return nil, errors.New("expected 20 address bytes")
		}
		data := make([]byte, 20)
		for i, value := range values {
			n, err := strconv.ParseUint(string(value), 10, 8)
			if err != nil {
				return nil, errors.New("invalid address byte")
			}
			data[i] = byte(n)
		}
		return json.Marshal("0x" + hex.EncodeToString(data))
	}
	var address string
	if err := json.Unmarshal(raw, &address); err != nil {
		return nil, err
	}
	value := strings.TrimPrefix(address, "0x")
	if len(value) != 40 {
		return nil, errors.New("expected 20-byte address")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return nil, errors.New("invalid address hex")
	}
	return raw, nil
}

// exactObject matches field names exactly. The envelope is decoded directly by
// serde, while typed payloads first pass through a JSON Value (last key wins).
func exactObject(data json.RawMessage, rejectDuplicates bool, names ...string) (json.RawMessage, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid JSON")
	}
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if !known[key] {
			continue
		}
		if _, ok := fields[key]; ok && rejectDuplicates {
			return nil, errors.New("duplicate field: " + key)
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func hasFields(data json.RawMessage, keys ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	for _, key := range keys {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

func nonNullIfPresent(data json.RawMessage, keys ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return false
	}
	for _, key := range keys {
		if value, ok := fields[key]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return true
}

// Price uses json.Number to preserve the exact price rather than converting to float64.
type Price struct {
	Symbol    string      `json:"symbol"`
	Timestamp int64       `json:"timestamp"`
	Value     json.Number `json:"value"`
}

// CryptoPrice and ChainlinkPrice share the same wire fields.
type CryptoPrice = Price
type ChainlinkPrice = Price

type Comment struct {
	ID               string         `json:"id"`
	Body             string         `json:"body"`
	CreatedAt        time.Time      `json:"createdAt"`
	ParentCommentID  *string        `json:"parentCommentID"`
	ParentEntityID   int64          `json:"parentEntityID"`
	ParentEntityType string         `json:"parentEntityType"`
	Profile          CommentProfile `json:"profile"`
	ReactionCount    int64          `json:"reactionCount"`
	ReplyAddress     *string        `json:"replyAddress"`
	ReportCount      int64          `json:"reportCount"`
	UserAddress      string         `json:"userAddress"`
}
type CommentProfile struct {
	BaseAddress           string  `json:"baseAddress"`
	DisplayUsernamePublic bool    `json:"displayUsernamePublic"`
	Name                  string  `json:"name"`
	ProxyWallet           *string `json:"proxyWallet"`
	Pseudonym             *string `json:"pseudonym"`
}

// ParseMessages parses a single message or an array; whitespace is a keepalive.
func ParseMessages(data []byte) ([]Message, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var messages []Message
		err := json.Unmarshal(data, &messages)
		return messages, err
	}
	var message Message
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, err
	}
	return []Message{message}, nil
}

// Client multiplexes compatible subscription streams over shared connections.
type Client struct {
	endpoint string
	config   Config
	mu       sync.Mutex
	groups   map[*connection]struct{}
}

// NewClient uses DefaultEndpoint if endpoint is empty. It connects on subscription.
func NewClient(endpoint string, config Config) (*Client, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.Fragment != "" {
		return nil, errors.New("invalid RTDS WebSocket URL")
	}
	return &Client{endpoint: endpoint, config: config}, nil
}

// Subscribe starts a stream for one topic/type. Closing the final matching stream
// sends unsubscribe when connected. Filters and credentials are snapshotted for replay.
func (c *Client) Subscribe(ctx context.Context, sub Subscription) (*Stream[Message], error) {
	return subscribe(c, ctx, sub, func(m Message) (Message, bool) { return m, true })
}

func subscribe[T any](c *Client, ctx context.Context, sub Subscription, decode func(Message) (T, bool)) (*Stream[T], error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sub.Topic == "" || sub.Type == "" {
		return nil, errors.New("topic and type are required")
	}
	data, err := json.Marshal(SubscriptionRequest{Action: "subscribe", Subscriptions: []Subscription{sub}})
	if err != nil {
		return nil, err
	}
	remove, err := json.Marshal(SubscriptionRequest{Action: "unsubscribe", Subscriptions: []Subscription{{Topic: sub.Topic, Type: sub.Type}}})
	if err != nil {
		return nil, err
	}
	var auth authIdentity
	if sub.ClobAuth != nil {
		auth.present = true
		auth.credentials = *sub.ClobAuth
	}
	return newStream(c, ctx, sub.Topic, sub.Type, auth, data, remove, decode), nil
}

func (c *Client) SubscribeCryptoPrices(ctx context.Context, symbols []string) (*Stream[Message], error) {
	return c.Subscribe(ctx, CryptoPrices(symbols))
}
func (c *Client) SubscribeChainlinkPrices(ctx context.Context, symbol *string) (*Stream[Message], error) {
	return c.Subscribe(ctx, ChainlinkPrices(symbol))
}
func (c *Client) SubscribeComments(ctx context.Context, eventType string, auth *Credentials) (*Stream[Message], error) {
	s := Comments(eventType)
	s.ClobAuth = auth
	return c.Subscribe(ctx, s)
}

// SubscribeTypedCryptoPrices streams decoded Binance prices. Invalid payloads are
// skipped; malformed message envelopes are reported on the stream's Errors channel.
func (c *Client) SubscribeTypedCryptoPrices(ctx context.Context, symbols []string) (*Stream[CryptoPrice], error) {
	return subscribe(c, ctx, CryptoPrices(symbols), Message.AsCryptoPrice)
}

// SubscribeTypedChainlinkPrices streams decoded Chainlink prices. Invalid
// payloads are skipped; malformed envelopes are reported on Errors.
func (c *Client) SubscribeTypedChainlinkPrices(ctx context.Context, symbol *string) (*Stream[ChainlinkPrice], error) {
	return subscribe(c, ctx, ChainlinkPrices(symbol), Message.AsChainlinkPrice)
}

// SubscribeTypedComments streams decoded comments, optionally with credentials.
// Invalid payloads are skipped; malformed envelopes are reported on Errors.
func (c *Client) SubscribeTypedComments(ctx context.Context, eventType string, auth *Credentials) (*Stream[Comment], error) {
	s := Comments(eventType)
	s.ClobAuth = auth
	return subscribe(c, ctx, s, Message.AsComment)
}
