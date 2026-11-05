package clob

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Credentials are the API key and secrets returned by the CLOB authentication endpoints.
type Credentials struct{ Key, Secret, Passphrase string }

// UnmarshalJSON accepts the API's apiKey as well as the upstream key alias.
func (c *Credentials) UnmarshalJSON(data []byte) error {
	var wire struct {
		Key        string `json:"key"`
		APIKey     string `json:"apiKey"`
		Secret     string `json:"secret"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Key != "" && wire.APIKey != "" {
		return fmt.Errorf("credentials contain both key and apiKey")
	}
	if wire.APIKey != "" {
		wire.Key = wire.APIKey
	}
	*c = Credentials{Key: wire.Key, Secret: wire.Secret, Passphrase: wire.Passphrase}
	return nil
}

// String prevents accidental disclosure when logging credentials with fmt.
func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{Key:%q, Secret:[REDACTED], Passphrase:[REDACTED]}", c.Key)
}
func (c Credentials) GoString() string { return c.String() }

// L1Headers creates the EIP-712 wallet proof used to create or derive API credentials.
func L1Headers(s HashSigner, chainID uint64, timestamp int64, nonce uint32) (http.Header, error) {
	if s == nil {
		return nil, fmt.Errorf("missing signer")
	}
	addr := s.Address()
	ts := strconv.FormatInt(timestamp, 10)
	structure := keccak(hashString("ClobAuth(address address,string timestamp,uint256 nonce,string message)"), addressWord(addr), hashString(ts), uintWord(uint64(nonce)), hashString("This message attests that I control the given wallet"))
	sig, err := signDigest(s, typedDigest(domainHash("ClobAuthDomain", "1", chainID, nil), structure))
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("POLY_ADDRESS", addr.String())
	h.Set("POLY_NONCE", strconv.FormatUint(uint64(nonce), 10))
	h.Set("POLY_TIMESTAMP", ts)
	h.Set("POLY_SIGNATURE", "0x"+hex.EncodeToString(sig))
	return h, nil
}

// L2Message signs the URL path, not the query string. body must be exactly the bytes sent.
func L2Message(timestamp int64, method string, u *url.URL, body []byte) (string, error) {
	if u == nil {
		return "", fmt.Errorf("missing request URL")
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return strconv.FormatInt(timestamp, 10) + method + path + strings.ReplaceAll(strings.ToValidUTF8(string(body), "\ufffd"), "'", "\""), nil
}

// L2Signature is the URL-safe padded base64 HMAC-SHA256 signature.
func L2Signature(secret, message string) (string, error) {
	key, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("invalid API secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// L2Headers authenticates a request with API credentials. Streaming bodies must be buffered by the caller.
func L2Headers(address Address, credentials Credentials, timestamp int64, method string, u *url.URL, body []byte) (http.Header, error) {
	msg, err := L2Message(timestamp, method, u, body)
	if err != nil {
		return nil, err
	}
	sig, err := L2Signature(credentials.Secret, msg)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("POLY_ADDRESS", address.Checksum())
	h.Set("POLY_API_KEY", credentials.Key)
	h.Set("POLY_PASSPHRASE", credentials.Passphrase)
	h.Set("POLY_SIGNATURE", sig)
	h.Set("POLY_TIMESTAMP", strconv.FormatInt(timestamp, 10))
	return h, nil
}
