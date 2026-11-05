package clob

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"net/url"
	"strings"
)

// resolveOrderParams obtains authoritative market metadata. Version zero selects
// the server version for token orders; specify version 3 for position orders.
// A nonzero V1 fee is treated as a caller-provided fee to validate.
func (c *Client) resolveOrderParams(ctx context.Context, p OrderParams) (OrderParams, U256, error) {
	if c == nil || ctx == nil {
		return p, "", fmt.Errorf("client and context are required")
	}
	id, err := ParseU256(p.AssetID)
	if err != nil {
		return p, "", fmt.Errorf("asset ID: %w", err)
	}
	p.AssetID = id.String()
	if p.Version == 0 {
		version, err := c.ResolveVersion(ctx, false)
		if err != nil {
			return p, id, fmt.Errorf("resolve order version: %w", err)
		}
		if version != 1 && version != 2 {
			return p, id, fmt.Errorf("unsupported token order version %d", version)
		}
		p.Version = int(version)
	}
	if p.Version < 1 || p.Version > 3 {
		return p, id, fmt.Errorf("unsupported order version %d", p.Version)
	}
	tick, err := c.TickSize(ctx, id)
	if err != nil {
		return p, id, fmt.Errorf("resolve tick size: %w", err)
	}
	marketTick, err := ParseTickSize(Decimal(tick.MinimumTickSize))
	if err != nil {
		return p, id, fmt.Errorf("market tick size: %w", err)
	}
	if p.TickSize != "" {
		requested, err := ParseTickSize(Decimal(p.TickSize))
		if err != nil || requested != marketTick {
			return p, id, fmt.Errorf("tick size %q does not match market tick size %s", p.TickSize, marketTick)
		}
	}
	p.TickSize = string(marketTick)
	if p.Version == 1 {
		var supplied *uint32
		if p.FeeRateBPS != 0 {
			if p.FeeRateBPS > math.MaxUint32 {
				return p, id, fmt.Errorf("fee rate exceeds uint32")
			}
			v := uint32(p.FeeRateBPS)
			supplied = &v
		}
		fee, err := c.ResolveFeeRateBPS(ctx, id, supplied)
		if err != nil {
			return p, id, fmt.Errorf("resolve V1 fee: %w", err)
		}
		p.FeeRateBPS = uint64(fee)
	}
	if p.Builder == (Hash{}) && c.config.BuilderCode != nil {
		code, err := ParseB256(string(*c.config.BuilderCode))
		if err != nil {
			return p, id, fmt.Errorf("builder code: %w", err)
		}
		b, err := hex.DecodeString(string(code)[2:])
		if err != nil {
			return p, id, fmt.Errorf("builder code: %w", err)
		}
		copy(p.Builder[:], b)
	}
	return p, id, nil
}

// BuildLimitOrder builds an order using cached or remotely fetched market metadata.
// Version 0 resolves the token protocol version; version 3 denotes a position.
// Any supplied TickSize must agree with the market. Other wallet fields remain
// the caller's responsibility, as with the offline BuildLimitOrder function.
func (c *Client) BuildLimitOrder(ctx context.Context, p OrderParams, size string) (SignableOrder, error) {
	p, _, err := c.resolveOrderParams(ctx, p)
	if err != nil {
		return SignableOrder{}, err
	}
	return BuildLimitOrder(p, size)
}

// BuildMarketOrder fetches the opposing book if Price is empty. UserUSDCBalance
// enables platform fee-aware sizing for USDC buys; a nonzero builder code also
// requires authenticated fee lookup through AuthenticatedClient.BuildMarketOrder.
// Alternatively, supply MarketBuyFees explicitly (but not both).
func (c *Client) BuildMarketOrder(ctx context.Context, p OrderParams, amount MarketAmount) (SignableOrder, error) {
	return c.buildMarketOrder(ctx, p, amount, nil)
}

// BuildMarketOrder binds wallet fields to the identity established by
// Client.Authenticate, then uses authenticated builder-fee lookup when needed.
// A client created directly by NewAuthenticatedClient cannot build orders;
// use Client.Authenticate to establish the chain and funder first. SignatureEOA
// is the zero value and is treated as unspecified for non-EOA clients.
func (a *AuthenticatedClient) BuildMarketOrder(ctx context.Context, p OrderParams, amount MarketAmount) (SignableOrder, error) {
	p, err := a.bindOrderParams(p)
	if err != nil {
		return SignableOrder{}, err
	}
	return a.publicClient.buildMarketOrder(ctx, p, amount, a)
}

