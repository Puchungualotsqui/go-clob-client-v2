package clob

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"strings"
	"sync"
	"time"
)

// AuthenticatedClientOptions configures the independent L2 HTTP client. Do not
// modify HTTPClient concurrently with requests. A nil client uses http.DefaultClient.
type AuthenticatedClientOptions struct {
	HTTPClient        *http.Client
	SignatureType     SignatureType
	UseServerTime     bool
	HeartbeatInterval time.Duration
}

// AuthenticatedClient owns its transport configuration and can be used concurrently.
// Its credentials are copied at construction.
type AuthenticatedClient struct {
	host              string
	httpClient        *http.Client
	address           Address
	credentials       Credentials
	publicClient      *Client
	orderIdentity     *authenticatedOrderBinding
	builderCache      *Client
	signatureType     SignatureType
	useServerTime     bool
	heartbeatInterval time.Duration
	heartbeatMu       sync.Mutex
	heartbeatCancel   context.CancelFunc
	heartbeatDone     chan struct{}
}

// NewAuthenticatedClient creates an L2 client using existing API credentials.
// An empty host selects DefaultCLOBHost. At most one options value is accepted.
func NewAuthenticatedClient(host string, address Address, credentials Credentials, options ...AuthenticatedClientOptions) (*AuthenticatedClient, error) {
	if host == "" {
		host = DefaultCLOBHost
	}
	u, err := url.Parse(host)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, fmt.Errorf("invalid CLOB host")
	}
	if len(options) > 1 {
		return nil, fmt.Errorf("at most one options value may be supplied")
	}
	var o AuthenticatedClientOptions
	if len(options) == 1 {
		o = options[0]
	}
	if o.HeartbeatInterval < 0 {
		return nil, fmt.Errorf("heartbeat interval must not be negative")
	}
	if o.HeartbeatInterval == 0 {
		o.HeartbeatInterval = 5 * time.Second
	}
	if o.HTTPClient == nil {
		o.HTTPClient = http.DefaultClient
	}
	// Never forward API credentials or signed request bodies on redirects.
	hc := *o.HTTPClient
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &AuthenticatedClient{host: strings.TrimRight(host, "/"), httpClient: &hc, address: address, credentials: credentials, builderCache: &Client{builderFeeRates: make(map[B256]BuilderFeeRateResponse)}, signatureType: o.SignatureType, useServerTime: o.UseServerTime, heartbeatInterval: o.HeartbeatInterval}, nil
}

func (c *AuthenticatedClient) do(ctx context.Context, method, path string, query any, cursor *string, body, result any) error {
	var params string
	var err error
	if query != nil || cursor != nil {
		if query == nil {
			query = struct{}{}
		}
		params, err = QueryParams(query, cursor)
		if err != nil {
			return err
		}
	}
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+"/"+path+params, bytes.NewReader(data))
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	if c.useServerTime {
		ts, err = c.serverTime(ctx)
		if err != nil {
			return err
		}
	}
	headers, err := L2Headers(c.address, c.credentials, ts, method, req.URL, data)
	if err != nil {
		return err
	}
	req.Header = headers
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.execute(req, result)
}

func (c *AuthenticatedClient) execute(req *http.Request, result any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return NewStatusError(resp.StatusCode, req.Method, req.URL.EscapedPath(), string(b))
	}
	if result == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return NewStatusError(http.StatusNotFound, req.Method, req.URL.EscapedPath(), "Unable to find requested resource")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode %s %s response: %w", req.Method, req.URL.EscapedPath(), err)
	}
	return nil
}

func (c *AuthenticatedClient) serverTime(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+"/time", nil)
	if err != nil {
		return 0, err
	}
	var ts int64
	err = c.execute(req, &ts)
	return ts, err
}

