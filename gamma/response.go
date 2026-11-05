package gamma

type ImageOptimization struct {
	ID                        *string  `json:"id,omitempty"`
	ImageURLSource            *string  `json:"imageUrlSource,omitempty"`
	ImageURLOptimized         *string  `json:"imageUrlOptimized,omitempty"`
	ImageSizeKbSource         *float64 `json:"imageSizeKbSource,omitempty"`
	ImageSizeKbOptimized      *float64 `json:"imageSizeKbOptimized,omitempty"`
	ImageOptimizedComplete    *bool    `json:"imageOptimizedComplete,omitempty"`
	ImageOptimizedLastUpdated *string  `json:"imageOptimizedLastUpdated,omitempty"`
	RelID                     *string  `json:"relID,omitempty"`
	Field                     *string  `json:"field,omitempty"`
	Relname                   *string  `json:"relname,omitempty"`
}

type Pagination struct {
	HasMore      *bool  `json:"hasMore,omitempty"`
	TotalResults *int32 `json:"totalResults,omitempty"`
}

type Team struct {
	ID           int32   `json:"id"`
	Name         *string `json:"name,omitempty"`
	League       *string `json:"league,omitempty"`
	Record       *string `json:"record,omitempty"`
	Logo         *string `json:"logo,omitempty"`
	Abbreviation *string `json:"abbreviation,omitempty"`
	Alias        *string `json:"alias,omitempty"`
	CreatedAt    *string `json:"createdAt,omitempty"`
	UpdatedAt    *string `json:"updatedAt,omitempty"`
	Color        *string `json:"color,omitempty"`
	ProviderID   *int32  `json:"providerId,omitempty"`
}

type SportsMetadata struct {
	ID         *int32       `json:"id,omitempty"`
	Sport      string       `json:"sport"`
	Image      string       `json:"image"`
	Resolution string       `json:"resolution"`
	Ordering   string       `json:"ordering"`
	Tags       CommaStrings `json:"tags"`
	Series     string       `json:"series"`
	CreatedAt  *string      `json:"createdAt,omitempty"`
}

type SportsMarketTypesResponse struct {
	MarketTypes []string `json:"marketTypes"`
}

type Tag struct {
	ID                  string  `json:"id"`
	Label               *string `json:"label,omitempty"`
	Slug                *string `json:"slug,omitempty"`
	ForceShow           *bool   `json:"forceShow,omitempty"`
	PublishedAt         *string `json:"publishedAt,omitempty"`
	CreatedBy           *int32  `json:"createdBy,omitempty"`
	UpdatedBy           *int32  `json:"updatedBy,omitempty"`
	CreatedAt           *string `json:"createdAt,omitempty"`
	UpdatedAt           *string `json:"updatedAt,omitempty"`
	ForceHide           *bool   `json:"forceHide,omitempty"`
	IsCarousel          *bool   `json:"isCarousel,omitempty"`
	RequiresTranslation *bool   `json:"requiresTranslation,omitempty"`
}

type RelatedTag struct {
	ID           StringFromAny  `json:"id"`
	TagID        *StringFromAny `json:"tagID,omitempty"`
	RelatedTagID *StringFromAny `json:"relatedTagID,omitempty"`
	Rank         *int32         `json:"rank,omitempty"`
}

type Category struct {
	ID             string  `json:"id"`
	Label          *string `json:"label,omitempty"`
	ParentCategory *string `json:"parentCategory,omitempty"`
	Slug           *string `json:"slug,omitempty"`
	PublishedAt    *string `json:"publishedAt,omitempty"`
	CreatedBy      *string `json:"createdBy,omitempty"`
	UpdatedBy      *string `json:"updatedBy,omitempty"`
	CreatedAt      *string `json:"createdAt,omitempty"`
	UpdatedAt      *string `json:"updatedAt,omitempty"`
}

type EventCreator struct {
	ID            string  `json:"id"`
	CreatorName   *string `json:"creatorName,omitempty"`
	CreatorHandle *string `json:"creatorHandle,omitempty"`
	CreatorURL    *string `json:"creatorUrl,omitempty"`
	CreatorImage  *string `json:"creatorImage,omitempty"`
	CreatedAt     *string `json:"createdAt,omitempty"`
	UpdatedAt     *string `json:"updatedAt,omitempty"`
}

