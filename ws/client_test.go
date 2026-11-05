package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMarketReconnectAndClose(t *testing.T) {
	requests := make(chan SubscriptionRequest, 8)
	paths := make(chan string, 4)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		paths <- r.URL.Path
		var req SubscriptionRequest
		if conn.ReadJSON(&req) != nil {
			return
		}
		requests <- req
		if len(paths) == 1 {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`[{"event_type":"book","asset_id":"other"},{"event_type":"book","asset_id":"123","bids":[]}]`))
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
	client, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/user", Config{InitialBackoff: time.Millisecond, HeartbeatInterval: 10 * time.Millisecond, HeartbeatTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.SubscribeMarket(context.Background(), []string{"123"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	select {
	case msg := <-stream.Messages:
		if msg.EventType != "book" || !strings.Contains(string(msg.Raw), `"123"`) {
			t.Fatalf("unexpected message: %+v", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
	for i := 0; i < 2; i++ {
		select {
		case req := <-requests:
			if req.Operation != "subscribe" || len(req.AssetIDs) != 1 || req.AssetIDs[0] != "123" || req.CustomFeatureEnabled == nil || !*req.CustomFeatureEnabled || req.InitialDump == nil || !*req.InitialDump {
				t.Fatalf("bad request: %+v", req)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no subscription replay")
		}
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-requests:
		if req.Operation != "unsubscribe" || req.InitialDump != nil {
			t.Fatalf("bad unsubscribe: %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("no unsubscribe")
	}
	for i := 0; i < 2; i++ {
		if p := <-paths; p != "/ws/market" {
			t.Fatalf("path %q", p)
		}
	}
}

func TestUserAuthAndParsers(t *testing.T) {
	cred := Credentials{Key: "key", Secret: "secret", Passphrase: "pass"}
	yes := true
	data, err := json.Marshal(SubscriptionRequest{Type: "user", Operation: "subscribe", Markets: []string{}, AssetIDs: []string{}, InitialDump: &yes, Auth: &cred})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"auth":{"apiKey":"key","secret":"secret","passphrase":"pass"}`) {
		t.Fatal(string(data))
	}
	messages, err := userParser([]byte(`[{"event_type":"trade","price":"0.10"},{"event_type":"book"}]`))
	if err != nil || len(messages) != 1 || messages[0].EventType != "trade" {
		t.Fatalf("%v %v", messages, err)
	}
	messages, err = marketParser([]string{"2"})([]byte(`[{"event_type":"price_change","price_changes":[{"asset_id":"2"}]},{"event_type":"new_market","assets_ids":["2"]}]`))
	if err != nil || len(messages) != 2 {
		t.Fatalf("%v %v", messages, err)
	}
}

func TestHeartbeatTimeoutReconnect(t *testing.T) {
	connections := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connections <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{HeartbeatInterval: 5 * time.Millisecond, HeartbeatTimeout: 10 * time.Millisecond, InitialBackoff: time.Millisecond})
	stream, err := client.SubscribeMarket(context.Background(), []string{"1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for i := 0; i < 2; i++ {
		select {
		case <-connections:
		case <-time.After(3 * time.Second):
			t.Fatal("heartbeat did not reconnect")
		}
	}
}

func TestContextCancellationStopsStream(t *testing.T) {
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		close(connected)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{})
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.SubscribeUser(ctx, nil, Credentials{Key: "k", Secret: "s", Passphrase: "p"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("not connected")
	}
	cancel()
	select {
	case _, ok := <-stream.Messages:
		if ok {
			t.Fatal("unexpected message")
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not stop")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSlowConsumerStopsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`[{"event_type":"book","asset_id":"1"},{"event_type":"book","asset_id":"1"}]`))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	client, _ := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{BufferSize: 1})
	stream, err := client.SubscribeMarket(context.Background(), []string{"1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	select {
	case err := <-stream.Errors:
		if err != errSlowConsumer {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("no overflow error")
	}
	if _, ok := <-stream.Messages; !ok {
		t.Fatal("missing buffered message")
	}
	if _, ok := <-stream.Messages; ok {
		t.Fatal("stream should be closed")
	}
}

func TestMidpoint(t *testing.T) {
	messages, err := ParseMessages([]byte(`{"event_type":"book","asset_id":"1","market":"0xabc","timestamp":"123456789","bids":[{"price":"0.123456789123456789"}],"asks":[{"price":"0.223456789123456789"}]}`))
	if err != nil || len(messages) != 1 {
		t.Fatalf("%v %v", messages, err)
	}
	midpoint, ok, err := messages[0].Midpoint()
	if err != nil || !ok || midpoint.Midpoint.FloatString(18) != "0.173456789123456789" || midpoint.Timestamp.String() != "123456789" {
		t.Fatalf("%+v %v %v", midpoint, ok, err)
	}
}
