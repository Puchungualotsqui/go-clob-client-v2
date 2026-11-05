package clob

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/sha3"
)

// Address and Hash are Ethereum primitives.
type Address [20]byte
type Hash [32]byte

func ParseAddress(s string) (Address, error) {
	var a Address
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil || len(b) != len(a) {
		return a, fmt.Errorf("invalid address %q", s)
	}
	copy(a[:], b)
	return a, nil
}
func (a Address) String() string { return "0x" + hex.EncodeToString(a[:]) }

// MarshalText and UnmarshalText encode addresses as hex strings in JSON and map keys.
func (a Address) MarshalText() ([]byte, error) { return []byte(a.String()), nil }
func (a *Address) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*a = Address{}
		return nil
	}
	v, err := ParseAddress(string(b))
	if err == nil {
		*a = v
	}
	return err
}

// Checksum returns the EIP-55 representation.
func (a Address) Checksum() string {
	s := hex.EncodeToString(a[:])
	h := keccak([]byte(s))
	b := []byte(s)
	for i, c := range b {
		n := h[i/2]
		if i%2 == 0 {
			n >>= 4
		} else {
			n &= 15
		}
		if c >= 'a' && c <= 'f' && n >= 8 {
			b[i] -= 32
		}
	}
	return "0x" + string(b)
}
func keccak(parts ...[]byte) Hash {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		_, _ = h.Write(p)
	}
	var out Hash
	h.Sum(out[:0])
	return out
}
func wordUint(v *big.Int) ([]byte, error) {
	if v == nil || v.Sign() < 0 || v.BitLen() > 256 {
		return nil, errors.New("invalid uint256")
	}
	b := make([]byte, 32)
	v.FillBytes(b)
	return b, nil
}
func uintWord(v uint64) []byte {
	b := make([]byte, 32)
	new(big.Int).SetUint64(v).FillBytes(b)
	return b
}
func addressWord(a Address) []byte { b := make([]byte, 32); copy(b[12:], a[:]); return b }
func hashString(s string) []byte   { h := keccak([]byte(s)); return h[:] }
func domainHash(name, version string, chainID uint64, contract *Address) Hash {
	typ := "EIP712Domain(string name,string version,uint256 chainId)"
	if contract != nil {
		typ = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
	}
	p := [][]byte{hashString(typ), hashString(name), hashString(version), uintWord(chainID)}
	if contract != nil {
		p = append(p, addressWord(*contract))
	}
	return keccak(p...)
}
func typedDigest(domain, structure Hash) Hash {
	return keccak([]byte{0x19, 0x01}, domain[:], structure[:])
}

// HashSigner signs a 32-byte EIP-712 digest without applying an Ethereum message prefix.
// SignHash returns a canonical low-S secp256k1 signature, r || s || v (v is 0/1 or 27/28).
// Implementations may use a local wallet, hardware wallet, or remote signer.
type HashSigner interface {
	Address() Address
	SignHash(Hash) ([]byte, error)
}

func signDigest(s HashSigner, digest Hash) ([]byte, error) {
	if s == nil {
		return nil, errors.New("missing signer")
	}
	sig, err := s.SignHash(digest)
	if err != nil {
		return nil, err
	}
	if len(sig) != 65 {
		return nil, errors.New("signer must return a 65-byte signature")
	}
	sig = append([]byte(nil), sig...)
	if sig[64] < 2 {
		sig[64] += 27
	}
	if sig[64] != 27 && sig[64] != 28 {
		return nil, errors.New("invalid signature recovery id")
	}
	return sig, nil
}

// DeriveWallet calculates the deterministic proxy or Safe wallet using CREATE2.
func DeriveWallet(owner Address, chainID uint64, kind SignatureType) (Address, error) {
	var zero Address
	if chainID != 137 && chainID != 80002 {
		return zero, errors.New("unsupported chain")
	}
	var factory, init string
	var salt Hash
	switch kind {
	case SignatureProxy:
		if chainID != 137 {
			return zero, errors.New("proxy wallets are not supported on this chain")
		}
		factory = "0xaB45c5A4B0c941a2F231C04C3f49182e1A254052"
		init = "d21df8dc65880a8606f09fe0ce3df9b8869287ab0b058be05aa9e8af6330a00b"
		salt = keccak(owner[:])
	case SignatureSafe:
		factory = "0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b"
		init = "2bce2127ff07fb632d16c8347c4ebf501f4841168bed00d9e6ef715ddb6fcecf"
		salt = keccak(addressWord(owner))
	default:
		return zero, errors.New("wallet derivation requires proxy or Safe signature type")
	}
	f, _ := ParseAddress(factory)
	code, _ := hex.DecodeString(init)
	h := keccak([]byte{0xff}, f[:], salt[:], code)
	copy(zero[:], h[12:])
	return zero, nil
}
