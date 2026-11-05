# Signing conformance vectors

`vectors.json` contains fixed inputs and expected outputs tested by Go. **The private key and HMAC secrets are public test values. Never fund or use them for real credentials.** The key is the well-known Anvil/Hardhat account 0, also present in `rs-clob-client-v2/src/auth.rs` and `py-sdk/tests/unit/test_order_typed_data_golden.py`; its address is in each order.

## Provenance and scope

- `rust_upstream` is copied exactly from `rs-clob-client-v2/src/auth.rs`, `tests::hmac_succeeds`. The other HMAC values were calculated independently using Python's standard-library `hmac` and `hashlib.sha256` over the exact UTF-8 `message`. The secret is URL-safe base64, and the result is padded URL-safe base64. Timestamps are fixed seconds; paths exclude query strings. Bodies are already normalized: Go/Rust replace apostrophes with double quotes while Python's helper takes a pre-serialized body verbatim.
- The two V2 EOA orders were constructed for this suite. Expected digests and deterministic 65-byte `r || s || v` signatures (v = 27 or 28) were produced by Go's `OrderDigest`/`SignOrder` and independently checked against Rust's SDK `OrderV2::eip712_signing_hash` and Alloy local signer. They are not upstream published order vectors. The timestamp is a fixed integer in milliseconds. Amounts and token IDs are decimal integer strings; rounding and builder behavior are not tested. V2 expiration is input but is not part of the EIP-712 signed struct.
- The historical Rust order check used the public SDK Solidity type and configuration plus Alloy's signer, not the authenticated client workflow. Its HMAC runner only checked that the fixture matched the upstream test vector; it did not execute the private Rust HMAC helper. These external checks are not part of the current Go test run. V1, V3, Poly1271, order serialization, live services, non-normalized HMAC bodies, and order construction are outside this suite.

## Run

From the Go project root:

```sh
go test -run TestSharedSigningConformance -v
```

The test reads the checked-in vectors and needs no sibling SDKs, credentials, clock, or network service.