type Chat struct {
	ID           string  `json:"id"`
	ChannelID    *string `json:"channelId,omitempty"`
	ChannelName  *string `json:"channelName,omitempty"`
	ChannelImage *string `json:"channelImage,omitempty"`
	Live         *bool   `json:"live,omitempty"`
	StartTime    *string `json:"startTime,omitempty"`
	EndTime      *string `json:"endTime,omitempty"`
}

type Template struct {
	ID               string  `json:"id"`
	EventTitle       *string `json:"eventTitle,omitempty"`
	EventSlug        *string `json:"eventSlug,omitempty"`
	EventImage       *string `json:"eventImage,omitempty"`
	MarketTitle      *string `json:"marketTitle,omitempty"`
	Description      *string `json:"description,omitempty"`
	ResolutionSource *string `json:"resolutionSource,omitempty"`
	NegRisk          *bool   `json:"negRisk,omitempty"`
	SortBy           *string `json:"sortBy,omitempty"`
	ShowMarketImages *bool   `json:"showMarketImages,omitempty"`
	SeriesSlug       *string `json:"seriesSlug,omitempty"`
	Outcomes         *string `json:"outcomes,omitempty"`
}

type Collection struct {
	ID                   string             `json:"id"`
	Ticker               *string            `json:"ticker,omitempty"`
	Slug                 *string            `json:"slug,omitempty"`
	Title                *string            `json:"title,omitempty"`
	Subtitle             *string            `json:"subtitle,omitempty"`
	CollectionType       *string            `json:"collectionType,omitempty"`
	Description          *string            `json:"description,omitempty"`
	Tags                 *string            `json:"tags,omitempty"`
	Image                *string            `json:"image,omitempty"`
	Icon                 *string            `json:"icon,omitempty"`
	HeaderImage          *string            `json:"headerImage,omitempty"`
	Layout               *string            `json:"layout,omitempty"`
	Active               *bool              `json:"active,omitempty"`
	Closed               *bool              `json:"closed,omitempty"`
	Archived             *bool              `json:"archived,omitempty"`
	New                  *bool              `json:"new,omitempty"`
	Featured             *bool              `json:"featured,omitempty"`
	Restricted           *bool              `json:"restricted,omitempty"`
	IsTemplate           *bool              `json:"isTemplate,omitempty"`
	TemplateVariables    *string            `json:"templateVariables,omitempty"`
	PublishedAt          *string            `json:"publishedAt,omitempty"`
	CreatedBy            *string            `json:"createdBy,omitempty"`
	UpdatedBy            *string            `json:"updatedBy,omitempty"`
	CreatedAt            *string            `json:"createdAt,omitempty"`
	UpdatedAt            *string            `json:"updatedAt,omitempty"`
	CommentsEnabled      *bool              `json:"commentsEnabled,omitempty"`
	ImageOptimized       *ImageOptimization `json:"imageOptimized,omitempty"`
	IconOptimized        *ImageOptimization `json:"iconOptimized,omitempty"`
	HeaderImageOptimized *ImageOptimization `json:"headerImageOptimized,omitempty"`
}

