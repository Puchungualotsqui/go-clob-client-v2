# CLOB WebSocket

This package uses `github.com/gorilla/websocket`, included in the parent module.

`NewClient` accepts a base endpoint (empty selects the production endpoint). Streams created by the same client share one market connection, and one user connection per distinct set of credentials. Connections retry failures, replay active subscriptions after reconnecting, and use text `PING`/`PONG` heartbeats. Valid U256 asset IDs are sent and matched as decimal strings; valid B256 market IDs are sent and matched as lowercase `0x`-prefixed hex strings. Equivalent representations share a subscription. Other nonempty string IDs remain unchanged and match exactly for compatibility. Wire asset IDs may also be unsigned JSON integers up to `uint64`; wire market IDs may also be arrays of 32 bytes. Malformed non-string IDs are reported on `Errors` and the event is not delivered to that stream. In a syntactically valid batch, malformed event headers are reported without suppressing the other events; malformed routing IDs reject only their own event. Invalid JSON rejects the entire frame. `ParseMessages` likewise returns valid batch messages alongside an error for malformed elements, so callers can inspect both results. An empty user market list subscribes to all markets.

IDs are reference-counted per connection (including duplicate IDs in a single call): closing or canceling a stream releases only its interests. When the last reference to an ID is released, an unsubscribe is sent if connected (unless a wildcard user subscription covers it). Released IDs are remembered for the lifetime of that connection. Reusing one forces a reconnect before adding it to the wire subscription, so frames in flight on the retired connection cannot reach its replacement subscriber. To bound this history, releasing 4096 distinct IDs on one connection also forces a reconnect. The last stream closes the shared connection. `Close` is safe to call concurrently; cancellation also releases the stream, but neither operation guarantees the server received an unsubscribe if the connection fails. Market events are routed by asset ID, including IDs in price-change batches and market lifecycle events. Like Rust, a `price_change` event is delivered **whole** if any entry matches: a batch can contain entries for assets the recipient did not subscribe to, so filter entries when applying updates. User events are routed by their `market` field; a user event without a market is delivered only to wildcard streams. Each recipient owns its `Raw` bytes.

Wildcard user streams share the connection with explicit-market streams. Entering wildcard mode while explicit markets are active, or leaving it while explicit streams remain, reconnects and replays the desired subscription. This avoids relying on an empty unsubscribe having a particular server-side meaning. Such transitions can interrupt delivery. Different credentials never share a connection, even when their API keys match.

```go
client, err := ws.NewClient("", ws.Config{})
if err != nil { /* handle error */ }
stream, err := client.SubscribeMarket(ctx, []string{"123"}, true)
if err != nil { /* handle error */ }
defer stream.Close()
for msg := range stream.Messages {
    // msg.EventType identifies the event; msg.Raw preserves the entire JSON.
    // msg.Decode(&event) decodes it into an application-specific struct.
    // msg.Typed() returns a pointer to the matching package event type.
    // msg.Midpoint() computes the exact midpoint for book events.
}
```

Import this package as `github.com/puchungualotsqui/go-clob-client-v2/ws`; the example assumes `ctx` is a `context.Context`. Drain `Messages` promptly: a full buffer stops only that stream and reports an error rather than silently dropping data. `Errors` is a bounded diagnostic channel; both channels close when the stream ends. A production consumer should monitor both channels. Routine diagnostics are best-effort. Important lifecycle errors take priority over older queued diagnostics when the buffer is full; callers must still drain `Errors` because later lifecycle errors can replace earlier ones. Connection and parse errors may be reported to multiple streams. Active streams receive `ws.ErrReconnecting` when ID reuse, retirement-history limits, a wildcard transition, or a custom-feature upgrade intentionally replaces a connection; use `errors.Is` and reconcile state. Network and write failures are also reported, but do not imply a successful replay. `MaxAttempts` limits consecutive failed connection attempts (zero means unlimited); successful connections reset the counter. Exhausting retries closes all streams on that connection with an error matching `ws.ErrRetriesExhausted`. Retry delay doubles up to `MaxBackoff`.

### Recovery state

Use `stream.State()` as the authoritative **local, current** lifecycle snapshot and `stream.StateChanges` as a coalescing wakeup. `StreamState` has a per-stream `Version`, a group `Generation` (successful full subscription writes), a group `Interruptions` count (retired connections while subscribers remained), and a `Phase`:

