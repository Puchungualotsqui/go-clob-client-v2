package gamma

type TeamsRequest struct {
	Limit        *int32   `query:"limit,omitempty"`
	Offset       *int32   `query:"offset,omitempty"`
	Order        *string  `query:"order,omitempty"`
	Ascending    *bool    `query:"ascending,omitempty"`
	League       []string `query:"league,omitempty"`
	Name         []string `query:"name,omitempty"`
	Abbreviation []string `query:"abbreviation,omitempty"`
}

type TagsRequest struct {
	Limit           *int32  `query:"limit,omitempty"`
	Offset          *int32  `query:"offset,omitempty"`
	Order           *string `query:"order,omitempty"`
	Ascending       *bool   `query:"ascending,omitempty"`
	IncludeTemplate *bool   `query:"include_template,omitempty"`
	IsCarousel      *bool   `query:"is_carousel,omitempty"`
}

type TagByIdRequest struct {
	ID              string `query:"-"`
	IncludeTemplate *bool  `query:"include_template,omitempty"`
}

type TagBySlugRequest struct {
	Slug            string `query:"-"`
	IncludeTemplate *bool  `query:"include_template,omitempty"`
}

type RelatedTagsByIdRequest struct {
	ID        string  `query:"-"`
	OmitEmpty *bool   `query:"omit_empty,omitempty"`
	Status    *string `query:"status,omitempty"`
}

type RelatedTagsBySlugRequest struct {
	Slug      string  `query:"-"`
	OmitEmpty *bool   `query:"omit_empty,omitempty"`
	Status    *string `query:"status,omitempty"`
}

// EventsRequest date filters must be RFC3339 timestamps and numeric filters
// must be exactly representable by Rust's Decimal when sent by the client.
type EventsRequest struct {
	Limit           *int32   `query:"limit,omitempty"`
	Offset          *int32   `query:"offset,omitempty"`
	Order           []string `query:"order,omitempty"`
	Ascending       *bool    `query:"ascending,omitempty"`
	ID              []string `query:"id,omitempty"`
	TagID           *string  `query:"tag_id,omitempty"`
	ExcludeTagID    []string `query:"exclude_tag_id,omitempty"`
	Slug            []string `query:"slug,omitempty"`
	TagSlug         *string  `query:"tag_slug,omitempty"`
	RelatedTags     *bool    `query:"related_tags,omitempty"`
	Active          *bool    `query:"active,omitempty"`
	Archived        *bool    `query:"archived,omitempty"`
	Featured        *bool    `query:"featured,omitempty"`
	Cyom            *bool    `query:"cyom,omitempty"`
	IncludeChat     *bool    `query:"include_chat,omitempty"`
	IncludeTemplate *bool    `query:"include_template,omitempty"`
	Recurrence      *string  `query:"recurrence,omitempty"`
	Closed          *bool    `query:"closed,omitempty"`
	LiquidityMin    *Number  `query:"liquidity_min,omitempty"`
	LiquidityMax    *Number  `query:"liquidity_max,omitempty"`
	VolumeMin       *Number  `query:"volume_min,omitempty"`
	VolumeMax       *Number  `query:"volume_max,omitempty"`
	StartDateMin    *string  `query:"start_date_min,omitempty"`
	StartDateMax    *string  `query:"start_date_max,omitempty"`
	EndDateMin      *string  `query:"end_date_min,omitempty"`
	EndDateMax      *string  `query:"end_date_max,omitempty"`
}

type EventByIdRequest struct {
	ID              string `query:"-"`
	IncludeChat     *bool  `query:"include_chat,omitempty"`
	IncludeTemplate *bool  `query:"include_template,omitempty"`
}

type EventBySlugRequest struct {
	Slug            string `query:"-"`
	IncludeChat     *bool  `query:"include_chat,omitempty"`
	IncludeTemplate *bool  `query:"include_template,omitempty"`
}

type EventTagsRequest struct {
	ID string `query:"-"`
}