type Event struct {
	ID                           string             `json:"id"`
	Ticker                       *string            `json:"ticker,omitempty"`
	Slug                         *string            `json:"slug,omitempty"`
	Title                        *string            `json:"title,omitempty"`
	Subtitle                     *string            `json:"subtitle,omitempty"`
	Description                  *string            `json:"description,omitempty"`
	ResolutionSource             *string            `json:"resolutionSource,omitempty"`
	StartDate                    *string            `json:"startDate,omitempty"`
	CreationDate                 *string            `json:"creationDate,omitempty"`
	EndDate                      *string            `json:"endDate,omitempty"`
	Image                        *string            `json:"image,omitempty"`
	Icon                         *string            `json:"icon,omitempty"`
	Active                       *bool              `json:"active,omitempty"`
	Closed                       *bool              `json:"closed,omitempty"`
	Archived                     *bool              `json:"archived,omitempty"`
	New                          *bool              `json:"new,omitempty"`
	Featured                     *bool              `json:"featured,omitempty"`
	Restricted                   *bool              `json:"restricted,omitempty"`
	Liquidity                    *Number            `json:"liquidity,omitempty"`
	Volume                       *Number            `json:"volume,omitempty"`
	OpenInterest                 *Number            `json:"openInterest,omitempty"`
	SortBy                       *string            `json:"sortBy,omitempty"`
	Category                     *string            `json:"category,omitempty"`
	Subcategory                  *string            `json:"subcategory,omitempty"`
	IsTemplate                   *bool              `json:"isTemplate,omitempty"`
	TemplateVariables            *string            `json:"templateVariables,omitempty"`
	PublishedAt                  *string            `json:"publishedAt,omitempty"`
	CreatedBy                    *string            `json:"createdBy,omitempty"`
	UpdatedBy                    *string            `json:"updatedBy,omitempty"`
	CreatedAt                    *string            `json:"createdAt,omitempty"`
	UpdatedAt                    *string            `json:"updatedAt,omitempty"`
	CommentsEnabled              *bool              `json:"commentsEnabled,omitempty"`
	Competitive                  *Number            `json:"competitive,omitempty"`
	Volume24hr                   *Number            `json:"volume24hr,omitempty"`
	Volume1wk                    *Number            `json:"volume1wk,omitempty"`
	Volume1mo                    *Number            `json:"volume1mo,omitempty"`
	Volume1yr                    *Number            `json:"volume1yr,omitempty"`
	FeaturedImage                *string            `json:"featuredImage,omitempty"`
	DisqusThread                 *string            `json:"disqusThread,omitempty"`
	ParentEvent                  *string            `json:"parentEvent,omitempty"`
	ParentEventID                *StringFromAny     `json:"parentEventId,omitempty"`
	SportsradarMatchID           *string            `json:"sportsradarMatchId,omitempty"`
	TurnProviderID               *StringFromAny     `json:"turnProviderId,omitempty"`
	EnableOrderBook              *bool              `json:"enableOrderBook,omitempty"`
	LiquidityAMM                 *Number            `json:"liquidityAmm,omitempty"`
	LiquidityCLOB                *Number            `json:"liquidityClob,omitempty"`
	NegRisk                      *bool              `json:"negRisk,omitempty"`
	NegRiskMarketID              *string            `json:"negRiskMarketID,omitempty" empty:"nil"`
	NegRiskFeeBips               *int32             `json:"negRiskFeeBips,omitempty"`
	CommentCount                 *int32             `json:"commentCount,omitempty"`
	ImageOptimized               *ImageOptimization `json:"imageOptimized,omitempty"`
	IconOptimized                *ImageOptimization `json:"iconOptimized,omitempty"`
	FeaturedImageOptimized       *ImageOptimization `json:"featuredImageOptimized,omitempty"`
	SubEvents                    *[]string          `json:"subEvents,omitempty"`
	Markets                      *[]Market          `json:"markets,omitempty"`
	Series                       *[]Series          `json:"series,omitempty"`
	Categories                   *[]Category        `json:"categories,omitempty"`
	Collections                  *[]Collection      `json:"collections,omitempty"`
	Tags                         *[]Tag             `json:"tags,omitempty"`
	Cyom                         *bool              `json:"cyom,omitempty"`
	ClosedTime                   *string            `json:"closedTime,omitempty"`
	ShowAllOutcomes              *bool              `json:"showAllOutcomes,omitempty"`
	ShowMarketImages             *bool              `json:"showMarketImages,omitempty"`
	AutomaticallyResolved        *bool              `json:"automaticallyResolved,omitempty"`
	EnableNegRisk                *bool              `json:"enableNegRisk,omitempty"`
	AutomaticallyActive          *bool              `json:"automaticallyActive,omitempty"`
	EventDate                    *string            `json:"eventDate,omitempty"`
	StartTime                    *string            `json:"startTime,omitempty"`
	EventWeek                    *int32             `json:"eventWeek,omitempty"`
	SeriesSlug                   *string            `json:"seriesSlug,omitempty"`
	Score                        *string            `json:"score,omitempty"`
	Elapsed                      *string            `json:"elapsed,omitempty"`
	Period                       *string            `json:"period,omitempty"`
	Live                         *bool              `json:"live,omitempty"`
	Ended                        *bool              `json:"ended,omitempty"`
	FinishedTimestamp            *string            `json:"finishedTimestamp,omitempty"`
	GmpChartMode                 *string            `json:"gmpChartMode,omitempty"`
	EventCreators                *[]EventCreator    `json:"eventCreators,omitempty"`
	TweetCount                   *int32             `json:"tweetCount,omitempty"`
	Chats                        *[]Chat            `json:"chats,omitempty"`
	FeaturedOrder                *int32             `json:"featuredOrder,omitempty"`
	EstimateValue                *bool              `json:"estimateValue,omitempty"`
	CantEstimate                 *bool              `json:"cantEstimate,omitempty"`
	EstimatedValue               *string            `json:"estimatedValue,omitempty"`
	Templates                    *[]Template        `json:"templates,omitempty"`
	SpreadsMainLine              *Number            `json:"spreadsMainLine,omitempty"`
	TotalsMainLine               *Number            `json:"totalsMainLine,omitempty"`
	CarouselMap                  *string            `json:"carouselMap,omitempty"`
	PendingDeployment            *bool              `json:"pendingDeployment,omitempty"`
	Deploying                    *bool              `json:"deploying,omitempty"`
	DeployingTimestamp           *string            `json:"deployingTimestamp,omitempty"`
	ScheduledDeploymentTimestamp *string            `json:"scheduledDeploymentTimestamp,omitempty"`
	GameStatus                   *string            `json:"gameStatus,omitempty"`
	RequiresTranslation          *bool              `json:"requiresTranslation,omitempty"`
	NegRiskAugmented             *bool              `json:"negRiskAugmented,omitempty"`
	GameID                       *int64             `json:"gameId,omitempty"`
	ElectionType                 *string            `json:"electionType,omitempty"`
	CountryName                  *string            `json:"countryName,omitempty"`
	Color                        *string            `json:"color,omitempty"`
	CumulativeMarkets            *bool              `json:"cumulativeMarkets,omitempty"`
	AwayTeamName                 *string            `json:"awayTeamName,omitempty"`
	HomeTeamName                 *string            `json:"homeTeamName,omitempty"`
}