func (c *Client) buildMarketOrder(ctx context.Context, p OrderParams, amount MarketAmount, auth *AuthenticatedClient) (SignableOrder, error) {
	if p.UserUSDCBalance != nil {
		if p.MarketBuyFees != nil || p.Side != Buy || !amount.InUSDC {
			return SignableOrder{}, fmt.Errorf("user USDC balance requires a USDC market buy without explicit MarketBuyFees")
		}
		if _, _, err := decimal(*p.UserUSDCBalance); err != nil {
			return SignableOrder{}, fmt.Errorf("user USDC balance: %w", err)
		}
	}
	p, id, err := c.resolveOrderParams(ctx, p)
	if err != nil {
		return SignableOrder{}, err
	}
	var levels []BookLevel
	if p.Price == "" {
		book, err := c.OrderBook(ctx, OrderBookSummaryRequest{TokenID: id})
		if err != nil {
			return SignableOrder{}, fmt.Errorf("resolve order book: %w", err)
		}
		bookID, err := ParseU256(string(book.AssetID))
		if err != nil {
			return SignableOrder{}, fmt.Errorf("order book asset ID is missing or invalid: %w", err)
		}
		if bookID != id {
			return SignableOrder{}, fmt.Errorf("order book asset ID %s does not match requested asset %s", bookID, id)
		}
		entries := book.Asks
		if p.Side == Sell {
			entries = book.Bids
		}
		levels = make([]BookLevel, len(entries))
		for i, entry := range entries {
			levels[i] = BookLevel{Price: entry.Price.String(), Size: entry.Size.String()}
		}
	}
	if p.UserUSDCBalance != nil {
		info, err := c.orderFeeInfo(ctx, id)
		if err != nil {
			return SignableOrder{}, fmt.Errorf("resolve market fee: %w", err)
		}
		builderRate := "0"
		if p.Builder != (Hash{}) {
			if auth == nil {
				return SignableOrder{}, fmt.Errorf("authenticated client required to resolve builder fee")
			}
			code := B256("0x" + hex.EncodeToString(p.Builder[:]))
			rate, err := auth.BuilderFeeRate(ctx, code)
			if err != nil {
				return SignableOrder{}, fmt.Errorf("resolve builder fee: %w", err)
			}
			bps := rate.BuilderTakerFeeRateBPS
			builderRate = fmt.Sprintf("%d.%04d", bps/10000, bps%10000)
		}
		p.MarketBuyFees = &MarketBuyFees{Balance: *p.UserUSDCBalance, PlatformRate: info.Rate.String(), Exponent: info.Exponent, BuilderRate: builderRate}
		p.UserUSDCBalance = nil
	}
	return BuildMarketOrder(p, amount, levels)
}

// orderFeeInfo avoids treating an asset absent from returned metadata as fee-free.
func (c *Client) orderFeeInfo(ctx context.Context, id U256) (FeeInfo, error) {
	c.mu.RLock()
	info, ok := c.feeInfos[id]
	condition, known := c.tokenConditions[id]
	gen := c.generation
	c.mu.RUnlock()
	if ok {
		return info, nil
	}
	if !known {
		market, err := c.MarketByToken(ctx, id)
		if err != nil {
			return FeeInfo{}, err
		}
		condition = market.ConditionID
	}
	if condition == "" {
		return FeeInfo{}, fmt.Errorf("market lookup returned no condition ID for asset %s", id)
	}
	market, err := getMarket[ClobMarketInfoResponse](c, ctx, "clob-markets/"+url.PathEscape(string(condition)), nil, nil)
	if err != nil {
		return FeeInfo{}, err
	}
	match := market.ConditionID == condition
	if _, err := ParseB256(string(condition)); err == nil {
		match = strings.EqualFold(string(market.ConditionID), string(condition))
	}
	if !match {
		return FeeInfo{}, fmt.Errorf("market metadata condition does not match lookup for asset %s", id)
	}
	for _, token := range market.Tokens {
		if token != nil && cacheTokenID(token.TokenID) == id {
			info := FeeInfo{}
			if market.FeeDetails != nil {
				info = FeeInfo{Rate: market.FeeDetails.Rate, Exponent: market.FeeDetails.Exponent}
			}
			c.mu.Lock()
			if c.generation == gen {
				for _, member := range market.Tokens {
					if member == nil {
						continue
					}
					memberID := cacheTokenID(member.TokenID)
					c.tickSizes[memberID] = market.MinTickSize
					if market.negRiskPresent {
						c.negRisks[memberID] = market.NegRisk
					}
					c.feeInfos[memberID] = info
					c.tokenConditions[memberID] = condition
				}
			}
			c.mu.Unlock()
			return info, nil
		}
	}
	return FeeInfo{}, fmt.Errorf("market metadata does not contain asset %s", id)
}

// SignOrder resolves the token's negative-risk exchange before signing. V3
// position orders always use the V3 exchange and do not require a risk lookup.
// The caller supplies the chain, signer and owner (normally the API key).
func (c *Client) SignOrder(ctx context.Context, signer HashSigner, order SignableOrder, chainID uint64, owner string) (SignedOrder, error) {
	if c == nil || ctx == nil {
		return SignedOrder{}, fmt.Errorf("client and context are required")
	}
	negRisk := false
	if order.Order.Version != 3 {
		if order.Order.TokenID == nil {
			return SignedOrder{}, fmt.Errorf("missing asset ID")
		}
		id, err := ParseU256(order.Order.TokenID.String())
		if err != nil {
			return SignedOrder{}, fmt.Errorf("asset ID: %w", err)
		}
		risk, err := c.NegRisk(ctx, id)
		if err != nil {
			return SignedOrder{}, fmt.Errorf("resolve negative risk: %w", err)
		}
		negRisk = risk.NegRisk
	}
	return SignOrder(signer, order, chainID, negRisk, owner)
}
