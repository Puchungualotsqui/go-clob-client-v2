package data

import (
	"context"
	"net/http"
	"net/url"
)

var _ = url.PathEscape

func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.do(ctx, http.MethodGet, "", nil, nil, &out, false)
	return out, err
}

func (c *Client) Positions(ctx context.Context, req PositionsRequest) ([]Position, error) {
	var out []Position
	err := c.do(ctx, http.MethodGet, "positions", req, nil, &out, false)
	return out, err
}

func (c *Client) Trades(ctx context.Context, req TradesRequest) ([]Trade, error) {
	var out []Trade
	err := c.do(ctx, http.MethodGet, "trades", req, nil, &out, false)
	return out, err
}

func (c *Client) Activity(ctx context.Context, req ActivityRequest) ([]Activity, error) {
	var out []Activity
	err := c.do(ctx, http.MethodGet, "activity", req, nil, &out, false)
	return out, err
}

func (c *Client) Holders(ctx context.Context, req HoldersRequest) ([]MetaHolder, error) {
	var out []MetaHolder
	err := c.do(ctx, http.MethodGet, "holders", req, nil, &out, false)
	return out, err
}

func (c *Client) Value(ctx context.Context, req ValueRequest) ([]Value, error) {
	var out []Value
	err := c.do(ctx, http.MethodGet, "value", req, nil, &out, false)
	return out, err
}

func (c *Client) ClosedPositions(ctx context.Context, req ClosedPositionsRequest) ([]ClosedPosition, error) {
	var out []ClosedPosition
	err := c.do(ctx, http.MethodGet, "closed-positions", req, nil, &out, false)
	return out, err
}

func (c *Client) Leaderboard(ctx context.Context, req TraderLeaderboardRequest) ([]TraderLeaderboardEntry, error) {
	var out []TraderLeaderboardEntry
	err := c.do(ctx, http.MethodGet, "v1/leaderboard", req, nil, &out, false)
	return out, err
}

func (c *Client) Traded(ctx context.Context, req TradedRequest) (Traded, error) {
	var out Traded
	err := c.do(ctx, http.MethodGet, "traded", req, nil, &out, false)
	return out, err
}

func (c *Client) OpenInterest(ctx context.Context, req OpenInterestRequest) ([]OpenInterest, error) {
	var out []OpenInterest
	err := c.do(ctx, http.MethodGet, "oi", req, nil, &out, false)
	return out, err
}

func (c *Client) LiveVolume(ctx context.Context, req LiveVolumeRequest) ([]LiveVolume, error) {
	var out []LiveVolume
	err := c.do(ctx, http.MethodGet, "live-volume", req, nil, &out, false)
	return out, err
}

func (c *Client) BuilderLeaderboard(ctx context.Context, req BuilderLeaderboardRequest) ([]BuilderLeaderboardEntry, error) {
	var out []BuilderLeaderboardEntry
	err := c.do(ctx, http.MethodGet, "v1/builders/leaderboard", req, nil, &out, false)
	return out, err
}

func (c *Client) BuilderVolume(ctx context.Context, req BuilderVolumeRequest) ([]BuilderVolumeEntry, error) {
	var out []BuilderVolumeEntry
	err := c.do(ctx, http.MethodGet, "v1/builders/volume", req, nil, &out, false)
	return out, err
}
