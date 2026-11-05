package gamma

import (
	"context"
	"net/http"
	"net/url"
)

var _ = url.PathEscape

func (c *Client) Teams(ctx context.Context, req TeamsRequest) ([]Team, error) {
	var out []Team
	err := c.do(ctx, http.MethodGet, "teams", req, nil, &out, false)
	return out, err
}

func (c *Client) Sports(ctx context.Context) ([]SportsMetadata, error) {
	var out []SportsMetadata
	err := c.do(ctx, http.MethodGet, "sports", nil, nil, &out, false)
	return out, err
}

func (c *Client) SportsMarketTypes(ctx context.Context) (SportsMarketTypesResponse, error) {
	var out SportsMarketTypesResponse
	err := c.do(ctx, http.MethodGet, "sports/market-types", nil, nil, &out, false)
	return out, err
}

func (c *Client) Tags(ctx context.Context, req TagsRequest) ([]Tag, error) {
	var out []Tag
	err := c.do(ctx, http.MethodGet, "tags", req, nil, &out, false)
	return out, err
}

func (c *Client) TagByID(ctx context.Context, req TagByIdRequest) (Tag, error) {
	var out Tag
	err := c.do(ctx, http.MethodGet, "tags/"+url.PathEscape(req.ID), req, nil, &out, false)
	return out, err
}

func (c *Client) TagBySlug(ctx context.Context, req TagBySlugRequest) (Tag, error) {
	var out Tag
	err := c.do(ctx, http.MethodGet, "tags/slug/"+url.PathEscape(req.Slug), req, nil, &out, false)
	return out, err
}

func (c *Client) RelatedTagsByID(ctx context.Context, req RelatedTagsByIdRequest) ([]RelatedTag, error) {
	var out []RelatedTag
	err := c.do(ctx, http.MethodGet, "tags/"+url.PathEscape(req.ID)+"/related-tags", req, nil, &out, false)
	return out, err
}

func (c *Client) RelatedTagsBySlug(ctx context.Context, req RelatedTagsBySlugRequest) ([]RelatedTag, error) {
	var out []RelatedTag
	err := c.do(ctx, http.MethodGet, "tags/slug/"+url.PathEscape(req.Slug)+"/related-tags", req, nil, &out, false)
	return out, err
}

func (c *Client) TagsRelatedToTagByID(ctx context.Context, req RelatedTagsByIdRequest) ([]Tag, error) {
	var out []Tag
	err := c.do(ctx, http.MethodGet, "tags/"+url.PathEscape(req.ID)+"/related-tags/tags", req, nil, &out, false)
	return out, err
}

func (c *Client) TagsRelatedToTagBySlug(ctx context.Context, req RelatedTagsBySlugRequest) ([]Tag, error) {
	var out []Tag
	err := c.do(ctx, http.MethodGet, "tags/slug/"+url.PathEscape(req.Slug)+"/related-tags/tags", req, nil, &out, false)
	return out, err
}

func (c *Client) Events(ctx context.Context, req EventsRequest) ([]Event, error) {
	var out []Event
	err := c.do(ctx, http.MethodGet, "events", req, nil, &out, false)
	return out, err
}

func (c *Client) EventByID(ctx context.Context, req EventByIdRequest) (Event, error) {
	var out Event
	err := c.do(ctx, http.MethodGet, "events/"+url.PathEscape(req.ID), req, nil, &out, false)
	return out, err
}

func (c *Client) EventBySlug(ctx context.Context, req EventBySlugRequest) (Event, error) {
	var out Event
	err := c.do(ctx, http.MethodGet, "events/slug/"+url.PathEscape(req.Slug), req, nil, &out, false)
	return out, err
}

func (c *Client) EventTags(ctx context.Context, req EventTagsRequest) ([]Tag, error) {
	var out []Tag
	err := c.do(ctx, http.MethodGet, "events/"+url.PathEscape(req.ID)+"/tags", req, nil, &out, false)
	return out, err
}

func (c *Client) Markets(ctx context.Context, req MarketsRequest) ([]Market, error) {
	var out []Market
	err := c.do(ctx, http.MethodGet, "markets", req, nil, &out, false)
	return out, err
}

func (c *Client) MarketByID(ctx context.Context, req MarketByIdRequest) (Market, error) {
	var out Market
	err := c.do(ctx, http.MethodGet, "markets/"+url.PathEscape(req.ID), req, nil, &out, false)
	return out, err
}

func (c *Client) MarketBySlug(ctx context.Context, req MarketBySlugRequest) (Market, error) {
	var out Market
	err := c.do(ctx, http.MethodGet, "markets/slug/"+url.PathEscape(req.Slug), req, nil, &out, false)
	return out, err
}

func (c *Client) MarketTags(ctx context.Context, req MarketTagsRequest) ([]Tag, error) {
	var out []Tag
	err := c.do(ctx, http.MethodGet, "markets/"+url.PathEscape(req.ID)+"/tags", req, nil, &out, false)
	return out, err
}

func (c *Client) Series(ctx context.Context, req SeriesListRequest) ([]Series, error) {
	var out []Series
	err := c.do(ctx, http.MethodGet, "series", req, nil, &out, false)
	return out, err
}

func (c *Client) SeriesByID(ctx context.Context, req SeriesByIdRequest) (Series, error) {
	var out Series
	err := c.do(ctx, http.MethodGet, "series/"+url.PathEscape(req.ID), req, nil, &out, false)
	return out, err
}

func (c *Client) Comments(ctx context.Context, req CommentsRequest) ([]Comment, error) {
	var out []Comment
	err := c.do(ctx, http.MethodGet, "comments", req, nil, &out, false)
	return out, err
}

func (c *Client) CommentsByID(ctx context.Context, req CommentsByIdRequest) ([]Comment, error) {
	var out []Comment
	err := c.do(ctx, http.MethodGet, "comments/"+url.PathEscape(req.ID), req, nil, &out, false)
	return out, err
}

func (c *Client) CommentsByUserAddress(ctx context.Context, req CommentsByUserAddressRequest) ([]Comment, error) {
	var out []Comment
	err := c.do(ctx, http.MethodGet, "comments/user_address/"+url.PathEscape(req.UserAddress), req, nil, &out, false)
	return out, err
}

func (c *Client) PublicProfile(ctx context.Context, req PublicProfileRequest) (PublicProfile, error) {
	var out PublicProfile
	err := c.do(ctx, http.MethodGet, "public-profile", req, nil, &out, false)
	return out, err
}

func (c *Client) Search(ctx context.Context, req SearchRequest) (SearchResults, error) {
	var out SearchResults
	err := c.do(ctx, http.MethodGet, "public-search", req, nil, &out, false)
	return out, err
}

func (c *Client) Status(ctx context.Context) (string, error) {
	var s string
	err := c.do(ctx, http.MethodGet, "status", nil, nil, &s, true)
	return s, err
}