func (c *AuthenticatedClient) APIKeys(ctx context.Context) (APIKeysResponse, error) {
	var r APIKeysResponse
	err := c.do(ctx, "GET", "auth/api-keys", nil, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) DeleteAPIKey(ctx context.Context) (json.RawMessage, error) {
	var r json.RawMessage
	err := c.do(ctx, "DELETE", "auth/api-key", nil, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) ClosedOnlyMode(ctx context.Context) (BanStatusResponse, error) {
	var r BanStatusResponse
	err := c.do(ctx, "GET", "auth/ban-status/closed-only", nil, nil, nil, &r)
	return r, err
}

const (
	resolveTradesTimeout      = 30 * time.Second
	resolveTradesPollInterval = 250 * time.Millisecond
)

// PostOrder submits a signed order and resolves settlement hashes on a best-effort
// basis. Polling may take up to 30 seconds and respects the caller's context.
// Clients created by Client.Authenticate reject mismatched order identity and owner.
func (c *AuthenticatedClient) PostOrder(ctx context.Context, order SignedOrder) (PostOrderResponse, error) {
	var r PostOrderResponse
	if err := c.validateSignedOrderIdentity(order); err != nil {
		return r, err
	}
	err := c.do(ctx, "POST", "order", nil, nil, order, &r)
	if err != nil {
		c.refreshVersionOnMismatch(ctx, err)
		return r, err
	}
	responses := []PostOrderResponse{r}
	c.resolveTransactionHashes(ctx, responses, []SignedOrder{order})
	return responses[0], nil
}

// PostOrders validates all bound order identities before submitting the batch,
// then resolves eligible settlement hashes using one shared polling window.
func (c *AuthenticatedClient) PostOrders(ctx context.Context, orders []SignedOrder) ([]PostOrderResponse, error) {
	for i, order := range orders {
		if err := c.validateSignedOrderIdentity(order); err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
	}
	if orders == nil {
		orders = []SignedOrder{}
	}
	var r []PostOrderResponse
	err := c.do(ctx, "POST", "orders", nil, nil, orders, &r)
	if err != nil {
		c.refreshVersionOnMismatch(ctx, err)
		return r, err
	}
	c.resolveTransactionHashes(ctx, r, orders)
	return r, nil
}

func (c *AuthenticatedClient) refreshVersionOnMismatch(ctx context.Context, err error) {
	var status StatusError
	if c.publicClient != nil && errors.As(err, &status) && strings.Contains(status.Message, "order_version_mismatch") {
		_, _ = c.publicClient.ResolveVersion(ctx, true)
	}
}

func (c *AuthenticatedClient) resolveTransactionHashes(ctx context.Context, responses []PostOrderResponse, orders []SignedOrder) {
	eligible := func(i int) bool {
		return (i >= len(orders) || orders[i].DeferExec == nil || !*orders[i].DeferExec) && len(responses[i].TransactionHashes) == 0
	}
	var ids []string
	seen := make(map[string]bool)
	for i, response := range responses {
		if eligible(i) {
			for _, id := range response.TradeIDs {
				if id != "" && !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
	}
	if len(ids) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, resolveTradesTimeout)
	defer cancel()
	resolved := make(map[string]TradeResponse, len(ids))
	for {
		for _, id := range ids {
			if _, ok := resolved[id]; ok {
				continue
			}
			if ctx.Err() != nil {
				break
			}
			page, err := c.Trades(ctx, TradesRequest{ID: &id}, nil)
			if err != nil {
				continue
			}
			for _, trade := range page.Data {
				if trade.ID == id && (trade.Status == TradeFailed || nonzeroSettlementHash(trade.TransactionHash)) {
					resolved[id] = trade
					break
				}
			}
		}
		if len(resolved) == len(ids) || ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(resolveTradesPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			break
		}
	}
	for i := range responses {
		if !eligible(i) {
			continue
		}
		for _, id := range responses[i].TradeIDs {
			if trade, ok := resolved[id]; ok && trade.Status != TradeFailed && nonzeroSettlementHash(trade.TransactionHash) {
				responses[i].TransactionHashes = append(responses[i].TransactionHashes, trade.TransactionHash)
			}
		}
	}
}

func nonzeroSettlementHash(hash B256) bool {
	_, err := ParseB256(string(hash))
	return err == nil && strings.Trim(string(hash)[2:], "0") != ""
}
func (c *AuthenticatedClient) Order(ctx context.Context, id string) (OpenOrderResponse, error) {
	var r OpenOrderResponse
	err := c.do(ctx, "GET", "data/order/"+url.PathEscape(id), nil, nil, nil, &r)
	return r, err
}

// Orders returns one page; pass its NextCursor to fetch the next page.
func (c *AuthenticatedClient) Orders(ctx context.Context, request OrdersRequest, cursor *string) (Page[OpenOrderResponse], error) {
	var r Page[OpenOrderResponse]
	err := c.do(ctx, "GET", "data/orders", request, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelOrder(ctx context.Context, id string) (CancelOrdersResponse, error) {
	var r CancelOrdersResponse
	err := c.do(ctx, "DELETE", "order", nil, nil, struct {
		ID string `json:"orderID"`
	}{id}, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelOrders(ctx context.Context, ids []string) (CancelOrdersResponse, error) {
	if ids == nil {
		ids = []string{}
	}
	var r CancelOrdersResponse
	err := c.do(ctx, "DELETE", "orders", nil, nil, ids, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelAllOrders(ctx context.Context) (CancelOrdersResponse, error) {
	var r CancelOrdersResponse
	err := c.do(ctx, "DELETE", "cancel-all", nil, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelMarketOrders(ctx context.Context, request CancelMarketOrderRequest) (CancelOrdersResponse, error) {
	var r CancelOrdersResponse
	err := c.do(ctx, "DELETE", "cancel-market-orders", nil, nil, request, &r)
	return r, err
}
func (c *AuthenticatedClient) Trades(ctx context.Context, request TradesRequest, cursor *string) (Page[TradeResponse], error) {
	var r Page[TradeResponse]
	err := c.do(ctx, "GET", "data/trades", request, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) Notifications(ctx context.Context) ([]NotificationResponse, error) {
	var r []NotificationResponse
	err := c.do(ctx, "GET", "notifications", c.signatureQuery(), nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) DeleteNotifications(ctx context.Context, request DeleteNotificationsRequest) error {
	return c.do(ctx, "DELETE", "notifications", request, nil, nil, nil)
}
func (c *AuthenticatedClient) BalanceAllowance(ctx context.Context, request BalanceAllowanceRequest) (BalanceAllowanceResponse, error) {
	if request.SignatureType == nil {
		request.SignatureType = &c.signatureType
	}
	var r BalanceAllowanceResponse
	err := c.do(ctx, "GET", "balance-allowance", request, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) UpdateBalanceAllowance(ctx context.Context, request UpdateBalanceAllowanceRequest) error {
	if request.SignatureType == nil {
		request.SignatureType = &c.signatureType
	}
	return c.do(ctx, "GET", "balance-allowance/update", request, nil, nil, nil)
}
func (c *AuthenticatedClient) IsOrderScoring(ctx context.Context, id string) (OrderScoringResponse, error) {
	var r OrderScoringResponse
	err := c.do(ctx, "GET", "order-scoring", struct {
		ID string `json:"order_id"`
	}{id}, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) AreOrdersScoring(ctx context.Context, ids []string) (OrdersScoringResponse, error) {
	if ids == nil {
		ids = []string{}
	}
	var r OrdersScoringResponse
	err := c.do(ctx, "POST", "orders-scoring", nil, nil, ids, &r)
	return r, err
}
func (c *AuthenticatedClient) signatureQuery() map[string]any {
	return map[string]any{"signature_type": uint8(c.signatureType)}
}
func (c *AuthenticatedClient) EarningsForUserForDay(ctx context.Context, date string, cursor *string) (Page[UserEarningResponse], error) {
	q := c.signatureQuery()
	q["date"] = date
	var r Page[UserEarningResponse]
	err := c.do(ctx, "GET", "rewards/user", q, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) TotalEarningsForUserForDay(ctx context.Context, date string) ([]TotalUserEarningResponse, error) {
	q := c.signatureQuery()
	q["date"] = date
	var r []TotalUserEarningResponse
	err := c.do(ctx, "GET", "rewards/user/total", q, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) UserEarningsAndMarketsConfig(ctx context.Context, request UserRewardsEarningRequest, cursor *string) (Page[UserRewardsEarningResponse], error) {
	q := struct {
		UserRewardsEarningRequest
		SignatureType uint8 `json:"signature_type"`
	}{request, uint8(c.signatureType)}
	var r Page[UserRewardsEarningResponse]
	err := c.do(ctx, "GET", "rewards/user/markets", q, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) RewardPercentages(ctx context.Context) (RewardsPercentagesResponse, error) {
	var r RewardsPercentagesResponse
	err := c.do(ctx, "GET", "rewards/user/percentages", c.signatureQuery(), nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) CurrentRewards(ctx context.Context, cursor *string) (Page[CurrentRewardResponse], error) {
	var r Page[CurrentRewardResponse]
	err := c.do(ctx, "GET", "rewards/markets/current", nil, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) RawRewardsForMarket(ctx context.Context, conditionID string, cursor *string) (Page[MarketRewardResponse], error) {
	var r Page[MarketRewardResponse]
	err := c.do(ctx, "GET", "rewards/markets/"+url.PathEscape(conditionID), nil, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) CreateReadonlyAPIKey(ctx context.Context) (ReadonlyAPIKeyResponse, error) {
	var r ReadonlyAPIKeyResponse
	err := c.do(ctx, "POST", "auth/readonly-api-key", nil, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) ReadonlyAPIKeys(ctx context.Context) ([]string, error) {
	var r struct {
		Keys []string `json:"readonlyApiKeys"`
	}
	err := c.do(ctx, "GET", "auth/readonly-api-keys", nil, nil, nil, &r)
	return r.Keys, err
}
func (c *AuthenticatedClient) DeleteReadonlyAPIKey(ctx context.Context, key string) error {
	return c.do(ctx, "DELETE", "auth/readonly-api-key", nil, nil, struct {
		Key string `json:"key"`
	}{key}, nil)
}
func (c *AuthenticatedClient) PreMigrationOrders(ctx context.Context, cursor *string) (Page[OpenOrderResponse], error) {
	var r Page[OpenOrderResponse]
	err := c.do(ctx, "GET", "data/pre-migration-orders", nil, cursor, nil, &r)
	return r, err
}

// BuilderFeeRate returns authenticated builder fees in basis points. Both maker
// and taker rates must be present, including when either rate is zero. Results
// are cached per credentials and builder code until the public client's caches
// are invalidated (or for the lifetime of a directly constructed client).
func (c *AuthenticatedClient) BuilderFeeRate(ctx context.Context, code B256) (BuilderFeeRateResponse, error) {
	if c == nil || c.builderCache == nil || ctx == nil {
		return BuilderFeeRateResponse{}, fmt.Errorf("authenticated client and context are required")
	}
	parsed, err := ParseB256(string(code))
	if err != nil {
		return BuilderFeeRateResponse{}, err
	}
	codeBytes, err := hex.DecodeString(string(parsed)[2:])
	if err != nil {
		return BuilderFeeRateResponse{}, err
	}
	// Authenticated clients may share a public cache; do not share responses
	// across identities or retain credentials as cache keys.
	keyHash := keccak([]byte("order-builder-fee-cache"), codeBytes, c.address[:], hashString(c.credentials.Key), hashString(c.credentials.Secret), hashString(c.credentials.Passphrase))
	key := B256("0x" + hex.EncodeToString(keyHash[:]))
	cache := c.builderCache
	cache.mu.RLock()
	r, ok := cache.builderFeeRates[key]
	gen := cache.generation
	cache.mu.RUnlock()
	if ok {
		return r, nil
	}
	var wire struct {
		Maker      *uint32 `json:"builderMakerFeeRateBps"`
		Taker      *uint32 `json:"builderTakerFeeRateBps"`
		MakerAlias *uint32 `json:"builder_maker_fee_rate_bps"`
		TakerAlias *uint32 `json:"builder_taker_fee_rate_bps"`
	}
	if err := c.do(ctx, "GET", "fees/builder-fees/"+url.PathEscape(string(parsed)), nil, nil, nil, &wire); err != nil {
		return BuilderFeeRateResponse{}, err
	}
	if wire.Taker != nil && wire.TakerAlias != nil || wire.Maker != nil && wire.MakerAlias != nil {
		return BuilderFeeRateResponse{}, fmt.Errorf("builder fee response contains duplicate fee fields")
	}
	if wire.Taker == nil {
		wire.Taker = wire.TakerAlias
	}
	if wire.Maker == nil {
		wire.Maker = wire.MakerAlias
	}
	if wire.Taker == nil {
		return BuilderFeeRateResponse{}, fmt.Errorf("builder fee response is missing taker fee")
	}
	if wire.Maker == nil {
		return BuilderFeeRateResponse{}, fmt.Errorf("builder fee response is missing maker fee")
	}
	r = BuilderFeeRateResponse{BuilderMakerFeeRateBPS: *wire.Maker, BuilderTakerFeeRateBPS: *wire.Taker}
	cache.mu.Lock()
	if cache.generation == gen {
		cache.builderFeeRates[key] = r
	}
	cache.mu.Unlock()
	return r, nil
}
func (c *AuthenticatedClient) CreateBuilderAPIKey(ctx context.Context) (Credentials, error) {
	var r l1Credentials
	err := c.do(ctx, "POST", "auth/builder-api-key", nil, nil, nil, &r)
	return Credentials(r), err
}
func (c *AuthenticatedClient) BuilderAPIKeys(ctx context.Context) ([]BuilderAPIKeyResponse, error) {
	var r []BuilderAPIKeyResponse
	err := c.do(ctx, "GET", "auth/builder-api-key", nil, nil, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) RevokeBuilderAPIKey(ctx context.Context) error {
	return c.do(ctx, "DELETE", "auth/builder-api-key", nil, nil, nil, nil)
}
func (c *AuthenticatedClient) BuilderTrades(ctx context.Context, code B256, request TradesRequest, cursor *string) (Page[BuilderTradeResponse], error) {
	var r Page[BuilderTradeResponse]
	parsed, err := ParseB256(string(code))
	if err != nil {
		return r, err
	}
	if strings.TrimLeft(string(parsed)[2:], "0") == "" {
		return r, NewValidationError("builder_code is required and cannot be zero")
	}
	q := struct {
		TradesRequest
		BuilderCode B256 `json:"builder_code"`
	}{request, parsed}
	err = c.do(ctx, "GET", "builder/trades", q, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) PostHeartbeat(ctx context.Context, id *string) (HeartbeatResponse, error) {
	var r HeartbeatResponse
	err := c.do(ctx, "POST", "v1/heartbeats", nil, nil, struct {
		ID *string `json:"heartbeat_id"`
	}{id}, &r)
	return r, err
}

// StartHeartbeats posts periodically until ctx is canceled or StopHeartbeats is called.
// Failed posts are retried on the next tick with the last successful ID.
func (c *AuthenticatedClient) StartHeartbeats(ctx context.Context) error {
	if ctx == nil {
		return NewValidationError("context is required")
	}
	c.heartbeatMu.Lock()
	defer c.heartbeatMu.Unlock()
	if c.heartbeatDone != nil {
		return NewValidationError("heartbeat task already active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.heartbeatCancel, c.heartbeatDone = cancel, done
	go func() {
		defer func() {
			c.heartbeatMu.Lock()
			if c.heartbeatDone == done {
				c.heartbeatCancel = nil
				c.heartbeatDone = nil
			}
			close(done)
			c.heartbeatMu.Unlock()
		}()
		ticker := time.NewTicker(c.heartbeatInterval)
		defer ticker.Stop()
		var id *string
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				response, err := c.PostHeartbeat(ctx, id)
				if err == nil {
					id = &response.HeartbeatID
				}
			}
		}
	}()
	return nil
}
func (c *AuthenticatedClient) HeartbeatsActive() bool {
	c.heartbeatMu.Lock()
	defer c.heartbeatMu.Unlock()
	return c.heartbeatDone != nil
}

// StopHeartbeats cancels any in-flight heartbeat and waits for the worker to exit.
func (c *AuthenticatedClient) StopHeartbeats() {
	c.heartbeatMu.Lock()
	cancel, done := c.heartbeatCancel, c.heartbeatDone
	if cancel != nil {
		cancel()
	}
	c.heartbeatMu.Unlock()
	if done != nil {
		<-done
	}
}

func (c *AuthenticatedClient) CreateRequest(ctx context.Context, request CreateRFQRequestRequest) (CreateRFQRequestResponse, error) {
	var r CreateRFQRequestResponse
	err := c.do(ctx, "POST", "rfq/request", nil, nil, request, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelRequest(ctx context.Context, request CancelRFQRequestRequest) error {
	return c.do(ctx, "DELETE", "rfq/request", nil, nil, request, nil)
}
func (c *AuthenticatedClient) Requests(ctx context.Context, request RFQRequestsRequest, cursor *string) (Page[RFQRequest], error) {
	var r Page[RFQRequest]
	err := c.do(ctx, "GET", "rfq/data/requests", request, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) CreateQuote(ctx context.Context, request CreateRFQQuoteRequest) (CreateRFQQuoteResponse, error) {
	var r CreateRFQQuoteResponse
	err := c.do(ctx, "POST", "rfq/quote", nil, nil, request, &r)
	return r, err
}
func (c *AuthenticatedClient) CancelQuote(ctx context.Context, request CancelRFQQuoteRequest) error {
	return c.do(ctx, "DELETE", "rfq/quote", nil, nil, request, nil)
}
func (c *AuthenticatedClient) Quotes(ctx context.Context, request RFQQuotesRequest, cursor *string) (Page[RFQQuote], error) {
	var r Page[RFQQuote]
	err := c.do(ctx, "GET", "rfq/data/quotes", request, cursor, nil, &r)
	return r, err
}
func (c *AuthenticatedClient) AcceptQuote(ctx context.Context, request AcceptRFQQuoteRequest) (AcceptRFQQuoteResponse, error) {
	var r AcceptRFQQuoteResponse
	err := c.do(ctx, "POST", "rfq/request/accept", nil, nil, request, nil)
	return r, err
}
func (c *AuthenticatedClient) ApproveOrder(ctx context.Context, request ApproveRFQOrderRequest) (ApproveRFQOrderResponse, error) {
	var r ApproveRFQOrderResponse
	err := c.do(ctx, "POST", "rfq/quote/approve", nil, nil, request, &r)
	return r, err
}
