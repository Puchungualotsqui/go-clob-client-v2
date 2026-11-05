package clob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

// AuthenticationOptions controls how Client.Authenticate obtains credentials.
// Credentials and Nonce cannot both be supplied. SignatureType defaults to EOA.
// Proxy and Safe funders are derived when omitted; Poly1271 requires a funder.
type AuthenticationOptions struct {
	Credentials   *Credentials
	Nonce         *uint32
	SignatureType SignatureType
	Funder        *Address
}

func missingHashSigner(signer HashSigner) bool {
	if signer == nil {
		return true
	}
	v := reflect.ValueOf(signer)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// authenticationTransport copies the public client's HTTP configuration while
// disabling redirects, so L1 proofs and L2 credentials cannot be forwarded.
func (c *Client) authenticationTransport(address Address, credentials Credentials, signatureType SignatureType) (*AuthenticatedClient, error) {
	config := c.Config()
	return NewAuthenticatedClient(c.Host(), address, credentials, AuthenticatedClientOptions{
		HTTPClient: c.httpClient, SignatureType: signatureType,
		UseServerTime: config.UseServerTime, HeartbeatInterval: config.HeartbeatInterval,
	})
}

// Unlike encoding/json's default struct decoding, Rust requires all credential
// fields to be present and non-null in API responses.
type l1Credentials Credentials

func (c *l1Credentials) UnmarshalJSON(data []byte) error {
	var fields struct {
		Key        *string `json:"key"`
		APIKey     *string `json:"apiKey"`
		Secret     *string `json:"secret"`
		Passphrase *string `json:"passphrase"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if (fields.Key == nil) == (fields.APIKey == nil) || fields.Secret == nil || fields.Passphrase == nil {
		return fmt.Errorf("credentials must contain exactly one key and both secret and passphrase")
	}
	key := fields.Key
	if key == nil {
		key = fields.APIKey
	}
	*c = l1Credentials{Key: *key, Secret: *fields.Secret, Passphrase: *fields.Passphrase}
	return nil
}

func (c *Client) l1APIKey(ctx context.Context, signer HashSigner, chainID uint64, nonce *uint32, method, path string) (Credentials, error) {
	var credentials Credentials
	if missingHashSigner(signer) {
		return credentials, NewValidationError("missing signer")
	}
	if chainID == 0 {
		return credentials, NewValidationError("chain ID must be set")
	}
	transport, err := NewAuthenticatedClient(c.Host(), Address{}, Credentials{}, AuthenticatedClientOptions{
		HTTPClient: c.httpClient, UseServerTime: c.Config().UseServerTime,
	})
	if err != nil {
		return credentials, err
	}
	req, err := http.NewRequestWithContext(ctx, method, transport.host+"/"+path, nil)
	if err != nil {
		return credentials, err
	}
	timestamp := time.Now().Unix()
	if transport.useServerTime {
		timestamp, err = transport.serverTime(ctx)
		if err != nil {
			return credentials, err
		}
	}
	var n uint32
	if nonce != nil {
		n = *nonce
	}
	headers, err := L1Headers(signer, chainID, timestamp, n)
	if err != nil {
		return credentials, err
	}
	req.Header = headers
	req.Header.Set("Accept", "application/json")
	var response l1Credentials
	err = transport.execute(req, &response)
	if err != nil {
		return credentials, err
	}
	return Credentials(response), nil
}

// CreateAPIKey creates L2 credentials for the signer's address and nonce.
// The nonce defaults to zero. chainID is supplied separately because HashSigner
// does not carry a chain ID.
func (c *Client) CreateAPIKey(ctx context.Context, signer HashSigner, chainID uint64, nonce *uint32) (Credentials, error) {
	return c.l1APIKey(ctx, signer, chainID, nonce, http.MethodPost, "auth/api-key")
}

// DeriveAPIKey retrieves existing L2 credentials for the signer's address and nonce.
func (c *Client) DeriveAPIKey(ctx context.Context, signer HashSigner, chainID uint64, nonce *uint32) (Credentials, error) {
	return c.l1APIKey(ctx, signer, chainID, nonce, http.MethodGet, "auth/derive-api-key")
}

// CreateOrDeriveAPIKey falls back to deriving only when creation returns a
// status-kind error, matching the Rust client. Signing, network and decode
// errors are returned directly.
func (c *Client) CreateOrDeriveAPIKey(ctx context.Context, signer HashSigner, chainID uint64, nonce *uint32) (Credentials, error) {
	credentials, err := c.CreateAPIKey(ctx, signer, chainID, nonce)
	if err == nil {
		return credentials, nil
	}
	var clobErr *Error
	if errors.As(err, &clobErr) && clobErr.Kind == KindStatus {
		return c.DeriveAPIKey(ctx, signer, chainID, nonce)
	}
	return Credentials{}, err
}

// Authenticate creates an independent L2 client using supplied credentials or
// create-or-derive. It retains the public client's host, HTTP client, server-time
// and heartbeat settings. The public client's protocol version cache is refreshed
// on an order version mismatch. Only Polygon and Amoy are supported, as in Rust.
// The original public client remains usable.
func (c *Client) Authenticate(ctx context.Context, signer HashSigner, chainID uint64, options ...AuthenticationOptions) (*AuthenticatedClient, error) {
	if missingHashSigner(signer) {
		return nil, NewValidationError("missing signer")
	}
	if chainID != Polygon && chainID != Amoy {
		return nil, NewValidationError("only Polygon and Amoy are supported")
	}
	if len(options) > 1 {
		return nil, NewValidationError("at most one authentication options value may be supplied")
	}
	var o AuthenticationOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.SignatureType > SignaturePoly1271 {
		return nil, NewValidationError("invalid signature type")
	}
	if o.Credentials != nil && o.Nonce != nil {
		return nil, NewValidationError("credentials and nonce cannot both be supplied")
	}
	address := signer.Address()
	funder, err := resolveAuthenticatedFunder(address, chainID, o.SignatureType, o.Funder)
	if err != nil {
		return nil, err
	}
	transport, err := c.authenticationTransport(address, Credentials{}, o.SignatureType)
	if err != nil {
		return nil, err
	}
	var credentials Credentials
	if o.Credentials != nil {
		credentials = *o.Credentials
	} else {
		var err error
		credentials, err = c.CreateOrDeriveAPIKey(ctx, signer, chainID, o.Nonce)
		if err != nil {
			return nil, err
		}
	}
	if signer.Address() != address {
		return nil, NewValidationError("signer address changed during authentication")
	}
	transport.credentials = credentials
	transport.orderIdentity = &authenticatedOrderBinding{address: address, chainID: chainID, signatureType: o.SignatureType, funder: funder}
	transport.publicClient = c
	transport.builderCache = c
	return transport, nil
}
