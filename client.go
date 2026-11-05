package clob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Client is a concurrency-safe CLOB HTTP client. Do not copy it after use.
type Client struct {
	host            string
	geoblockHost    string
	config          Config
	httpClient      *http.Client
	mu              sync.RWMutex
	generation      uint64
	tickSizes       map[U256]TickSize
	negRisks        map[U256]bool
	feeRates        map[U256]FeeRateResponse
	feeInfos        map[U256]FeeInfo
	tokenConditions map[U256]B256
	builderFeeRates map[B256]BuilderFeeRateResponse
	version         uint32
	versionRequest  uint64
	versionCached   uint64
}

// NewClient creates a client. An empty host selects DefaultCLOBHost. An optional
// HTTP client can be supplied for custom transport, timeout, or testing.
func NewClient(host string, config Config, clients ...*http.Client) (*Client, error) {
	if host == "" {
		host = DefaultCLOBHost
	}
	host, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}
	geo := DefaultGeoblockHost
	if config.GeoblockHost != nil {
		geo = *config.GeoblockHost
	}
	geo, err = normalizeHost(geo)
	if err != nil {
		return nil, err
	}
	if len(clients) > 1 || (len(clients) == 1 && clients[0] == nil) {
		return nil, fmt.Errorf("expected at most one non-nil HTTP client")
	}
	hc := http.DefaultClient
	if len(clients) == 1 {
		hc = clients[0]
	}
	if config.GeoblockHost != nil {
		v := *config.GeoblockHost
		config.GeoblockHost = &v
	}
	if config.BuilderCode != nil {
		v := *config.BuilderCode
		config.BuilderCode = &v
	}
	return &Client{host: host, geoblockHost: geo, config: config, httpClient: hc, tickSizes: make(map[U256]TickSize), negRisks: make(map[U256]bool), feeRates: make(map[U256]FeeRateResponse), feeInfos: make(map[U256]FeeInfo), tokenConditions: make(map[U256]B256), builderFeeRates: make(map[B256]BuilderFeeRateResponse)}, nil
}

func normalizeHost(host string) (string, error) {
	u, err := url.Parse(host)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("invalid API host %q", host)
	}
	return strings.TrimRight(host, "/"), nil
}

func (c *Client) Host() string { return c.host }

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	config := c.config
	if config.GeoblockHost != nil {
		v := *config.GeoblockHost
		config.GeoblockHost = &v
	}
	if config.BuilderCode != nil {
		v := *config.BuilderCode
		config.BuilderCode = &v
	}
	return config
}

// NewRequest builds a request with an already serialized body, suitable for
// attaching authentication headers computed over those exact bytes. Path is
// relative to the configured CLOB host and may contain escaped path segments.
func (c *Client) NewRequest(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Request, error) {
	return c.newRequest(ctx, c.host, method, path, query, body)
}
func (c *Client) newRequest(ctx context.Context, host, method, path string, query url.Values, body []byte) (*http.Request, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	endpoint := host + "/" + strings.TrimPrefix(path, "/")
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go_clob_client")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// Do executes a request, returning a structured status error for non-2xx
// responses. A non-nil dst receives a JSON response; JSON null is a 404, as in
// the Rust client. A nil dst discards a successful response body. Requests with
// POLY authentication headers do not follow redirects, because their signatures
// are URL-specific and redirects could expose credentials to another host.
func (c *Client) Do(req *http.Request, dst any) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}
	hc := c.httpClient
	if req.Header.Get("POLY_SIGNATURE") != "" || req.Header.Get("POLY_API_KEY") != "" || req.Header.Get("POLY_PASSPHRASE") != "" {
		client := *hc
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		hc = &client
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewStatusError(resp.StatusCode, req.Method, req.URL.EscapedPath(), string(data))
	}
	if dst == nil {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return NewStatusError(http.StatusNotFound, req.Method, req.URL.EscapedPath(), "Unable to find requested resource")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decoding %s %s response: %w", req.Method, req.URL.EscapedPath(), err)
	}
	return nil
}

func (c *Client) request(ctx context.Context, host, method, path string, query url.Values, body any, dst any) error {
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
	}
	req, err := c.newRequest(ctx, host, method, path, query, b)
	if err != nil {
		return err
	}
	return c.Do(req, dst)
}

func queryValues(request any, cursor *string) (url.Values, error) {
	if request == nil {
		request = struct{}{}
	}
	s, err := QueryParams(request, cursor)
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(strings.TrimPrefix(s, "?"))
}

// InvalidateInternalCaches clears all market metadata and the cached version.
func (c *Client) InvalidateInternalCaches() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	clear(c.tickSizes)
	clear(c.negRisks)
	clear(c.feeRates)
	clear(c.feeInfos)
	clear(c.tokenConditions)
	clear(c.builderFeeRates)
	c.version = 0
}
func cacheTokenID(id U256) U256 {
	v, err := ParseU256(id.String())
	if err != nil {
		return id
	}
	return v
}
func (c *Client) SetTickSize(id U256, size TickSize) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.tickSizes[cacheTokenID(id)] = size
}
func (c *Client) SetNegRisk(id U256, neg bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.negRisks[cacheTokenID(id)] = neg
}
func (c *Client) SetFeeRateBPS(id U256, bps uint32) { c.SetFeeRate(id, FeeRateResponse{BaseFee: bps}) }
func (c *Client) SetFeeRate(id U256, rate FeeRateResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.feeRates[cacheTokenID(id)] = rate
}