- `PhaseConnecting`: no connection has yet been retired or successfully subscribed for this group.
- `PhaseReplaySent`: the current connection's full subscription request was written. This includes the initial connection; streams joining an active connection see the current group state.
- `PhaseInterrupted`: a connection was retired and no replacement full subscription has been written yet. `Generation` is the last successful write and `Interruptions` has advanced.
- `PhaseClosed`: this stream has stopped; its final snapshot remains available.

Each state transition advances `Version`; a failed dial without an established connection does not by itself change the phase or counters (dial errors are reported on `Errors`). On reconnect, `PhaseReplaySent` has a new `Generation` and retains the interruption count, correlating the replay with the latest observed interruption. The counter also exposes interruptions that happened while wakeups were coalesced. Counters belong to the shared group and reset when its last stream closes and a new group is created. A stream that joins a group sees its current counters, not necessarily zero.

`StateChanges` has one buffered wakeup, not an event log. After each receive, read `State()`; if several transitions occurred, the snapshot is the latest. It is safe to call `State()` concurrently, but only one consumer should drain `StateChanges` (fan out notifications if needed). It closes with the stream, and a buffered wakeup may still be read after closure. For example:

```go
for range stream.StateChanges {
    state := stream.State()
    if state.Phase == ws.PhaseReplaySent {
        // Reconcile using state.Generation and state.Interruptions.
        // Recheck State after asynchronous work; Version detects transitions.
    }
}
```

This snapshot describes what the client has observed, not an atomic barrier against future failures: a connection may already be broken before the read loop detects it, and it can fail immediately after any state check. A successful write is **not** a server acknowledgment, a snapshot-complete signal, or proof that missed events were recovered. Rechecking `Version` can detect observed transitions during reconciliation, but cannot make external work atomic with connection state.

`Stream.Replays` remains available for compatibility: it reports full writes with a generation and `Reconnected` flag, coalescing unread notifications to the latest replay. Joining an already-connected group or sending an incremental subscription does not produce one. **Do not use `Replays` and `Errors` as an ordered lifecycle:** an unread replay from an earlier connection can be received after an interruption, and an older error can be received after a newer replay. Notifications and messages on different channels have no cross-channel ordering; use `State()` for recovery decisions.

`Message.Typed` supports book, price change, tick size change, last trade price, best bid/ask, new market, market resolved, user trade, and user order events. Decimal values and timestamps use `json.Number` to preserve precision; IDs and enum-like fields are strings. Optional scalar fields are pointers. Typed decoding checks required fields, identifier formats, and quoted i64 timestamps, but does not enforce Rust's decimal range or enum validation. `Decode` remains permissive and `Raw` retains the original JSON; a routed opaque ID need not be accepted by `Typed`. Unknown event types return an error from `Typed`; their raw JSON remains available from `ParseMessages`. Stream parsers currently filter unknown event types.

Subscription requests and reconnect replays request `initial_dump: true`. As in Rust's ID multiplexing, a stream joining an already subscribed ID does **not** receive a separate initial dump; it receives subsequent events. A new subscriber can also observe a dump requested for another stream. Applications requiring an independent snapshot should obtain one separately. A stream joining an ID that remains active can also receive an in-flight event read before it joined; there are no per-subscriber sequence markers. The retirement policy rejects frames from the old connection, not historical data the server may send on the new one. There is no protocol acknowledgement or sequence-based recovery here, so reconnects, wildcard transitions, and network failures can leave gaps; reconcile state after reconnects.

Custom features are enabled for the shared market connection if any stream requests them. Enabling them after another market stream is active reconnects and replays all active IDs with the flag set; the flag stays enabled until the last market stream closes. Consequently streams that requested `false` can also receive custom event types. Unlike Rust's lagging broadcast receivers, a slow Go stream is terminated instead of silently skipping messages.

Credentials are included in user subscription and unsubscribe messages. Use `wss://` for real credentials and do not log the serialized requests. Multiplexing and replay assume the server treats subscriptions as sets of IDs and honors wildcard subscriptions on a fresh connection; the protocol provides no acknowledgement to verify those assumptions.