type Market struct {
	ID                           string             `json:"id"`
	Question                     *string            `json:"question,omitempty"`
	ConditionID                  *string            `json:"conditionId,omitempty" empty:"nil"`
	Slug                         *string            `json:"slug,omitempty"`
	TwitterCardImage             *string            `json:"twitterCardImage,omitempty"`
	ResolutionSource             *string            `json:"resolutionSource,omitempty"`
	EndDate                      *string            `json:"endDate,omitempty"`
	Category                     *string            `json:"category,omitempty"`
	AMMType                      *string            `json:"ammType,omitempty"`
	Liquidity                    *Number            `json:"liquidity,omitempty"`
	SponsorName                  *string            `json:"sponsorName,omitempty"`
	SponsorImage                 *string            `json:"sponsorImage,omitempty"`
	StartDate                    *string            `json:"startDate,omitempty"`
	XAxisValue                   *string            `json:"xAxisValue,omitempty"`
	YAxisValue                   *string            `json:"yAxisValue,omitempty"`
	DenominationToken            *Number            `json:"denominationToken,omitempty"`
	Fee                          *Number            `json:"fee,omitempty"`
	Image                        *string            `json:"image,omitempty"`
	Icon                         *string            `json:"icon,omitempty"`
	LowerBound                   *string            `json:"lowerBound,omitempty"`
	UpperBound                   *string            `json:"upperBound,omitempty"`
	Description                  *string            `json:"description,omitempty"`
	Outcomes                     *StringList        `json:"outcomes,omitempty"`
	OutcomePrices                *NumberList        `json:"outcomePrices,omitempty"`
	Volume                       *Number            `json:"volume,omitempty"`
	Active                       *bool              `json:"active,omitempty"`
	MarketType                   *string            `json:"marketType,omitempty"`
	FormatType                   *string            `json:"formatType,omitempty"`
	LowerBoundDate               *string            `json:"lowerBoundDate,omitempty"`
	UpperBoundDate               *string            `json:"upperBoundDate,omitempty"`
	Closed                       *bool              `json:"closed,omitempty"`
	MarketMakerAddress           *string            `json:"marketMakerAddress,omitempty" empty:"nil"`
	CreatedBy                    *int32             `json:"createdBy,omitempty"`
	UpdatedBy                    *int32             `json:"updatedBy,omitempty"`
	CreatedAt                    *string            `json:"createdAt,omitempty"`
	UpdatedAt                    *string            `json:"updatedAt,omitempty"`
	ClosedTime                   *string            `json:"closedTime,omitempty"`
	WideFormat                   *bool              `json:"wideFormat,omitempty"`
	New                          *bool              `json:"new,omitempty"`
	MailchimpTag                 *string            `json:"mailchimpTag,omitempty"`
	Featured                     *bool              `json:"featured,omitempty"`
	Archived                     *bool              `json:"archived,omitempty"`
	ResolvedBy                   *string            `json:"resolvedBy,omitempty"`
	Restricted                   *bool              `json:"restricted,omitempty"`
	MarketGroup                  *int32             `json:"marketGroup,omitempty"`
	GroupItemTitle               *string            `json:"groupItemTitle,omitempty"`
	GroupItemThreshold           *string            `json:"groupItemThreshold,omitempty"`
	QuestionID                   *string            `json:"questionID,omitempty" empty:"nil"`
	UmaEndDate                   *string            `json:"umaEndDate,omitempty"`
	EnableOrderBook              *bool              `json:"enableOrderBook,omitempty"`
	OrderPriceMinTickSize        *Number            `json:"orderPriceMinTickSize,omitempty"`
	OrderMinSize                 *Number            `json:"orderMinSize,omitempty"`
	UmaResolutionStatus          *string            `json:"umaResolutionStatus,omitempty"`
	CurationOrder                *int32             `json:"curationOrder,omitempty"`
	VolumeNum                    *Number            `json:"volumeNum,omitempty"`
	LiquidityNum                 *Number            `json:"liquidityNum,omitempty"`
	EndDateIso                   *string            `json:"endDateIso,omitempty"`
	StartDateIso                 *string            `json:"startDateIso,omitempty"`
	UmaEndDateIso                *string            `json:"umaEndDateIso,omitempty"`
	HasReviewedDates             *bool              `json:"hasReviewedDates,omitempty"`
	ReadyForCron                 *bool              `json:"readyForCron,omitempty"`
	CommentsEnabled              *bool              `json:"commentsEnabled,omitempty"`
	Volume24hr                   *Number            `json:"volume24hr,omitempty"`
	Volume1wk                    *Number            `json:"volume1wk,omitempty"`
	Volume1mo                    *Number            `json:"volume1mo,omitempty"`
	Volume1yr                    *Number            `json:"volume1yr,omitempty"`
	GameStartTime                *string            `json:"gameStartTime,omitempty"`
	SecondsDelay                 *int32             `json:"secondsDelay,omitempty"`
	CLOBTokenIDs                 *NumberList        `json:"clobTokenIds,omitempty"`
	DisqusThread                 *string            `json:"disqusThread,omitempty"`
	ShortOutcomes                *string            `json:"shortOutcomes,omitempty"`
	TeamAID                      *string            `json:"teamAID,omitempty"`
	TeamBID                      *string            `json:"teamBID,omitempty"`
	UmaBond                      *string            `json:"umaBond,omitempty"`
	UmaReward                    *Number            `json:"umaReward,omitempty"`
	FpmmLive                     *bool              `json:"fpmmLive,omitempty"`
	Volume24hrAMM                *Number            `json:"volume24hrAmm,omitempty"`
	Volume1wkAMM                 *Number            `json:"volume1wkAmm,omitempty"`
	Volume1moAMM                 *Number            `json:"volume1moAmm,omitempty"`
	Volume1yrAMM                 *Number            `json:"volume1yrAmm,omitempty"`
	Volume24hrCLOB               *Number            `json:"volume24hrClob,omitempty"`
	Volume1wkCLOB                *Number            `json:"volume1wkClob,omitempty"`
	Volume1moCLOB                *Number            `json:"volume1moClob,omitempty"`
	Volume1yrCLOB                *Number            `json:"volume1yrClob,omitempty"`
	VolumeAMM                    *Number            `json:"volumeAmm,omitempty"`
	VolumeCLOB                   *Number            `json:"volumeClob,omitempty"`
	LiquidityAMM                 *Number            `json:"liquidityAmm,omitempty"`
	LiquidityCLOB                *Number            `json:"liquidityClob,omitempty"`
	MakerBaseFee                 *int32             `json:"makerBaseFee,omitempty"`
	TakerBaseFee                 *int32             `json:"takerBaseFee,omitempty"`
	MakerRebatesFeeShareBps      *int32             `json:"makerRebatesFeeShareBps,omitempty"`
	CustomLiveness               *int32             `json:"customLiveness,omitempty"`
	AcceptingOrders              *bool              `json:"acceptingOrders,omitempty"`
	NotificationsEnabled         *bool              `json:"notificationsEnabled,omitempty"`
	Score                        *int32             `json:"score,omitempty"`
	ImageOptimized               *ImageOptimization `json:"imageOptimized,omitempty"`
	IconOptimized                *ImageOptimization `json:"iconOptimized,omitempty"`
	Events                       *[]Event           `json:"events,omitempty"`
	Categories                   *[]Category        `json:"categories,omitempty"`
	Tags                         *[]Tag             `json:"tags,omitempty"`
	Creator                      *string            `json:"creator,omitempty"`
	Ready                        *bool              `json:"ready,omitempty"`
	Funded                       *bool              `json:"funded,omitempty"`
	PastSlugs                    *string            `json:"pastSlugs,omitempty"`
	ReadyTimestamp               *string            `json:"readyTimestamp,omitempty"`
	FundedTimestamp              *string            `json:"fundedTimestamp,omitempty"`
	AcceptingOrdersTimestamp     *string            `json:"acceptingOrdersTimestamp,omitempty"`
	Competitive                  *Number            `json:"competitive,omitempty"`
	RewardsMinSize               *Number            `json:"rewardsMinSize,omitempty"`
	RewardsMaxSpread             *Number            `json:"rewardsMaxSpread,omitempty"`
	Spread                       *Number            `json:"spread,omitempty"`
	AutomaticallyResolved        *bool              `json:"automaticallyResolved,omitempty"`
	OneDayPriceChange            *Number            `json:"oneDayPriceChange,omitempty"`
	OneHourPriceChange           *Number            `json:"oneHourPriceChange,omitempty"`
	OneWeekPriceChange           *Number            `json:"oneWeekPriceChange,omitempty"`
	OneMonthPriceChange          *Number            `json:"oneMonthPriceChange,omitempty"`
	OneYearPriceChange           *Number            `json:"oneYearPriceChange,omitempty"`
	LastTradePrice               *Number            `json:"lastTradePrice,omitempty"`
	BestBid                      *Number            `json:"bestBid,omitempty"`
	BestAsk                      *Number            `json:"bestAsk,omitempty"`
	AutomaticallyActive          *bool              `json:"automaticallyActive,omitempty"`
	ClearBookOnStart             *bool              `json:"clearBookOnStart,omitempty"`
	ChartColor                   *string            `json:"chartColor,omitempty"`
	SeriesColor                  *string            `json:"seriesColor,omitempty"`
	ShowGmpSeries                *bool              `json:"showGmpSeries,omitempty"`
	ShowGmpOutcome               *bool              `json:"showGmpOutcome,omitempty"`
	ManualActivation             *bool              `json:"manualActivation,omitempty"`
	NegRiskOther                 *bool              `json:"negRiskOther,omitempty"`
	GameID                       *string            `json:"gameId,omitempty"`
	GroupItemRange               *string            `json:"groupItemRange,omitempty"`
	SportsMarketType             *string            `json:"sportsMarketType,omitempty"`
	Line                         *Number            `json:"line,omitempty"`
	UmaResolutionStatuses        *string            `json:"umaResolutionStatuses,omitempty"`
	PendingDeployment            *bool              `json:"pendingDeployment,omitempty"`
	Deploying                    *bool              `json:"deploying,omitempty"`
	DeployingTimestamp           *string            `json:"deployingTimestamp,omitempty"`
	ScheduledDeploymentTimestamp *string            `json:"scheduledDeploymentTimestamp,omitempty"`
	RFQEnabled                   *bool              `json:"rfqEnabled,omitempty"`
	EventStartTime               *string            `json:"eventStartTime,omitempty"`
	SubmittedBy                  *string            `json:"submittedBy,omitempty"`
	RequiresTranslation          *bool              `json:"requiresTranslation,omitempty"`
	PagerDutyNotificationEnabled *bool              `json:"pagerDutyNotificationEnabled,omitempty"`
	Approved                     *bool              `json:"approved,omitempty"`
	Cyom                         *bool              `json:"cyom,omitempty"`
	FeesEnabled                  *bool              `json:"feesEnabled,omitempty"`
	HoldingRewardsEnabled        *bool              `json:"holdingRewardsEnabled,omitempty"`
	NegRisk                      *bool              `json:"negRisk,omitempty"`
	NegRiskRequestID             *string            `json:"negRiskRequestID,omitempty" empty:"nil"`
	NegRiskMarketID              *string            `json:"negRiskMarketID,omitempty" empty:"nil"`
	SentDiscord                  *bool              `json:"sentDiscord,omitempty"`
	TwitterCardLastRefreshed     *int64             `json:"twitterCardLastRefreshed,omitempty"`
	TwitterCardLocation          *string            `json:"twitterCardLocation,omitempty"`
	TwitterCardLastValidated     *string            `json:"twitterCardLastValidated,omitempty"`
	CLOBRewards                  *[]ClobReward      `json:"clobRewards,omitempty"`
	CategoryMailchimpTag         *string            `json:"categoryMailchimpTag,omitempty"`
	Subcategory                  *string            `json:"subcategory,omitempty"`
}

