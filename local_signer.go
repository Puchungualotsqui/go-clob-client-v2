package clob

import (
	"encoding/hex"
	"errors"
	"strings"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// LocalSigner signs EIP-712 digests with a secp256k1 private key. Do not copy or
// log private keys; use a remote HashSigner for hardware-backed key storage.
type LocalSigner struct {
	key     *secp256k1.PrivateKey
	address Address
}

// NewLocalSigner accepts a 32-byte hex private key, optionally prefixed with 0x.
func NewLocalSigner(privateKey string) (*LocalSigner, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(privateKey, "0x"))
	if err != nil || len(b) != 32 {
		return nil, errors.New("private key must be 32-byte hex")
	}
	var scalar secp256k1.ModNScalar
	if scalar.SetByteSlice(b) || scalar.IsZero() {
		return nil, errors.New("private key outside secp256k1 range")
	}
	key := secp256k1.NewPrivateKey(&scalar)
	pub := key.PubKey().SerializeUncompressed()
	hash := keccak(pub[1:])
	var addr Address
	copy(addr[:], hash[12:])
	return &LocalSigner{key: key, address: addr}, nil
}
func (s *LocalSigner) Address() Address { return s.address }
func (s *LocalSigner) SignHash(h Hash) ([]byte, error) {
	if s == nil || s.key == nil {
		return nil, errors.New("missing private key")
	}
	compact := ecdsa.SignCompact(s.key, h[:], false)
	// Ethereum signatures support recovery IDs 0 and 1, not the rare 2 and 3.
	if compact[0] != 27 && compact[0] != 28 {
		return nil, errors.New("unsupported secp256k1 recovery id")
	}
	sig := make([]byte, 65)
	copy(sig, compact[1:])
	sig[64] = compact[0]
	return sig, nil
}
func (s *LocalSigner) String() string   { return "LocalSigner{private key: [REDACTED]}" }
func (s *LocalSigner) GoString() string { return s.String() }
