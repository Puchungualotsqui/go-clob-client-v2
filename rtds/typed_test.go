package rtds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testTypedSubscription[T any](t *testing.T, sub Subscription, eventType, payload string, want T, start func(*Client) (*Stream[T], error)) {
	t.Helper()
	requests := make(chan SubscriptionRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var req SubscriptionRequest
		if conn.ReadJSON(&req) != nil {
			return
		}
		requests <- req
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":`))
		batch := fmt.Sprintf(`[{"topic":"other","type":%q,"timestamp":1,"payload":%s},{"topic":%q,"type":"unexpected","timestamp":1,"payload":{}},{"topic":%q,"type":%q,"timestamp":1,"payload":{}},{"topic":%q,"type":%q,"timestamp":1,"payload":%s}]`, eventType, payload, sub.Topic, sub.Topic, eventType, sub.Topic, eventType, payload)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(batch))
		if conn.ReadJSON(&req) == nil {
			requests <- req
		}
	}))
	defer server.Close()
	client, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := start(client)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	select {
	case req := <-requests:
		if req.Action != "subscribe" || len(req.Subscriptions) != 1 || !reflect.DeepEqual(req.Subscriptions[0], sub) {
			t.Fatalf("unexpected subscription: %+v", req)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no subscribe request")
	}
	select {
	case err := <-stream.Errors:
		if err == nil || !strings.Contains(err.Error(), "websocket message") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("malformed envelope was not reported")
	}
	select {
	case got := <-stream.Messages:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no typed payload")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got, ok := <-stream.Messages:
		if ok {
			t.Fatalf("unexpected extra typed payload: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not close")
	}
	select {
	case req := <-requests:
		wantSub := Subscription{Topic: sub.Topic, Type: sub.Type}
		if req.Action != "unsubscribe" || len(req.Subscriptions) != 1 || !reflect.DeepEqual(req.Subscriptions[0], wantSub) {
			t.Fatalf("unexpected unsubscribe: %+v", req)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no unsubscribe request")
	}
}

func TestTypedSubscriptions(t *testing.T) {
	t.Run("binance", func(t *testing.T) {
		symbols := []string{"btcusdt"}
		testTypedSubscription(t, CryptoPrices(symbols), "update", `{"symbol":"btcusdt","timestamp":2,"value":123.123456789012345678}`, Price{Symbol: "btcusdt", Timestamp: 2, Value: json.Number("123.123456789012345678")}, func(c *Client) (*Stream[CryptoPrice], error) {
			return c.SubscribeTypedCryptoPrices(context.Background(), symbols)
		})
	})
	t.Run("chainlink", func(t *testing.T) {
		symbol := "btc/usd"
		testTypedSubscription(t, ChainlinkPrices(&symbol), "update", `{"symbol":"btc/usd","timestamp":2,"value":42}`, Price{Symbol: "btc/usd", Timestamp: 2, Value: json.Number("42")}, func(c *Client) (*Stream[ChainlinkPrice], error) {
			return c.SubscribeTypedChainlinkPrices(context.Background(), &symbol)
		})
	})
	t.Run("comments with auth", func(t *testing.T) {
		auth := &Credentials{Key: "key", Secret: "secret", Passphrase: "pass"}
		sub := Comments(CommentCreated)
		sub.ClobAuth = auth
		created, err := time.Parse(time.RFC3339, "2025-07-25T14:49:35Z")
		if err != nil {
			t.Fatal(err)
		}
		payload := `{"id":"1","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":3,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"alice"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677"}`
		want := Comment{ID: "1", Body: "hi", CreatedAt: created, ParentEntityID: 3, ParentEntityType: "Event", Profile: CommentProfile{BaseAddress: "0xce533188d53a16ed580fd5121dedf166d3482677", Name: "alice"}, UserAddress: "0xce533188d53a16ed580fd5121dedf166d3482677"}
		testTypedSubscription(t, sub, CommentCreated, payload, want, func(c *Client) (*Stream[Comment], error) {
			return c.SubscribeTypedComments(context.Background(), CommentCreated, auth)
		})
	})
	t.Run("comments wildcard without auth", func(t *testing.T) {
		created, err := time.Parse(time.RFC3339, "2025-07-25T14:49:35Z")
		if err != nil {
			t.Fatal(err)
		}
		payload := `{"id":"2","body":"hi","createdAt":"2025-07-25T14:49:35Z","parentEntityID":3,"parentEntityType":"Event","profile":{"baseAddress":"0xce533188d53a16ed580fd5121dedf166d3482677","name":"alice"},"userAddress":"0xce533188d53a16ed580fd5121dedf166d3482677"}`
		want := Comment{ID: "2", Body: "hi", CreatedAt: created, ParentEntityID: 3, ParentEntityType: "Event", Profile: CommentProfile{BaseAddress: "0xce533188d53a16ed580fd5121dedf166d3482677", Name: "alice"}, UserAddress: "0xce533188d53a16ed580fd5121dedf166d3482677"}
		testTypedSubscription(t, Comments(""), ReactionRemoved, payload, want, func(c *Client) (*Stream[Comment], error) {
			return c.SubscribeTypedComments(context.Background(), "", nil)
		})
	})
}

func TestTypedSubscriptionContextValidation(t *testing.T) {
	client, err := NewClient("", Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SubscribeTypedCryptoPrices(nil, nil); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.SubscribeTypedChainlinkPrices(ctx, nil); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	if _, err := client.SubscribeTypedComments(ctx, "", nil); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