type ClobReward struct {
	ID               *string `json:"id,omitempty"`
	AssetAddress     *string `json:"assetAddress,omitempty" empty:"nil"`
	ConditionID      *string `json:"conditionId,omitempty" empty:"nil"`
	StartDate        *string `json:"startDate,omitempty"`
	EndDate          *string `json:"endDate,omitempty"`
	RewardsAmount    *Number `json:"rewardsAmount,omitempty"`
	RewardsDailyRate *Number `json:"rewardsDailyRate,omitempty"`
}

type Series struct {
	ID                  string        `json:"id"`
	Ticker              *string       `json:"ticker,omitempty"`
	Slug                *string       `json:"slug,omitempty"`
	Title               *string       `json:"title,omitempty"`
	Subtitle            *string       `json:"subtitle,omitempty"`
	SeriesType          *string       `json:"seriesType,omitempty"`
	Recurrence          *string       `json:"recurrence,omitempty"`
	Description         *string       `json:"description,omitempty"`
	Image               *string       `json:"image,omitempty"`
	Icon                *string       `json:"icon,omitempty"`
	Layout              *string       `json:"layout,omitempty"`
	Active              *bool         `json:"active,omitempty"`
	Closed              *bool         `json:"closed,omitempty"`
	Archived            *bool         `json:"archived,omitempty"`
	New                 *bool         `json:"new,omitempty"`
	Featured            *bool         `json:"featured,omitempty"`
	Restricted          *bool         `json:"restricted,omitempty"`
	IsTemplate          *bool         `json:"isTemplate,omitempty"`
	TemplateVariables   *bool         `json:"templateVariables,omitempty"`
	PublishedAt         *string       `json:"publishedAt,omitempty"`
	CreatedBy           *string       `json:"createdBy,omitempty"`
	UpdatedBy           *string       `json:"updatedBy,omitempty"`
	CreatedAt           *string       `json:"createdAt,omitempty"`
	UpdatedAt           *string       `json:"updatedAt,omitempty"`
	CommentsEnabled     *bool         `json:"commentsEnabled,omitempty"`
	Competitive         *Number       `json:"competitive,omitempty"`
	Volume24hr          *Number       `json:"volume24hr,omitempty"`
	Volume              *Number       `json:"volume,omitempty"`
	Liquidity           *Number       `json:"liquidity,omitempty"`
	StartDate           *string       `json:"startDate,omitempty"`
	PythTokenID         *string       `json:"pythTokenID,omitempty"`
	CgAssetName         *string       `json:"cgAssetName,omitempty"`
	Score               *int32        `json:"score,omitempty"`
	Events              *[]Event      `json:"events,omitempty"`
	Collections         *[]Collection `json:"collections,omitempty"`
	Categories          *[]Category   `json:"categories,omitempty"`
	Tags                *[]Tag        `json:"tags,omitempty"`
	CommentCount        *int32        `json:"commentCount,omitempty"`
	Chats               *[]Chat       `json:"chats,omitempty"`
	RequiresTranslation *bool         `json:"requiresTranslation,omitempty"`
}

