package clob

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

func getMarket[T any](c *Client, ctx context.Context, path string, query any, cursor *string) (T, error) {
	var out T
	q, err := queryValues(query, cursor)
	if err != nil {
		return out, err
	}
	err = c.request(ctx, c.host, http.MethodGet, path, q, nil, &out)
	return out, err
}

// StreamData fetches pages until TerminalCursor and invokes yield for each item.
// Returning an error from fetch or yield stops iteration immediately.
func StreamData[T any](ctx context.Context, fetch func(context.Context, *string) (Page[T], error), yield func(T) error) error {
	if ctx == nil || fetch == nil || yield == nil {
		return fmt.Errorf("context, fetch, and yield are required")
	}
	var cursor *string
	seen := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := fetch(ctx, cursor)
		if err != nil {
			return err
		}
		if page.NextCursor != TerminalCursor {
			if page.NextCursor == "" {
				return fmt.Errorf("pagination returned an empty next cursor")
			}
			if _, ok := seen[page.NextCursor]; ok {
				return fmt.Errorf("pagination returned a repeated cursor %q", page.NextCursor)
			}
			seen[page.NextCursor] = struct{}{}
		}
		for _, item := range page.Data {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := yield(item); err != nil {
				return err
			}
		}
		if page.NextCursor == TerminalCursor {
			return nil
		}
		next := page.NextCursor
		cursor = &next
	}
}

func postMarket[T any](c *Client, ctx context.Context, path string, body any) (T, error) {
	var out T
	err := c.request(ctx, c.host, http.MethodPost, path, nil, body, &out)
	return out, err
}

func (c *Client) OK(ctx context.Context) (string, error) {
	return getMarket[string](c, ctx, "", nil, nil)
}
func (c *Client) ServerTime(ctx context.Context) (int64, error) {
	return getMarket[int64](c, ctx, "time", nil, nil)
}
func (c *Client) Version(ctx context.Context) (uint32, error) { return c.ResolveVersion(ctx, false) }

// ResolveVersion optionally refreshes the cached protocol version.
func (c *Client) ResolveVersion(ctx context.Context, force bool) (uint32, error) {
	c.mu.Lock()
	if !force && c.version != 0 {
		v := c.version
		c.mu.Unlock()
		return v, nil
	}
	gen := c.generation
	c.versionRequest++
	seq := c.versionRequest
	c.mu.Unlock()
	r, err := getMarket[struct {
		Version *uint32 `json:"version"`
	}](c, ctx, "version", nil, nil)
	if err != nil {
		return 0, err
	}
	if r.Version == nil {
		return 0, fmt.Errorf("decoding version response: missing version")
	}
	c.mu.Lock()
	// Only successful requests advance the cached sequence, so a failed newer
	// request cannot prevent an older successful response from being cached.
	if c.generation == gen && seq > c.versionCached {
		c.version = *r.Version
		c.versionCached = seq
	}
	c.mu.Unlock()
	return *r.Version, nil
}
func (c *Client) Midpoint(ctx context.Context, r MidpointRequest) (MidpointResponse, error) {
	return getMarket[MidpointResponse](c, ctx, "midpoint", r, nil)
}
func (c *Client) Midpoints(ctx context.Context, r []MidpointRequest) (MidpointsResponse, error) {
	if r == nil {
		r = []MidpointRequest{}
	}
	return postMarket[MidpointsResponse](c, ctx, "midpoints", r)
}
func (c *Client) Price(ctx context.Context, r PriceRequest) (PriceResponse, error) {
	return getMarket[PriceResponse](c, ctx, "price", r, nil)
}
func (c *Client) Prices(ctx context.Context, r []PriceRequest) (PricesResponse, error) {
	if r == nil {
		r = []PriceRequest{}
	}
	return postMarket[PricesResponse](c, ctx, "prices", r)
}
func (c *Client) PriceHistory(ctx context.Context, r PriceHistoryRequest) (PriceHistoryResponse, error) {
	return getMarket[PriceHistoryResponse](c, ctx, "prices-history", r, nil)
}
func (c *Client) Spread(ctx context.Context, r SpreadRequest) (SpreadResponse, error) {
	return getMarket[SpreadResponse](c, ctx, "spread", r, nil)
}
func (c *Client) Spreads(ctx context.Context, r []SpreadRequest) (SpreadsResponse, error) {
	if r == nil {
		r = []SpreadRequest{}
	}
	return postMarket[SpreadsResponse](c, ctx, "spreads", r)
}

