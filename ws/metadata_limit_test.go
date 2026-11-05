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

func nextMessage(t *testing.T, s *Stream[Message]) Message {
	t.Helper()
	select {
	case msg, ok := <-s.Messages:
		if !ok {
			t.Fatal("message stream closed")
		}
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
		return Message{}
	}
}

func TestDeliveredMetadataAcrossConnections(t *testing.T) {
	h, c := newWSHarness(t)
	c.config.BufferSize = 8
	a := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 1, false)
	b := subscribeMarket(t, c, []string{"a"}, false)
	before := time.Now()
	p.send(t, `[{"event_type":"book","asset_id":"a","id":1},{"event_type":"book","asset_id":"a","id":2}]`)
	first := nextMessage(t, a)
	second := nextMessage(t, a)
	shared := nextMessage(t, b)
	shared2 := nextMessage(t, b)
	after := time.Now()
	if first.ConnectionID == 0 || first.Generation != 1 || first.ReceivedAt.Before(before) || first.ReceivedAt.After(after) {
		t.Fatalf("invalid metadata: %+v", first)
	}
	for _, msg := range []Message{second, shared, shared2} {
		if msg.ConnectionID != first.ConnectionID || msg.Generation != first.Generation || msg.ReceivedAt != first.ReceivedAt {
			t.Fatalf("batch/recipient metadata differs: %+v vs %+v", msg, first)
		}
	}
	first.Raw[0] = 'x'
	if shared.Raw[0] == 'x' {
		t.Fatal("recipients share Raw storage")
	}

	// Leave an old message buffered while the connection is replaced.
	p.send(t, `{"event_type":"book","asset_id":"a","id":"old"}`)
	oldA := nextMessage(t, a) // guarantees that the same frame reached b
	if err := p.conn.Close(); err != nil {
		t.Fatal(err)
	}
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	replay(t, a, 2, true)
	p.send(t, `{"event_type":"book","asset_id":"a","id":"new"}`)
	oldB := nextMessage(t, b)
	fresh := nextMessage(t, b)
	if !strings.Contains(string(oldB.Raw), `"old"`) || oldB.ConnectionID != first.ConnectionID || oldB.ReceivedAt != oldA.ReceivedAt || oldB.Generation != 1 {
		t.Fatalf("old buffered message lost metadata: %+v", oldB)
	}
	if !strings.Contains(string(fresh.Raw), `"new"`) || fresh.ConnectionID == 0 || fresh.ConnectionID == first.ConnectionID || fresh.Generation != 2 {
		t.Fatalf("reconnected message metadata: %+v", fresh)
	}

	// Different groups and clients can have generation 1 simultaneously.
	user := subscribeUser(t, c, nil, Credentials{Key: "k", Secret: "s", Passphrase: "p"})
	up := h.peer(t)
	up.request(t, "subscribe")
	up.send(t, `{"event_type":"order","id":"user"}`)
	userMsg := nextMessage(t, user)
	if userMsg.ConnectionID == 0 || userMsg.ConnectionID == first.ConnectionID || userMsg.ConnectionID == fresh.ConnectionID || userMsg.Generation != 1 || userMsg.ReceivedAt.IsZero() {
		t.Fatalf("user metadata: %+v", userMsg)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	newGroup := subscribeMarket(t, c, []string{"a"}, false)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	groupMsg := nextMessage(t, newGroup)
	if groupMsg.Generation != 1 || groupMsg.ConnectionID == 0 || groupMsg.ConnectionID == fresh.ConnectionID || groupMsg.ConnectionID == first.ConnectionID || groupMsg.ConnectionID == userMsg.ConnectionID {
		t.Fatalf("new group metadata: %+v", groupMsg)
	}
	other, err := NewClient(c.endpoint, c.config)
	if err != nil {
		t.Fatal(err)
	}
	otherStream := subscribeMarket(t, other, []string{"a"}, false)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	otherMsg := nextMessage(t, otherStream)
	if otherMsg.Generation != 1 || otherMsg.ConnectionID == 0 || otherMsg.ConnectionID == groupMsg.ConnectionID || otherMsg.ConnectionID == userMsg.ConnectionID {
		t.Fatalf("other client metadata: %+v", otherMsg)
	}
}

func TestParseMessagesMetadataIsZeroAndNotMarshaled(t *testing.T) {
	messages, err := ParseMessages([]byte(`{"event_type":"book","asset_id":"a"}`))
	if err != nil || len(messages) != 1 {
		t.Fatalf("parse: %v %v", messages, err)
	}
	m := messages[0]
	if m.ConnectionID != 0 || m.Generation != 0 || !m.ReceivedAt.IsZero() {
		t.Fatalf("metadata: %+v", m)
	}
	m.ConnectionID, m.Generation, m.ReceivedAt = 1, 2, time.Now()
	encoded, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "ConnectionID") || strings.Contains(string(encoded), "ReceivedAt") || strings.Contains(string(encoded), "Generation") {
		t.Fatalf("metadata serialized: %s", encoded)
	}
}

