package bridge

import (
	"context"
	"net/http"
	"net/url"
)

func (c *Client) Deposit(ctx context.Context, req DepositRequest) (DepositResponse, error) {
	var out DepositResponse
	err := c.do(ctx, http.MethodPost, "deposit", nil, req, &out, false)
	return out, err
}
func (c *Client) Withdraw(ctx context.Context, req WithdrawRequest) (WithdrawResponse, error) {
	var out WithdrawResponse
	err := c.do(ctx, http.MethodPost, "withdraw", nil, req, &out, false)
	return out, err
}
func (c *Client) SupportedAssets(ctx context.Context) (SupportedAssetsResponse, error) {
	var out SupportedAssetsResponse
	err := c.do(ctx, http.MethodGet, "supported-assets", nil, nil, &out, false)
	return out, err
}
func (c *Client) Status(ctx context.Context, req StatusRequest) (StatusResponse, error) {
	var out StatusResponse
	err := c.do(ctx, http.MethodGet, "status/"+url.PathEscape(req.Address), nil, nil, &out, false)
	return out, err
}
func (c *Client) Quote(ctx context.Context, req QuoteRequest) (QuoteResponse, error) {
	var out QuoteResponse
	err := c.do(ctx, http.MethodPost, "quote", nil, req, &out, false)
	return out, err
}