func (c *Client) TickSize(ctx context.Context, id U256) (TickSizeResponse, error) {
	id = cacheTokenID(id)
	c.mu.RLock()
	v, ok := c.tickSizes[id]
	gen := c.generation
	c.mu.RUnlock()
	if ok {
		return TickSizeResponse{MinimumTickSize: v}, nil
	}
	r, err := getMarket[TickSizeResponse](c, ctx, "tick-size", MidpointRequest{TokenID: id}, nil)
	if err != nil {
		return r, err
	}
	c.mu.Lock()
	if c.generation == gen {
		if _, exists := c.tickSizes[id]; !exists {
			c.tickSizes[id] = r.MinimumTickSize
		}
	}
	c.mu.Unlock()
	return r, nil
}
func (c *Client) NegRisk(ctx context.Context, id U256) (NegRiskResponse, error) {
	id = cacheTokenID(id)
	c.mu.RLock()
	v, ok := c.negRisks[id]
	gen := c.generation
	c.mu.RUnlock()
	if ok {
		return NegRiskResponse{NegRisk: v}, nil
	}
	r, err := getMarket[NegRiskResponse](c, ctx, "neg-risk", MidpointRequest{TokenID: id}, nil)
	if err != nil {
		return r, err
	}
	c.mu.Lock()
	if c.generation == gen {
		if _, exists := c.negRisks[id]; !exists {
			c.negRisks[id] = r.NegRisk
		}
	}
	c.mu.Unlock()
	return r, nil
}
func (c *Client) FeeRateBPS(ctx context.Context, id U256) (FeeRateResponse, error) {
	id = cacheTokenID(id)
	c.mu.RLock()
	v, ok := c.feeRates[id]
	gen := c.generation
	c.mu.RUnlock()
	if ok {
		return v, nil
	}
	r, err := getMarket[FeeRateResponse](c, ctx, "fee-rate", MidpointRequest{TokenID: id}, nil)
	if err != nil {
		return r, err
	}
	c.mu.Lock()
	if c.generation == gen {
		if _, exists := c.feeRates[id]; !exists {
			c.feeRates[id] = r
		}
	}
	c.mu.Unlock()
	return r, nil
}

// ResolveFeeRateBPS validates a caller-provided V1 fee against the market fee.
func (c *Client) ResolveFeeRateBPS(ctx context.Context, id U256, user *uint32) (uint32, error) {
	r, err := c.FeeRateBPS(ctx, id)
	if err != nil {
		return 0, err
	}
	if user != nil && r.BaseFee > 0 && *user != r.BaseFee {
		return 0, NewValidationError(fmt.Sprintf("invalid user-provided fee rate %d; market fee rate must be %d", *user, r.BaseFee))
	}
	return r.BaseFee, nil
}
func (c *Client) CheckGeoblock(ctx context.Context) (GeoblockResponse, error) {
	var out GeoblockResponse
	err := c.request(ctx, c.geoblockHost, http.MethodGet, "api/geoblock", nil, nil, &out)
	return out, err
}
func (c *Client) OrderBook(ctx context.Context, r OrderBookSummaryRequest) (OrderBookSummaryResponse, error) {
	return getMarket[OrderBookSummaryResponse](c, ctx, "book", r, nil)
}
func (c *Client) OrderBooks(ctx context.Context, r []OrderBookSummaryRequest) ([]OrderBookSummaryResponse, error) {
	if r == nil {
		r = []OrderBookSummaryRequest{}
	}
	return postMarket[[]OrderBookSummaryResponse](c, ctx, "books", r)
}