type CommentPosition struct {
	TokenID      *Number `json:"tokenId,omitempty"`
	PositionSize *Number `json:"positionSize,omitempty"`
}

type CommentProfile struct {
	Name                  *string            `json:"name,omitempty"`
	Pseudonym             *string            `json:"pseudonym,omitempty"`
	DisplayUsernamePublic *bool              `json:"displayUsernamePublic,omitempty"`
	Bio                   *string            `json:"bio,omitempty"`
	IsMod                 *bool              `json:"isMod,omitempty"`
	IsCreator             *bool              `json:"isCreator,omitempty"`
	ProxyWallet           *string            `json:"proxyWallet,omitempty" empty:"nil"`
	BaseAddress           *string            `json:"baseAddress,omitempty" empty:"nil"`
	ProfileImage          *string            `json:"profileImage,omitempty"`
	ProfileImageOptimized *ImageOptimization `json:"profileImageOptimized,omitempty"`
	Positions             *[]CommentPosition `json:"positions,omitempty"`
}

type Reaction struct {
	ID           string          `json:"id"`
	CommentID    *int32          `json:"commentID,omitempty"`
	ReactionType *string         `json:"reactionType,omitempty"`
	Icon         *string         `json:"icon,omitempty"`
	UserAddress  *string         `json:"userAddress,omitempty" empty:"nil"`
	CreatedAt    *string         `json:"createdAt,omitempty"`
	Profile      *CommentProfile `json:"profile,omitempty"`
}

