package clob

import (
	"context"
	"fmt"
)

// authenticatedOrderBinding is established once at authentication and never mutated.

type authenticatedOrderBinding struct {
	address       Address
	chainID       uint64
	signatureType SignatureType
	funder        Address // maker, including the EOA case
}

func resolveAuthenticatedFunder(address Address, chain uint64, typ SignatureType, supplied *Address) (Address, error) {
	if address == (Address{}) {
		return Address{}, NewValidationError("signer address cannot be zero")
	}
	if typ > SignaturePoly1271 {
		return Address{}, NewValidationError("invalid signature type")
	}
	if typ == SignatureEOA {
		if supplied != nil {
			return Address{}, NewValidationError("EOA signature type cannot have a funder")
		}
		return address, nil
	}
	if supplied != nil {
		if *supplied == (Address{}) {
			return Address{}, NewValidationError("funder cannot be zero")
		}
		return *supplied, nil
	}
	if typ == SignaturePoly1271 {
		return Address{}, NewValidationError("Poly1271 requires a deposit wallet funder")
	}
	return DeriveWallet(address, chain, typ)
}

func (a *AuthenticatedClient) orderBinding() (authenticatedOrderBinding, error) {
	if a == nil || a.orderIdentity == nil || a.publicClient == nil {
		return authenticatedOrderBinding{}, fmt.Errorf("authenticated order operations require Client.Authenticate")
	}
	return *a.orderIdentity, nil
}

func (b authenticatedOrderBinding) validateOrderIdentity(order Order) error {
	expectedSigner := b.address
	if b.signatureType == SignaturePoly1271 {
		expectedSigner = b.funder
	}
	if order.SignatureType != b.signatureType || order.Maker != b.funder || order.Signer != expectedSigner {
		return fmt.Errorf("order identity does not match authentication")
	}
	return nil
}

// Standalone L2 clients have no chain or funder binding and retain their existing behavior.
// These field comparisons do not cryptographically verify the signature or its chain.
func (a *AuthenticatedClient) validateSignedOrderIdentity(order SignedOrder) error {
	if a == nil {
		return fmt.Errorf("authenticated client is required")
	}
	if a.orderIdentity == nil {
		return nil
	}
	if err := a.orderIdentity.validateOrderIdentity(order.Order); err != nil {
		return err
	}
	if order.Owner != a.credentials.Key {
		return fmt.Errorf("order owner does not match authenticated API key")
	}
	return nil
}

// bindOrderParams checks any supplied wallet fields and fills omitted values from
// authentication. SignatureEOA is also the zero value, so it is treated as
// omitted for non-EOA clients; callers cannot explicitly request EOA there.
func (a *AuthenticatedClient) bindOrderParams(p OrderParams) (OrderParams, error) {
	b, err := a.orderBinding()
	if err != nil {
		return p, err
	}
	if p.Signer != (Address{}) && p.Signer != b.address {
		return p, fmt.Errorf("order signer does not match authenticated address")
	}
	if p.ChainID != 0 && p.ChainID != b.chainID {
		return p, fmt.Errorf("order chain does not match authenticated chain")
	}
	if p.SignatureType != SignatureEOA && p.SignatureType != b.signatureType {
		return p, fmt.Errorf("order signature type does not match authentication")
	}
	if p.Funder != nil && (b.signatureType == SignatureEOA || *p.Funder != b.funder) {
		return p, fmt.Errorf("order funder does not match authentication")
	}
	p.Signer, p.ChainID, p.SignatureType = b.address, b.chainID, b.signatureType
	if b.signatureType != SignatureEOA {
		funder := b.funder
		p.Funder = &funder
	} else {
		p.Funder = nil
	}
	return p, nil
}

// BuildLimitOrder uses authenticated wallet identity and authoritative market metadata.
func (a *AuthenticatedClient) BuildLimitOrder(ctx context.Context, p OrderParams, size string) (SignableOrder, error) {
	p, err := a.bindOrderParams(p)
	if err != nil {
		return SignableOrder{}, err
	}
	return a.publicClient.BuildLimitOrder(ctx, p, size)
}

// BuildAuthenticatedMarketOrder is an alias for BuildMarketOrder.
func (a *AuthenticatedClient) BuildAuthenticatedMarketOrder(ctx context.Context, p OrderParams, amount MarketAmount) (SignableOrder, error) {
	return a.BuildMarketOrder(ctx, p, amount)
}

// SignOrder checks the signer and order identity before signing on the bound
// chain, and sets the owner to the authenticated API key. PostOrder and PostOrders
// compare identity fields for bound clients, but do not cryptographically verify
// a submitted signature or the chain on which it was signed.
func (a *AuthenticatedClient) SignOrder(ctx context.Context, signer HashSigner, order SignableOrder) (SignedOrder, error) {
	b, err := a.orderBinding()
	if err != nil {
		return SignedOrder{}, err
	}
	if missingHashSigner(signer) || signer.Address() != b.address {
		return SignedOrder{}, fmt.Errorf("signer does not match authenticated address")
	}
	if err := b.validateOrderIdentity(order.Order); err != nil {
		return SignedOrder{}, err
	}
	return a.publicClient.SignOrder(ctx, signer, order, b.chainID, a.credentials.Key)
}