// OrderBookHash matches the Rust client's SHA-256 of the serialized snapshot.
func (c *Client) OrderBookHash(book OrderBookSummaryResponse) (string, error) {
	// The Rust type uses vectors, never null arrays.
	if book.Bids == nil {
		book.Bids = []OrderSummary{}
	}
	if book.Asks == nil {
		book.Asks = []OrderSummary{}
	}
	id, err := ParseU256(book.AssetID.String())
	if err != nil {
		return "", err
	}
	n, _ := new(big.Int).SetString(id.String(), 10)
	// Alloy serializes U256 as a hexadecimal quantity in this Rust hash,
	// whereas the public Go model serializes it as a decimal string.
	wire := struct {
		Market         B256             `json:"market"`
		AssetID        string           `json:"asset_id"`
		Timestamp      UnixMilliseconds `json:"timestamp"`
		Hash           *string          `json:"hash"`
		Bids           []OrderSummary   `json:"bids"`
		Asks           []OrderSummary   `json:"asks"`
		MinOrderSize   Decimal          `json:"min_order_size"`
		NegRisk        bool             `json:"neg_risk"`
		TickSize       TickSize         `json:"tick_size"`
		LastTradePrice *Decimal         `json:"last_trade_price"`
	}{book.Market, "0x" + n.Text(16), book.Timestamp, book.Hash, book.Bids, book.Asks, book.MinOrderSize, book.NegRisk, book.TickSize, book.LastTradePrice}
	b, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}
func (c *Client) LastTradePrice(ctx context.Context, r LastTradePriceRequest) (LastTradePriceResponse, error) {
	return getMarket[LastTradePriceResponse](c, ctx, "last-trade-price", r, nil)
}
func (c *Client) LastTradesPrices(ctx context.Context, r []LastTradePriceRequest) ([]LastTradesPricesResponse, error) {
	if r == nil {
		r = []LastTradePriceRequest{}
	}
	var out []LastTradesPricesResponse
	err := c.request(ctx, c.host, http.MethodGet, "last-trades-prices", nil, r, &out)
	return out, err
}
func (c *Client) Market(ctx context.Context, conditionID string) (MarketResponse, error) {
	return getMarket[MarketResponse](c, ctx, "markets/"+url.PathEscape(conditionID), nil, nil)
}
func (c *Client) Markets(ctx context.Context, cursor *string) (Page[MarketResponse], error) {
	return getMarket[Page[MarketResponse]](c, ctx, "markets", nil, cursor)
}
func (c *Client) SamplingMarkets(ctx context.Context, cursor *string) (Page[MarketResponse], error) {
	return getMarket[Page[MarketResponse]](c, ctx, "sampling-markets", nil, cursor)
}
func (c *Client) SimplifiedMarkets(ctx context.Context, cursor *string) (Page[SimplifiedMarketResponse], error) {
	return getMarket[Page[SimplifiedMarketResponse]](c, ctx, "simplified-markets", nil, cursor)
}
func (c *Client) SamplingSimplifiedMarkets(ctx context.Context, cursor *string) (Page[SimplifiedMarketResponse], error) {
	return getMarket[Page[SimplifiedMarketResponse]](c, ctx, "sampling-simplified-markets", nil, cursor)
}
func (c *Client) ClobMarketInfo(ctx context.Context, conditionID string) (ClobMarketInfoResponse, error) {
	c.mu.RLock()
	gen := c.generation
	c.mu.RUnlock()
	return c.clobMarketInfo(ctx, conditionID, gen)
}