type Comment struct {
	ID               string          `json:"id"`
	Body             *string         `json:"body,omitempty"`
	ParentEntityType *string         `json:"parentEntityType,omitempty"`
	ParentEntityID   *int32          `json:"parentEntityID,omitempty"`
	ParentCommentID  *string         `json:"parentCommentID,omitempty"`
	UserAddress      *string         `json:"userAddress,omitempty" empty:"nil"`
	ReplyAddress     *string         `json:"replyAddress,omitempty" empty:"nil"`
	CreatedAt        *string         `json:"createdAt,omitempty"`
	UpdatedAt        *string         `json:"updatedAt,omitempty"`
	Profile          *CommentProfile `json:"profile,omitempty"`
	Reactions        *[]Reaction     `json:"reactions,omitempty"`
	ReportCount      *int32          `json:"reportCount,omitempty"`
	ReactionCount    *int32          `json:"reactionCount,omitempty"`
}

type PublicProfileUser struct {
	ID      *string `json:"id,omitempty"`
	Creator *bool   `json:"creator,omitempty"`
	IsMod   *bool   `json:"mod,omitempty"`
}

type PublicProfile struct {
	CreatedAt             *string              `json:"createdAt,omitempty"`
	ProxyWallet           *string              `json:"proxyWallet,omitempty" empty:"nil"`
	ProfileImage          *string              `json:"profileImage,omitempty"`
	DisplayUsernamePublic *bool                `json:"displayUsernamePublic,omitempty"`
	Bio                   *string              `json:"bio,omitempty"`
	Pseudonym             *string              `json:"pseudonym,omitempty"`
	Name                  *string              `json:"name,omitempty"`
	Users                 *[]PublicProfileUser `json:"users,omitempty"`
	XUsername             *string              `json:"xUsername,omitempty"`
	VerifiedBadge         *bool                `json:"verifiedBadge,omitempty"`
}

