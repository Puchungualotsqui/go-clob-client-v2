package data

type PositionsRequest struct {
	User          string        `query:"user"`
	Filter        *MarketFilter `query:",inline"`
	SizeThreshold *Number       `query:"sizeThreshold,omitempty"`
	Redeemable    *bool         `query:"redeemable,omitempty"`
	Mergeable     *bool         `query:"mergeable,omitempty"`
	Limit         *int32        `query:"limit,omitempty"`
	Offset        *int32        `query:"offset,omitempty"`
	SortBy        *string       `query:"sortBy,omitempty"`
	SortDirection *string       `query:"sortDirection,omitempty"`
	Title         *string       `query:"title,omitempty"`
}

type TradesRequest struct {
	User        *string       `query:"user,omitempty"`
	Filter      *MarketFilter `query:",inline"`
	Limit       *int32        `query:"limit,omitempty"`
	Offset      *int32        `query:"offset,omitempty"`
	TakerOnly   *bool         `query:"takerOnly,omitempty"`
	TradeFilter *TradeFilter  `query:",inline"`
	Side        *string       `query:"side,omitempty"`
}

type ActivityRequest struct {
	User          string        `query:"user"`
	Filter        *MarketFilter `query:",inline"`
	ActivityTypes []string      `query:"type,omitempty,comma"`
	Limit         *int32        `query:"limit,omitempty"`
	Offset        *int32        `query:"offset,omitempty"`
	Start         *uint64       `query:"start,omitempty"`
	End           *uint64       `query:"end,omitempty"`
	SortBy        *string       `query:"sortBy,omitempty"`
	SortDirection *string       `query:"sortDirection,omitempty"`
	Side          *string       `query:"side,omitempty"`
}

type HoldersRequest struct {
	Markets    []string `query:"market,omitempty,comma"`
	Limit      *int32   `query:"limit,omitempty"`
	MinBalance *int32   `query:"minBalance,omitempty"`
}

type TradedRequest struct {
	User string `query:"user"`
}

type ValueRequest struct {
	User    string   `query:"user"`
	Markets []string `query:"market,omitempty,comma"`
}

type OpenInterestRequest struct {
	Markets []string `query:"market,omitempty,comma"`
}

type LiveVolumeRequest struct {
	ID uint64 `query:"id"`
}

type ClosedPositionsRequest struct {
	User          string        `query:"user"`
	Filter        *MarketFilter `query:",inline"`
	Title         *string       `query:"title,omitempty"`
	Limit         *int32        `query:"limit,omitempty"`
	Offset        *int32        `query:"offset,omitempty"`
	SortBy        *string       `query:"sortBy,omitempty"`
	SortDirection *string       `query:"sortDirection,omitempty"`
}

type BuilderLeaderboardRequest struct {
	TimePeriod *string `query:"timePeriod,omitempty"`
	Limit      *int32  `query:"limit,omitempty"`
	Offset     *int32  `query:"offset,omitempty"`
}

type BuilderVolumeRequest struct {
	TimePeriod *string `query:"timePeriod,omitempty"`
}

type TraderLeaderboardRequest struct {
	Category   *string `query:"category,omitempty"`
	TimePeriod *string `query:"timePeriod,omitempty"`
	OrderBy    *string `query:"orderBy,omitempty"`
	Limit      *int32  `query:"limit,omitempty"`
	Offset     *int32  `query:"offset,omitempty"`
	User       *string `query:"user,omitempty"`
	UserName   *string `query:"userName,omitempty"`
}