func (c *Client) clobMarketInfo(ctx context.Context, conditionID string, gen uint64) (ClobMarketInfoResponse, error) {
	if _, err := ParseB256(conditionID); err != nil {
		return ClobMarketInfoResponse{}, fmt.Errorf("condition ID: %w", err)
	}
	r, err := getMarket[ClobMarketInfoResponse](c, ctx, "clob-markets/"+url.PathEscape(conditionID), nil, nil)
	if err != nil {
		return r, err
	}
	if !strings.EqualFold(conditionID, string(r.ConditionID)) {
		return ClobMarketInfoResponse{}, fmt.Errorf("market metadata condition does not match requested condition %s", conditionID)
	}
	info := FeeInfo{}
	if r.FeeDetails != nil {
		info = FeeInfo{Rate: r.FeeDetails.Rate, Exponent: r.FeeDetails.Exponent}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == gen {
		for _, token := range r.Tokens {
			if token != nil {
				id := cacheTokenID(token.TokenID)
				c.tickSizes[id] = r.MinTickSize
				if r.negRiskPresent {
					c.negRisks[id] = r.NegRisk
				}
				c.feeInfos[id] = info
				c.tokenConditions[id] = r.ConditionID
			}
		}
	}
	return r, nil
}
func (c *Client) MarketByToken(ctx context.Context, id U256) (MarketByTokenResponse, error) {
	id, err := ParseU256(id.String())
	if err != nil {
		return MarketByTokenResponse{}, err
	}
	return getMarket[MarketByTokenResponse](c, ctx, "markets-by-token/"+id.String(), nil, nil)
}

// FeeInfo returns V2 fee parameters, loading market metadata if necessary.
func (c *Client) FeeInfo(ctx context.Context, id U256) (FeeInfo, error) {
	id, err := ParseU256(id.String())
	if err != nil {
		return FeeInfo{}, err
	}
	return c.orderFeeInfo(ctx, id)
}
func (c *Client) FeeExponent(ctx context.Context, id U256) (uint32, error) {
	info, err := c.FeeInfo(ctx, id)
	return info.Exponent, err
}
func (c *Client) MarketTradesEvents(ctx context.Context, conditionID string) (json.RawMessage, error) {
	return getMarket[json.RawMessage](c, ctx, "markets/live-activity/"+url.PathEscape(conditionID), nil, nil)
}
func (c *Client) CalculateMarketPrice(ctx context.Context, id U256, side Side, amount Amount, orderType OrderType) (Decimal, error) {
	id, err := ParseU256(string(id))
	if err != nil {
		return "", fmt.Errorf("asset ID: %w", err)
	}
	book, err := c.OrderBook(ctx, OrderBookSummaryRequest{TokenID: id})
	if err != nil {
		return "", err
	}
	bookID, err := ParseU256(string(book.AssetID))
	if err != nil || bookID != id {
		return "", fmt.Errorf("order book asset ID %q does not match requested asset %s", book.AssetID, id)
	}
	if side != Buy && side != Sell {
		return "", NewValidationError(fmt.Sprintf("invalid side: %s", side))
	}
	if side == Sell && amount.IsUSDC() {
		return "", NewValidationError("SELL orders must specify their amount in shares, not USDC")
	}
	entries := book.Asks
	if side == Sell {
		entries = book.Bids
	}
	target, ok := new(big.Rat).SetString(amount.Value().String())
	if !ok {
		return "", NewValidationError("invalid market amount")
	}
	total := new(big.Rat)
	for i := len(entries) - 1; i >= 0; i-- {
		size, ok := new(big.Rat).SetString(entries[i].Size.String())
		if !ok {
			return "", NewValidationError("invalid book size")
		}
		if amount.IsUSDC() {
			price, ok := new(big.Rat).SetString(entries[i].Price.String())
			if !ok {
				return "", NewValidationError("invalid book price")
			}
			size.Mul(size, price)
		}
		total.Add(total, size)
		if total.Cmp(target) >= 0 {
			return entries[i].Price, nil
		}
	}
	if len(entries) != 0 && orderType != FOK {
		return entries[0].Price, nil
	}
	return "", NewValidationError(fmt.Sprintf("insufficient liquidity to fill %s on %s", amount.Value(), side))
}
