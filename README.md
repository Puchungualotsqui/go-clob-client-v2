# Go CLOB client v2

A Go implementation of the Polymarket CLOB SDK. It includes CLOB market and authenticated HTTP endpoints, RFQ, API-key authentication, EIP-712 order signing and order construction, plus Data, Gamma, Bridge, CTF, CLOB WebSocket and RTDS clients. The module and its tests do not require sibling SDK checkouts or non-Go toolchains.

**Status: port in progress, not yet a drop-in equivalent of the Rust SDK.** It has not been tested against live services or with real funds. In particular, independently verify order payloads, signatures, fee calculations, wallet configuration and settlement behavior before using it for trading. See the differences below.

## Module and tests

Requires Go 1.24 or newer. The module path is `github.com/puchungualotsqui/go-clob-client-v2`. After the standalone repository has been created and a release or commit is available, install it with:

```sh
go get github.com/puchungualotsqui/go-clob-client-v2@latest
```

```sh
go test -race ./...
go vet ./...
```

Go-only regression fixtures are described in `conformance/signing/README.md` and `conformance/decimal/README.md`. The signing corpus covers a limited subset, not every order type or submission workflow; the decimal tests describe Go behavior rather than performing cross-language comparisons.

The `.github/workflows/go.yml` workflow runs formatting, vet, race-enabled tests, a build and a clean-consumer check for Go 1.24 and the current stable Go release. It is discovered by GitHub Actions when this directory is the root of the standalone repository; it is not discovered while this directory remains nested in another repository. The consumer check uses a local module replacement, so it does not verify remote fetching or a published release. The repository name's `-v2` denotes the API version, not the Go module's semantic major version: initial Go releases can use v0 or v1 tags. A Go module released at major version 2 or later requires a corresponding `/v2` (or later) module-path suffix.

## CLOB market data

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    clob "github.com/puchungualotsqui/go-clob-client-v2"
)

func main() {
    client, err := clob.NewClient("", clob.DefaultConfig())
    if err != nil { log.Fatal(err) }
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    timestamp, err := client.ServerTime(ctx)
    if err != nil { log.Fatal(err) }
    fmt.Println(timestamp)
}
```

`NewClient` accepts an optional `*http.Client`, and all network methods accept a context. An empty host selects the production CLOB endpoint. Other service clients live in `data`, `gamma`, `bridge`, `ctf`, `ws` and `rtds`; see the package documentation and the streaming and CTF READMEs.

## Authentication and orders

`NewLocalSigner` or a custom `HashSigner` supplies a wallet signer. `Client.Authenticate` accepts Polygon or Amoy chain ID and optionally existing credentials; otherwise it creates or derives API credentials. The returned `AuthenticatedClient` is separate from the public client. Never commit private keys or credentials, and use HTTPS/WSS for authenticated requests.

The package-level `BuildLimitOrder` and `BuildMarketOrder` accept explicit `OrderParams`. Alternatively, the client methods resolve market metadata, protocol version, books and risk mode as needed. Clients created by `Client.Authenticate` provide `BuildLimitOrder`, `BuildMarketOrder` and `SignOrder` methods that bind the wallet, chain and owner to authentication; configure proxy, Safe or Poly1271 funders through `AuthenticationOptions`. Standalone `NewAuthenticatedClient` clients do not have this binding and cannot use these authenticated builders. Balance-aware market buys can set `UserUSDCBalance`. As in Rust, absent market fee details are treated as zero fees; confirm that assumption for your markets before trading.

Bound clients check submitted orders' identity fields, but do not cryptographically verify an externally supplied signature or its chain. A signed order must not be mutated before submission. V2/V3 expiration is not covered by the protocol signature. Order posting may perform best-effort settlement-hash polling for up to 30 seconds. A network or response-decoding error after submission does **not** prove an order failed: reconcile order and trade state before retrying to avoid duplicate exposure.

`AuthenticatedClient.BuildSignAndPostLimitOrder` and `BuildSignAndPostMarketOrder` combine those steps. For automatically selected token versions, they rebuild and re-sign at most once after an HTTP 400 carrying the exact `order_version_mismatch` code and a successful refresh reports a different supported version. This relies on the service treating that response as a rejection; it has not been independently verified against a live service. Explicitly pinned versions and V3 positions are not retried. A V2-to-V1 transition is not retried if it would discard metadata or builder attribution. Unlike Rust's substring check, unrelated errors containing the marker do not trigger retry. Inspect `PostOrderResponse.Success` even when the error is nil.

These convenience methods return `*SubmissionError` for transport, response-decoding, HTTP 408 and server errors that can leave submission outcomes uncertain. Its `Order` field contains the signed payload to aid reconciliation; treat it as sensitive and do not blindly resend it. The error unwraps to its underlying cause. Do not mutate pointer-backed `OrderParams` fields while a workflow is in progress; concurrent workflows require a concurrency-safe signer.

## Known differences and limitations

- CLOB and RTDS subscriptions share connections and reference-count compatible subscriptions. RTDS deliberately isolates incompatible filters, types or credentials, so it can use more than one connection. CLOB events can be decoded with `Message.Typed`, but event-specific subscription methods and all Rust decoding checks are not implemented. See `ws/README.md` and `rtds/README.md` for routing, reconnect, buffering and shutdown behavior.
- Model validation is stricter for required fields, identifiers, dates and numeric values, but does not yet reproduce every Rust serde rule. In particular, Rust Decimal parsing can round excess precision; Data and Gamma request validation instead reject values that are not exactly representable, while Gamma responses can retain excess fractional precision. Bridge rejects digitless base-unit amounts instead of treating them as zero. Some auxiliary fields remain strings; validate constructed values at application boundaries.
- Gamma's `RelatedTag` ID fields and `Event.ParentEventID` / `Event.TurnProviderID` use `StringFromAny` rather than `Number` to represent Rust's string-or-integer identifiers. Existing Go struct literals or assignments using `Number` for those fields need explicit conversion. Unknown Bridge transaction statuses now fail decoding, matching the current Rust enum.
- Offline order builders require explicit market metadata. Authenticated builders inherit wallet settings from `Client.Authenticate`; low-level builders still require callers to supply them. Validation rejects some inputs accepted by Rust; USDC market buys may spend slightly less to preserve the requested price limit. The zero-value signature type is treated as unspecified by authenticated builders, which substitute the bound type.
- Authenticated clients do not automatically start heartbeats. Call `StartHeartbeats` and `StopHeartbeats` when needed. Background heartbeat errors are not exposed individually.
- CTF transactions require a configured Ethereum backend and signer, correct chain, funds and approvals. They wait for receipts and report a mined failure as an error. See `ctf/README.md`.
- Endpoint and signing tests use local mocks and upstream vectors. Full Rust feature, serialization and streaming parity and live network behavior have not been established.

The original Rust SDK is MIT licensed; its license is included in `LICENSE`.
