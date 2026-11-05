package rtds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWireFilters(t *testing.T) {
	symbol := "btc/usd"
	binance := CryptoPrices([]string{"btcusdt", "ethusdt"})
	chainlink := ChainlinkPrices(&symbol)
	chainlink.ClobAuth = &Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	data, err := json.Marshal(SubscriptionRequest{Action: "subscribe", Subscriptions: []Subscription{binance, chainlink, Comments(CommentCreated)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"filters":["btcusdt","ethusdt"]`, `"filters":"{\"symbol\":\"btc/usd\"}"`, `"clob_auth":{"key":"k","secret":"s","passphrase":"p"}`, `"type":"comment_created"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s in %s", want, data)
		}
	}
	filter := "not json"
	data, _ = json.Marshal(Subscription{Topic: "custom", Type: "*", Filters: &filter})
	if !strings.Contains(string(data), `"filters":"not json"`) {
		t.Fatal(string(data))
	}
}

func TestSubscriptionFilterRoundTrip(t *testing.T) {
	for _, input := range []string{
		`{"topic":"crypto_prices","type":"update","filters":["btc","eth"]}`,
		`{"topic":"crypto_prices_chainlink","type":"*","filters":"{\"symbol\":\"btc/usd\"}"}`,
		`{"topic":"custom","type":"*","filters":"123"}`,
		`{"topic":"custom","type":"*","filters":null}`,
		`{"topic":"comments","type":"*"}`,
	} {
		var sub Subscription
		if err := json.Unmarshal([]byte(input), &sub); err != nil {
			t.Fatal(err)
		}
		output, err := json.Marshal(sub)
		if err != nil {
			t.Fatal(err)
		}
		var before, after any
		if err := json.Unmarshal([]byte(input), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(output, &after); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("round trip: %s -> %s", input, output)
		}
	}
	var sub Subscription
	if err := json.Unmarshal([]byte(`{"topic":"crypto_prices_chainlink","type":"*","filters":{}}`), &sub); err == nil {
		t.Fatal("expected invalid chainlink filter error")
	}
}

func TestMessageEnvelopeAndPayloadHelpers(t *testing.T) {
	for _, input := range []string{`null`, `{}`, `{"topic":"x","type":"y","timestamp":null,"payload":{}}`, `[{"topic":"x"}]`} {
		if _, err := ParseMessages([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	msgs, err := ParseMessages([]byte(`{"topic":"crypto_prices","type":"update","timestamp":1,"payload":{"symbol":"btc","timestamp":2,"value":123.123456789012345678}}`))
	if err != nil {
		t.Fatal(err)
	}
	price, ok := msgs[0].AsCryptoPrice()
	if !ok || price.Value.String() != "123.123456789012345678" {
		t.Fatalf("%+v %v", price, ok)
	}
	if _, ok := msgs[0].AsChainlinkPrice(); ok {
		t.Fatal("accepted wrong topic")
	}
	msgs[0].Payload = json.RawMessage(`{}`)
	if _, ok := msgs[0].AsCryptoPrice(); ok {
		t.Fatal("accepted missing price fields")
	}
	msgs[0].Topic = "comments"
	msgs[0].Payload = json.RawMessage(`{"id":"1","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":3,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"alice"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677"}`)
	comment, ok := msgs[0].AsComment()
	if !ok || comment.ID != "1" || comment.Profile.Name != "alice" {
		t.Fatalf("%+v %v", comment, ok)
	}
}

func TestParse(t *testing.T) {
	messages, err := ParseMessages([]byte(`[{"topic":"crypto_prices","type":"update","timestamp":1,"payload":{"symbol":"btcusdt","timestamp":2,"value":123.123456789012345678}}]`))
	if err != nil || len(messages) != 1 {
		t.Fatalf("%v %v", messages, err)
	}
	var price Price
	if err := messages[0].DecodePayload(&price); err != nil || price.Value.String() != "123.123456789012345678" {
		t.Fatalf("%+v %v", price, err)
	}
	messages, err = ParseMessages([]byte(" \n"))
	if err != nil || len(messages) != 0 {
		t.Fatalf("%v %v", messages, err)
	}
}

func TestSubscribeReconnectAndUnsubscribe(t *testing.T) {
	requests := make(chan SubscriptionRequest, 8)
	upgrader := websocket.Upgrader{}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var req SubscriptionRequest
		if conn.ReadJSON(&req) != nil {
			return
		}
		requests <- req
		if connections.Add(1) == 1 {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`[{"topic":"other","type":"update","timestamp":1,"payload":{}},{"topic":"crypto_prices_chainlink","type":"update","timestamp":2,"payload":{"value":1}}]`))
			return
		}
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if string(data) == "PING" {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("PONG"))
				continue
			}
			req = SubscriptionRequest{}
			if json.Unmarshal(data, &req) == nil {
				requests <- req
				return
			}
		}
	}))
	defer server.Close()
	client, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{InitialBackoff: time.Millisecond, HeartbeatInterval: 10 * time.Millisecond, HeartbeatTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	symbol := "btc/usd"
	stream, err := client.SubscribeChainlinkPrices(context.Background(), &symbol)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	select {
	case msg := <-stream.Messages:
		if msg.Topic != "crypto_prices_chainlink" {
			t.Fatal(msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
	for i := 0; i < 2; i++ {
		select {
		case req := <-requests:
			if req.Action != "subscribe" || len(req.Subscriptions) != 1 || req.Subscriptions[0].Topic != "crypto_prices_chainlink" || req.Subscriptions[0].Filters == nil || *req.Subscriptions[0].Filters != `{"symbol":"btc/usd"}` {
				t.Fatalf("%+v", req)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no replay")
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-requests:
		if req.Action != "unsubscribe" || req.Subscriptions[0].Type != "*" {
			t.Fatalf("%+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("no unsubscribe")
	}
}

func TestCancelAndFailedDial(t *testing.T) {
	client, _ := NewClient("ws://127.0.0.1:1", Config{MaxAttempts: 1})
	stream, err := client.SubscribeCryptoPrices(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-stream.Errors:
		if err == nil {
			t.Fatal("expected dial error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no dial error")
	}
	select {
	case <-stream.Messages:
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatedReplayPreservesWire(t *testing.T) {
	requests := make(chan string, 3)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		requests <- string(data)
		if count.Add(1) == 1 {
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{InitialBackoff: time.Millisecond})
	symbol := "btc/usd"
	sub := ChainlinkPrices(&symbol)
	sub.ClobAuth = &Credentials{Key: "k", Secret: "s", Passphrase: "p"}
	stream, err := client.Subscribe(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var first string
	for i := 0; i < 2; i++ {
		select {
		case request := <-requests:
			if !strings.Contains(request, `"clob_auth":{"key":"k","secret":"s","passphrase":"p"}`) || !strings.Contains(request, `"filters":"{\"symbol\":\"btc/usd\"}"`) {
				t.Fatalf("bad request %s", request)
			}
			if i == 0 {
				first = request
			} else if first != request {
				t.Fatalf("replay changed: %s", request)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing subscription")
		}
	}
}
