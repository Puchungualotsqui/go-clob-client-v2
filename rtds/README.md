# RTDS

This package uses `github.com/gorilla/websocket`, included in the parent module.

`NewClient` accepts an endpoint (empty selects the production endpoint). `Subscribe` accepts any topic/type and optional filter and credentials; `SubscribeCryptoPrices`, `SubscribeChainlinkPrices`, and `SubscribeComments` provide common subscriptions. RTDS expects Chainlink filters as JSON-encoded strings but Binance filters as JSON arrays; the constructors and `Subscription` marshaler handle this distinction.

```go
client, err := rtds.NewClient("", rtds.Config{})
if err != nil { /* handle error */ }
stream, err := client.SubscribeCryptoPrices(ctx, []string{"btcusdt"})
if err != nil { /* handle error */ }
defer stream.Close()
for msg := range stream.Messages {
    var price rtds.Price
    if err := msg.DecodePayload(&price); err != nil { /* handle error */ }
    // price.Value is a json.Number; use its String method to retain precision.
}
```

Import this package as `github.com/puchungualotsqui/go-clob-client-v2/rtds`; the example assumes `ctx` is a `context.Context`. Streams created by the same client share connections when safe. Identical wire subscriptions (topic, type, filters and credentials) share one server subscription with a reference per stream, including across raw and typed APIs. Different topics can share a connection only when their credentials are identical; absent credentials and an explicitly supplied credential object are distinct. Filters and credentials are snapshotted when subscribing.

A connection carries at most one distinct subscription per topic. Different filters or types for the same topic are isolated, including wildcard versus specific types: RTDS messages do not identify their originating subscription and unsubscribe only specifies topic/type. After the last reference to a topic is removed, that topic is not reused on that socket, since already-in-flight messages cannot be attributed safely. Retired topics become eligible again after the socket closes; a long-lived connection is rotated after 128 topic retirements to bound retained state. These conservative rules and rotations can require multiple connections or cause a brief interruption; account for server connection limits.

Connections automatically reconnect and replay only active subscriptions with their original filters and credentials. They send text `PING`/`PONG` heartbeats. Writes and message delivery are ordered per socket. Socket writes and payload copying/decoding do not hold the client-wide lock or block independent connections, although expensive decoding can delay other streams on the same socket. `Stream.Close` and context cancellation release only that stream; the last reference attempts unsubscribe when connected, and the connection closes when no subscriptions remain. `Close` waits for the write result, or reports an error if the socket retires before the queued write can be attempted; it does not wait for the server to acknowledge unsubscribe. Cancellation or a disconnected connection cannot guarantee server receipt.

Drain `Messages` promptly: a full buffer stops the stream and reports an error rather than silently dropping data. `Errors` is a bounded, best-effort diagnostic channel; both channels close when the stream ends. A production consumer should monitor both channels. `MaxAttempts` limits consecutive failed dials per stream (zero means unlimited); a successful connection resets each active stream's counter. A stream joining during an in-flight dial counts that dial if it fails, but does not inherit earlier failures. Retry delay is shared by the connection and doubles up to `MaxBackoff`, so a late joiner can wait through an existing backoff.

### Recovery state

Raw and typed streams expose `State()` and `StateChanges`. `State()` is a synchronized, versioned snapshot of what this client has observed, not a server acknowledgement:

- `PhaseDisconnected`: no usable socket has been observed, or the previous socket was retired.
- `PhaseReplaying`: a socket is established and this subscription's initial or replay write has not yet completed.
- `PhaseReady`: this subscription's write completed locally on the current socket. It does **not** mean the server accepted it, delivered a snapshot, or filled any gap.
- `PhaseClosed`: the stream stopped; its final snapshot remains available.

`Generation` counts established sockets in the shared connection group, including one whose subscription write subsequently fails; `Interruptions` counts socket retirements observed while that group still had streams. A failed dial does not increment either counter. Both are group-scoped, reset with a new group, and a late joiner sees the group's current values. `Version` starts at 1 for each stream and advances on each snapshot change. A late joiner to an already-written identical subscription is immediately locally ready; a new topic remains replaying until its own write completes.

`StateChanges` is a one-slot coalescing wakeup, not an event log: after receiving, read `State()` and use its counters and version to detect changes. It initially contains a wakeup, closes with the stream, and may still contain a buffered wakeup after closure. Only one consumer should drain it. No ordering is guaranteed between state wakeups, messages, and errors, and a broken connection may not be noticed immediately. Applications requiring continuity must reconcile independently after interruptions; neither reconnect nor a ready state guarantees gap-free recovery.

For decoded streams, use `SubscribeTypedCryptoPrices`, `SubscribeTypedChainlinkPrices`, or `SubscribeTypedComments`. These use the same filters, authentication, reconnection, and `Close` behavior as their raw counterparts. Like Rust's typed subscriptions, they skip payloads that cannot be decoded; malformed message envelopes are reported on `Errors`. Use a raw subscription when every payload or its envelope is needed.

`Message.AsCryptoPrice`, `AsChainlinkPrice`, and `AsComment` provide topic-aware payload decoding; they return false for a different topic or an invalid payload. The raw `Message` and `DecodePayload` remain available for custom topics and events. Subscription decoding retains filters for wire round-trips: non-Chainlink filters are stored as JSON text (including quotes for JSON strings), whereas Chainlink filters are stored as the decoded string.

## Differences from Rust

- Existing convenience subscriptions return raw `Message` streams; the separate `SubscribeTyped...` methods return typed payloads. Typed streams discard invalid payloads, while raw streams retain them.
- Sharing is deliberately restricted by topic and authentication identity. There are no client-level unsubscribe or subscription-count APIs; close the individual stream to release its reference.
- Prices use `json.Number`, not Rust's bounded decimal type; comment addresses are strings and are not validated as Polygon addresses. Typed helpers check required fields but Go's JSON decoding does not enforce all Rust deserialization rules (for example, null optional/default fields).
- Authentication is supplied per subscription instead of using Rust's authenticated client state. Errors are delivered separately on a best-effort channel; a full message buffer stops the Go stream instead of Rust's lagged-receiver behavior.

Authenticated subscriptions serialize CLOB credentials into the request. Use `wss://` for real credentials and do not log serialized requests.
