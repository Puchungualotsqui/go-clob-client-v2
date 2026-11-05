package rtds

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMessageEnvelopeCompatibility(t *testing.T) {
	valid := map[string]json.RawMessage{"topic": json.RawMessage(`"crypto_prices"`), "type": json.RawMessage(`"update"`), "timestamp": json.RawMessage(`0`), "payload": json.RawMessage(`null`)}
	for _, key := range []string{"topic", "type", "timestamp", "payload"} {
		original := valid[key]
		delete(valid, key)
		raw, _ := json.Marshal(valid)
		if _, err := ParseMessages(raw); err == nil {
			t.Errorf("missing %s accepted", key)
		}
		valid[key] = json.RawMessage(`null`)
		raw, _ = json.Marshal(valid)
		_, err := ParseMessages(raw)
		if (err == nil) != (key == "payload") {
			t.Errorf("null %s: %v", key, err)
		}
		valid[key] = original
	}
	for _, raw := range []string{`{"topic":"x","type":"y","timestamp":"1","payload":{}}`, `null`, `[null]`} {
		if _, err := ParseMessages([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestTypedPayloadCompatibility(t *testing.T) {
	for _, topic := range []string{"crypto_prices", "crypto_prices_chainlink"} {
		for _, tc := range []struct {
			payload string
			ok      bool
		}{
			{`{"symbol":"x","timestamp":0,"value":0}`, true},
			{`{"symbol":"x","timestamp":0,"value":"1.234567890123456789"}`, true},
			{`{"symbol":null,"timestamp":0,"value":1}`, false},
			{`{"symbol":"x","timestamp":null,"value":1}`, false},
			{`{"symbol":"x","timestamp":0,"value":null}`, false},
			{`{"symbol":"x","timestamp":0,"value":""}`, false},
			{`{"symbol":"x","timestamp":0}`, false},
		} {
			m := Message{Topic: topic, Payload: json.RawMessage(tc.payload)}
			var p Price
			var ok bool
			if topic == "crypto_prices" {
				p, ok = m.AsCryptoPrice()
				if _, other := m.AsChainlinkPrice(); other {
					t.Error("wrong topic accepted")
				}
			} else {
				p, ok = m.AsChainlinkPrice()
			}
			if ok != tc.ok {
				t.Errorf("%s %s: got %+v %v", topic, tc.payload, p, ok)
			}
		}
	}
	base := `{"id":"1","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":0,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"n"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677"}`
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &payload); err != nil {
		t.Fatal(err)
	}
	check := func() bool {
		raw, _ := json.Marshal(payload)
		_, ok := (Message{Topic: "comments", Payload: raw}).AsComment()
		return ok
	}
	if !check() {
		t.Fatal("valid comment rejected")
	}
	for _, key := range []string{"id", "body", "createdAt", "parentEntityID", "parentEntityType", "profile", "userAddress"} {
		old := payload[key]
		delete(payload, key)
		if check() {
			t.Errorf("missing %s accepted", key)
		}
		payload[key] = json.RawMessage(`null`)
		if check() {
			t.Errorf("null %s accepted", key)
		}
		payload[key] = old
	}
	for _, key := range []string{"reactionCount", "reportCount"} {
		payload[key] = json.RawMessage(`null`)
		if check() {
			t.Errorf("null %s accepted", key)
		}
		delete(payload, key)
	}
	payload["profile"] = json.RawMessage(`{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"n","displayUsernamePublic":null}`)
	if check() {
		t.Error("null displayUsernamePublic accepted")
	}
	payload["profile"] = json.RawMessage(`{"baseAddress":null,"name":"n"}`)
	if check() {
		t.Error("null profile address accepted")
	}
}

func TestTypedCommentAddresses(t *testing.T) {
	address := "0xce533188d53a16ed580fd5121dedf166d3482677"
	profile := map[string]any{"baseAddress": address, "name": "alice"}
	body := map[string]any{"id": "1", "body": "hi", "createdAt": "2025-07-25T14:49:35Z", "parentEntityID": 1, "parentEntityType": "Event", "profile": profile, "userAddress": address}
	check := func(fields map[string]any, key string, value any, ok bool) {
		t.Helper()
		old, had := fields[key]
		fields[key] = value
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		_, got := (Message{Topic: "comments", Payload: raw}).AsComment()
		if got != ok {
			t.Errorf("%s=%v: got %v", key, value, got)
		}
		if had {
			fields[key] = old
		} else {
			delete(fields, key)
		}
	}
	addressBytes := make([]int, 20)
	addressBytes[0] = 1
	body["userAddress"] = addressBytes
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	comment, ok := (Message{Topic: "comments", Payload: raw}).AsComment()
	if !ok || comment.UserAddress != "0x01"+strings.Repeat("00", 19) {
		t.Fatalf("byte array address: %+v, %v", comment, ok)
	}
	body["userAddress"] = address
	for _, field := range []struct {
		fields   map[string]any
		key      string
		optional bool
	}{
		{body, "userAddress", false}, {body, "replyAddress", true},
		{profile, "baseAddress", false}, {profile, "proxyWallet", true},
	} {
		for _, v := range []any{address, strings.TrimPrefix(address, "0x"), strings.ToUpper(strings.TrimPrefix(address, "0x")), make([]int, 20)} {
			check(field.fields, field.key, v, true)
		}
		for _, v := range []any{"", "0x1", "0x" + strings.Repeat("zz", 20), make([]int, 19), make([]int, 21), 1, true} {
			check(field.fields, field.key, v, false)
		}
		check(field.fields, field.key, nil, field.optional)
		badBytes := make([]any, 20)
		badBytes[0] = nil
		check(field.fields, field.key, badBytes, false)
	}
}

func TestSubscriptionWireCompatibility(t *testing.T) {
	symbol := "eth/usd"
	request := SubscriptionRequest{Action: "subscribe", Subscriptions: []Subscription{CryptoPrices([]string{"btcusdt"}), ChainlinkPrices(&symbol), Comments(CommentCreated)}}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"filters":["btcusdt"]`, `"filters":"{\"symbol\":\"eth/usd\"}"`, `"type":"comment_created"`} {
		if !strings.Contains(string(raw), part) {
			t.Errorf("missing %s in %s", part, raw)
		}
	}
	var got SubscriptionRequest
	if err := json.Unmarshal(raw, &got); err != nil || !reflect.DeepEqual(got, request) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	s := ChainlinkPrices(&symbol)
	before := s
	if err := json.Unmarshal([]byte(`{"topic":"crypto_prices_chainlink","type":"*","filters":null}`), &s); err == nil || !reflect.DeepEqual(s, before) {
		t.Fatalf("invalid decode changed value: %+v, %v", s, err)
	}
}