type SearchTag struct {
	ID         *string `json:"id,omitempty"`
	Label      *string `json:"label,omitempty"`
	Slug       *string `json:"slug,omitempty"`
	EventCount *int32  `json:"eventCount,omitempty"`
}

type Profile struct {
	ID                    string             `json:"id"`
	Name                  *string            `json:"name,omitempty"`
	User                  *int32             `json:"user,omitempty"`
	Referral              *string            `json:"referral,omitempty"`
	CreatedBy             *int32             `json:"createdBy,omitempty"`
	UpdatedBy             *int32             `json:"updatedBy,omitempty"`
	CreatedAt             *string            `json:"createdAt,omitempty"`
	UpdatedAt             *string            `json:"updatedAt,omitempty"`
	UtmSource             *string            `json:"utmSource,omitempty"`
	UtmMedium             *string            `json:"utmMedium,omitempty"`
	UtmCampaign           *string            `json:"utmCampaign,omitempty"`
	UtmContent            *string            `json:"utmContent,omitempty"`
	UtmTerm               *string            `json:"utmTerm,omitempty"`
	WalletActivated       *bool              `json:"walletActivated,omitempty"`
	Pseudonym             *string            `json:"pseudonym,omitempty"`
	DisplayUsernamePublic *bool              `json:"displayUsernamePublic,omitempty"`
	ProfileImage          *string            `json:"profileImage,omitempty"`
	Bio                   *string            `json:"bio,omitempty"`
	ProxyWallet           *string            `json:"proxyWallet,omitempty" empty:"nil"`
	ProfileImageOptimized *ImageOptimization `json:"profileImageOptimized,omitempty"`
	IsCloseOnly           *bool              `json:"isCloseOnly,omitempty"`
	IsCertReq             *bool              `json:"isCertReq,omitempty"`
	CertReqDate           *string            `json:"certReqDate,omitempty"`
}

type SearchResults struct {
	Events     *[]Event     `json:"events,omitempty"`
	Tags       *[]SearchTag `json:"tags,omitempty"`
	Profiles   *[]Profile   `json:"profiles,omitempty"`
	Pagination *Pagination  `json:"pagination,omitempty"`
}
