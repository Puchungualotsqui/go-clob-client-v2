# Decimal model regression tests

Run from the Go project root:

```sh
go test ./conformance/decimal
```

These Go-only tests cover selected decimal inputs previously used by the local cross-SDK comparison: precision, scale and magnitude boundaries, invalid values, and optional Gamma fields. They check the Go models' acceptance and serialized field values; they are regression tests of Go behavior, not independent cross-language conformance vectors. They require no sibling SDK or external runner.