// MarketsRequest validates typed token IDs, hashes, addresses, dates, and
// decimal filters before the client sends a request. ID and slug remain opaque.
type MarketsRequest struct {
	Limit               *int32   `query:"limit,omitempty"`
	Offset              *int32   `query:"offset,omitempty"`
	Order               *string  `query:"order,omitempty"`
	Ascending           *bool    `query:"ascending,omitempty"`
	ID                  []string `query:"id,omitempty"`
	Slug                []string `query:"slug,omitempty"`
	CLOBTokenIDs        []Number `query:"clob_token_ids,omitempty"`
	ConditionIDs        []string `query:"condition_ids,omitempty"`
	MarketMakerAddress  []string `query:"market_maker_address,omitempty"`
	LiquidityNumMin     *Number  `query:"liquidity_num_min,omitempty"`
	LiquidityNumMax     *Number  `query:"liquidity_num_max,omitempty"`
	VolumeNumMin        *Number  `query:"volume_num_min,omitempty"`
	VolumeNumMax        *Number  `query:"volume_num_max,omitempty"`
	StartDateMin        *string  `query:"start_date_min,omitempty"`
	StartDateMax        *string  `query:"start_date_max,omitempty"`
	EndDateMin          *string  `query:"end_date_min,omitempty"`
	EndDateMax          *string  `query:"end_date_max,omitempty"`
	TagID               *string  `query:"tag_id,omitempty"`
	RelatedTags         *bool    `query:"related_tags,omitempty"`
	Cyom                *bool    `query:"cyom,omitempty"`
	UmaResolutionStatus *string  `query:"uma_resolution_status,omitempty"`
	GameID              *string  `query:"game_id,omitempty"`
	SportsMarketTypes   []string `query:"sports_market_types,omitempty"`
	RewardsMinSize      *Number  `query:"rewards_min_size,omitempty"`
	QuestionIDs         []string `query:"question_ids,omitempty"`
	IncludeTag          *bool    `query:"include_tag,omitempty"`
	Closed              *bool    `query:"closed,omitempty"`
}

type MarketByIdRequest struct {
	ID         string `query:"-"`
	IncludeTag *bool  `query:"include_tag,omitempty"`
}

type MarketBySlugRequest struct {
	Slug       string `query:"-"`
	IncludeTag *bool  `query:"include_tag,omitempty"`
}

type MarketTagsRequest struct {
	ID string `query:"-"`
}

type SeriesListRequest struct {
	Limit            *int32   `query:"limit,omitempty"`
	Offset           *int32   `query:"offset,omitempty"`
	Order            *string  `query:"order,omitempty"`
	Ascending        *bool    `query:"ascending,omitempty"`
	Slug             []string `query:"slug,omitempty"`
	CategoriesIDs    []string `query:"categories_ids,omitempty"`
	CategoriesLabels []string `query:"categories_labels,omitempty"`
	Closed           *bool    `query:"closed,omitempty"`
	IncludeChat      *bool    `query:"include_chat,omitempty"`
	Recurrence       *string  `query:"recurrence,omitempty"`
}

type SeriesByIdRequest struct {
	ID          string `query:"-"`
	IncludeChat *bool  `query:"include_chat,omitempty"`
}

type CommentsRequest struct {
	ParentEntityType string  `query:"parent_entity_type"`
	ParentEntityID   string  `query:"parent_entity_id"`
	Limit            *int32  `query:"limit,omitempty"`
	Offset           *int32  `query:"offset,omitempty"`
	Order            *string `query:"order,omitempty"`
	Ascending        *bool   `query:"ascending,omitempty"`
	GetPositions     *bool   `query:"get_positions,omitempty"`
	HoldersOnly      *bool   `query:"holders_only,omitempty"`
}

type CommentsByIdRequest struct {
	ID           string `query:"-"`
	GetPositions *bool  `query:"get_positions,omitempty"`
}

// CommentsByUserAddressRequest requires a 20-byte 0x-prefixed UserAddress.
type CommentsByUserAddressRequest struct {
	UserAddress string  `query:"-"`
	Limit       *int32  `query:"limit,omitempty"`
	Offset      *int32  `query:"offset,omitempty"`
	Order       *string `query:"order,omitempty"`
	Ascending   *bool   `query:"ascending,omitempty"`
}

// PublicProfileRequest requires a 20-byte 0x-prefixed Address.
type PublicProfileRequest struct {
	Address string `query:"address"`
}

type SearchRequest struct {
	Q                 string   `query:"q"`
	Cache             *bool    `query:"cache,omitempty"`
	EventsStatus      *string  `query:"events_status,omitempty"`
	LimitPerType      *int32   `query:"limit_per_type,omitempty"`
	Page              *int32   `query:"page,omitempty"`
	EventsTag         []string `query:"events_tag,omitempty"`
	KeepClosedMarkets *int32   `query:"keep_closed_markets,omitempty"`
	Sort              *string  `query:"sort,omitempty"`
	Ascending         *bool    `query:"ascending,omitempty"`
	SearchTags        *bool    `query:"search_tags,omitempty"`
	SearchProfiles    *bool    `query:"search_profiles,omitempty"`
	Recurrence        *string  `query:"recurrence,omitempty"`
	ExcludeTagID      []string `query:"exclude_tag_id,omitempty"`
	Optimized         *bool    `query:"optimized,omitempty"`
}