func TestReadLimitAndReconnect(t *testing.T) {
	if (Config{}).defaults().ReadLimit != DefaultReadLimit || (Config{ReadLimit: -1}).defaults().ReadLimit != DefaultReadLimit {
		t.Fatal("default read limit not applied")
	}
	for _, mode := range []string{"text", "binary", "fragmented"} {
		t.Run(mode, func(t *testing.T) {
			h, c := newWSHarness(t)
			valid := `{"event_type":"book","asset_id":"a"}`
			limit := 5000
			c.config.ReadLimit = int64(limit)
			s := subscribeMarket(t, c, []string{"a"}, false)
			p := h.peer(t)
			p.request(t, "subscribe", "a")
			// Whitespace makes this a valid message exactly at the limit.
			p.send(t, valid+strings.Repeat(" ", limit-len(valid)))
			first := nextMessage(t, s)
			kind := websocket.TextMessage
			if mode == "binary" {
				kind = websocket.BinaryMessage
			}
			if mode == "fragmented" {
				writer, err := p.conn.NextWriter(kind)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write([]byte(strings.Repeat("x", limit+1))); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err := p.conn.WriteMessage(kind, []byte(strings.Repeat("x", limit+1))); err != nil {
				t.Fatal(err)
			}
			requireStreamError(t, s, websocket.ErrReadLimit)
			p = h.peer(t)
			p.request(t, "subscribe", "a")
			p.send(t, valid)
			fresh := nextMessage(t, s)
			if fresh.ConnectionID == first.ConnectionID || fresh.Generation != first.Generation+1 {
				t.Fatalf("reconnect metadata: %+v then %+v", first, fresh)
			}
			// The configured limit is reapplied to every replacement socket.
			if err := p.conn.WriteMessage(websocket.BinaryMessage, []byte(strings.Repeat("x", limit+1))); err != nil {
				t.Fatal(err)
			}
			requireStreamError(t, s, websocket.ErrReadLimit)
			p = h.peer(t)
			p.request(t, "subscribe", "a")
			p.send(t, valid)
			third := nextMessage(t, s)
			if third.Generation != fresh.Generation+1 || third.ConnectionID == fresh.ConnectionID {
				t.Fatalf("second reconnect metadata: %+v then %+v", fresh, third)
			}
		})
	}
}

func TestDefaultReadLimitRejectsOversizedMessage(t *testing.T) {
	h, c := newWSHarness(t)
	s := subscribeMarket(t, c, []string{"a"}, false)
	p := h.peer(t)
	p.request(t, "subscribe", "a")
	// The peer may stop accepting the payload as soon as it sees its length.
	_ = p.conn.WriteMessage(websocket.BinaryMessage, make([]byte, DefaultReadLimit+1))
	requireStreamError(t, s, websocket.ErrReadLimit)
	p = h.peer(t)
	p.request(t, "subscribe", "a")
	p.send(t, `{"event_type":"book","asset_id":"a"}`)
	if msg := nextMessage(t, s); msg.Generation != 2 {
		t.Fatalf("message after default limit failure: %+v", msg)
	}
}

func TestReadLimitCompressedPayload(t *testing.T) {
	peers := make(chan *websocket.Conn, 2)
	closes := make(chan error, 2)
	upgrader := websocket.Upgrader{EnableCompression: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		peers <- conn
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				closes <- err
				return
			}
		}
	}))
	defer server.Close()
	dialer := *websocket.DefaultDialer
	dialer.EnableCompression = true
	c, err := NewClient("ws"+strings.TrimPrefix(server.URL, "http"), Config{Dialer: &dialer, ReadLimit: 128, InitialBackoff: time.Millisecond, HeartbeatInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.SubscribeMarket(context.Background(), []string{"a"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var p *websocket.Conn
	select {
	case p = <-peers:
	case <-time.After(3 * time.Second):
		t.Fatal("no connection")
	}
	p.EnableWriteCompression(true)
	payload := `{"event_type":"book","asset_id":"a","padding":"` + strings.Repeat("a", 512) + `"}`
	if err := p.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	requireStreamError(t, s, websocket.ErrReadLimit)
	select {
	case err := <-closes:
		if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
			t.Fatalf("expected close code 1009, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no close frame")
	}
	select {
	case p = <-peers:
	case <-time.After(3 * time.Second):
		t.Fatal("no reconnect")
	}
	if err := p.WriteMessage(websocket.TextMessage, []byte(`{"event_type":"book","asset_id":"a"}`)); err != nil {
		t.Fatal(err)
	}
	msg := nextMessage(t, s)
	if msg.Generation != 2 || msg.ConnectionID == 0 {
		t.Fatalf("message after compressed failure: %+v", msg)
	}
}
