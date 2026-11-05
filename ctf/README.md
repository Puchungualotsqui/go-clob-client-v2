# Conditional Token Framework client

This package ports the Rust CTF client's three on-chain ID reads and its split,
merge, standard redemption and negative-risk redemption transactions. `New`
and `WithNegRisk` select the Rust-configured contracts for Polygon (137) and
Amoy (80002). The backend must implement `ctf.Backend`, as `*ethclient.Client`
does. The package depends on `github.com/ethereum/go-ethereum`, included in the
parent module.

Pass a `context.Context` to each operation. Transactions additionally require a
`*bind.TransactOpts` with a signer, normally created using
`bind.NewKeyedTransactorWithChainID`. The caller is responsible for connecting
to the correct chain, funding the signer, and obtaining any required collateral
or token approvals. Never put private keys in source code. Transaction calls
submit and wait for a receipt; use a context deadline. If waiting fails after
submission, the response still contains the transaction hash so the caller can
check its status before retrying. A mined transaction with failed status is
reported as an error. `NoSend` is not supported because these methods wait for
receipts. Transaction options are copied, and the method's context overrides
`TransactOpts.Context`.

All amounts and index sets are `*big.Int` uint256 values; nil, negative and
overflowing values are rejected. `BinaryPartition` and the binary-market
request constructors provide independent `[1, 2]` index sets. As in Rust, the
contract enforces market-specific constraints. This package does not expose
`prepareCondition`, which is declared in Rust's ABI but has no public client
method. It does not implement wallet approvals, private-key management, chain
verification, or offline ID calculation.
