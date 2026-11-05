package rtds

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRetirementCompletesQueuedClosesWhileDecoderBlocked(t *testing.T) {
	for _, final := range []bool{false, true} {
		name := "remaining stream"
		if final {
			name = "final stream"
		}
		t.Run(name, func(t *testing.T) {
			server := newSocketServer(t)
			dialer := *websocket.DefaultDialer
			var dials atomic.Int32
			wrapped := make(chan *blockedWriteConn, 1)
			dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
				if err != nil || dials.Add(1) != 1 {
					return conn, err
				}
				blocked := &blockedWriteConn{Conn: conn, entered: make(chan struct{}), release: make(chan struct{})}
				wrapped <- blocked
				return blocked, nil
			}
			client := server.client(t, Config{Dialer: &dialer, InitialBackoff: time.Millisecond})
			decodeEntered := make(chan struct{})
			decodeRelease := make(chan struct{})
			decodeReturned := make(chan struct{})
			var decodeOnce, releaseDecodeOnce sync.Once
			releaseDecode := func() { releaseDecodeOnce.Do(func() { close(decodeRelease) }) }
			slow, err := subscribe(client, context.Background(), Subscription{Topic: "slow", Type: "*"}, func(msg Message) (Message, bool) {
				decodeOnce.Do(func() { close(decodeEntered) })
				<-decodeRelease
				close(decodeReturned)
				return msg, true
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { releaseDecode(); _ = slow.Close() }()
			first := server.event(t, "subscribe", "slow")
			awaitRTDSState(t, slow, PhaseReady, 1, 0)
			target, err := client.Subscribe(context.Background(), Subscription{Topic: "target", Type: "*"})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { releaseDecode(); _ = target.Close() }()
			server.event(t, "subscribe", "target")
			if err := first.conn.WriteMessage(websocket.TextMessage, []byte(`{"topic":"slow","type":"x","timestamp":1,"payload":{}}`)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-decodeEntered:
			case <-time.After(3 * time.Second):
				t.Fatal("decoder did not block")
			}
			blocked := receive(t, wrapped)
			var releaseWriteOnce sync.Once
			releaseWrite := func() { releaseWriteOnce.Do(func() { close(blocked.release) }) }
			defer releaseWrite()
			blocked.block.Store(true)
			trigger, err := client.Subscribe(context.Background(), Subscription{Topic: "trigger", Type: "*"})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { releaseWrite(); releaseDecode(); _ = trigger.Close() }()
			select {
			case <-blocked.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("writer did not block")
			}
			client.mu.Lock()
			group := slow.group
			session := group.session
			client.mu.Unlock()
			closeResults := make([]<-chan error, 0, 3)
			startClose := func(stream *Stream[Message]) {
				result := make(chan error, 1)
				closeResults = append(closeResults, result)
				go func() { result <- stream.Close() }()
				select {
				case <-stream.done:
				case <-time.After(3 * time.Second):
					t.Fatal("stream did not begin closing")
				}
			}
			startClose(trigger)
			startClose(target)
			if final {
				startClose(slow)
			}
			client.mu.Lock()
			pending := append([]*socketWrite(nil), session.pending...)
			client.mu.Unlock()
			if len(pending) != len(closeResults) {
				t.Fatalf("queued writes = %d, want %d", len(pending), len(closeResults))
			}
			if err := session.conn.Close(); err != nil {
				t.Fatal(err)
			}
			releaseWrite()
			for _, result := range closeResults {
				if err := receive(t, result); !errors.Is(err, errWriteNotSent) {
					t.Fatalf("queued close error = %v", err)
				}
			}
			select {
			case <-decodeReturned:
				t.Fatal("decoder returned before queued closes")
			default:
			}
			select {
			case <-session.done:
			case <-time.After(3 * time.Second):
				t.Fatal("writer did not stop")
			}
			client.mu.Lock()
			retired, remaining := session.retired, len(session.pending)
			client.mu.Unlock()
			if !retired || remaining != 0 {
				t.Fatalf("retired = %v, pending = %d", retired, remaining)
			}
			if final {
				select {
				case <-group.ctx.Done():
				default:
					t.Fatal("final retired group was not canceled")
				}
			} else {
				awaitRTDSState(t, slow, PhaseDisconnected, 1, 1)
			}
			releaseDecode()
			select {
			case <-session.stop:
			case <-time.After(3 * time.Second):
				t.Fatal("reader cleanup did not finish")
			}
			for _, write := range pending {
				select {
				case err := <-write.result:
					t.Fatalf("write completed more than once: %v", err)
				default:
				}
			}
			if !final {
				server.event(t, "subscribe", "slow")
				awaitRTDSState(t, slow, PhaseReady, 2, 1)
			}
		})
	}
}
