package data

import (
	"reflect"
	"strings"
)

type Health struct {
	Data string `json:"data"`
}

type ApiError struct {
	Error string `json:"error"`
}

type Position struct {
	ProxyWallet        string  `json:"proxyWallet"`
	Asset              Number  `json:"asset"`
	ConditionID        string  `json:"conditionId"`
	Size               Number  `json:"size"`
	AvgPrice           Number  `json:"avgPrice"`
	InitialValue       Number  `json:"initialValue"`
	CurrentValue       Number  `json:"currentValue"`
	CashPnL            Number  `json:"cashPnl"`
	PercentPnL         Number  `json:"percentPnl"`
	TotalBought        Number  `json:"totalBought"`
	RealizedPnL        Number  `json:"realizedPnl"`
	PercentRealizedPnL Number  `json:"percentRealizedPnl"`
	CurPrice           Number  `json:"curPrice"`
	Redeemable         bool    `json:"redeemable"`
	Mergeable          bool    `json:"mergeable"`
	Title              string  `json:"title"`
	Slug               string  `json:"slug"`
	Icon               string  `json:"icon"`
	EventSlug          string  `json:"eventSlug"`
	EventID            *string `json:"eventId,omitempty" empty:"nil"`
	Outcome            string  `json:"outcome"`
	OutcomeIndex       int32   `json:"outcomeIndex"`
	OppositeOutcome    string  `json:"oppositeOutcome"`
	OppositeAsset      Number  `json:"oppositeAsset"`
	EndDate            *string `json:"endDate,omitempty" empty:"nil"`
	NegativeRisk       bool    `json:"negativeRisk"`
}

func (v *Position) UnmarshalJSON(b []byte) error {
	type plain Position
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}

type ClosedPosition struct {
	ProxyWallet     string `json:"proxyWallet"`
	Asset           Number `json:"asset"`
	ConditionID     string `json:"conditionId"`
	AvgPrice        Number `json:"avgPrice"`
	TotalBought     Number `json:"totalBought"`
	RealizedPnL     Number `json:"realizedPnl"`
	CurPrice        Number `json:"curPrice"`
	Timestamp       int64  `json:"timestamp"`
	Title           string `json:"title"`
	Slug            string `json:"slug"`
	Icon            string `json:"icon"`
	EventSlug       string `json:"eventSlug"`
	Outcome         string `json:"outcome"`
	OutcomeIndex    int32  `json:"outcomeIndex"`
	OppositeOutcome string `json:"oppositeOutcome"`
	OppositeAsset   Number `json:"oppositeAsset"`
	EndDate         string `json:"endDate"`
}

type Trade struct {
	ProxyWallet           string  `json:"proxyWallet"`
	Side                  string  `json:"side"`
	Asset                 Number  `json:"asset"`
	ConditionID           string  `json:"conditionId"`
	Size                  Number  `json:"size"`
	Price                 Number  `json:"price"`
	Timestamp             int64   `json:"timestamp"`
	Title                 string  `json:"title"`
	Slug                  string  `json:"slug"`
	Icon                  string  `json:"icon"`
	EventSlug             string  `json:"eventSlug"`
	Outcome               string  `json:"outcome"`
	OutcomeIndex          int32   `json:"outcomeIndex"`
	Name                  *string `json:"name,omitempty" empty:"nil"`
	Pseudonym             *string `json:"pseudonym,omitempty" empty:"nil"`
	Bio                   *string `json:"bio,omitempty" empty:"nil"`
	ProfileImage          *string `json:"profileImage,omitempty" empty:"nil"`
	ProfileImageOptimized *string `json:"profileImageOptimized,omitempty" empty:"nil"`
	TransactionHash       string  `json:"transactionHash"`
}

func (v *Trade) UnmarshalJSON(b []byte) error {
	type plain Trade
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}

type Activity struct {
	ProxyWallet           string  `json:"proxyWallet"`
	Timestamp             int64   `json:"timestamp"`
	ConditionID           *string `json:"conditionId,omitempty" empty:"nil"`
	ActivityType          string  `json:"type"`
	Size                  Number  `json:"size"`
	USDCSize              Number  `json:"usdcSize"`
	TransactionHash       string  `json:"transactionHash"`
	Price                 *Number `json:"price,omitempty"`
	Asset                 *Number `json:"asset,omitempty" empty:"nil"`
	Side                  *string `json:"side,omitempty" empty:"nil"`
	OutcomeIndex          *int32  `json:"outcomeIndex,omitempty"`
	Title                 *string `json:"title,omitempty" empty:"nil"`
	Slug                  *string `json:"slug,omitempty" empty:"nil"`
	Icon                  *string `json:"icon,omitempty" empty:"nil"`
	EventSlug             *string `json:"eventSlug,omitempty" empty:"nil"`
	Outcome               *string `json:"outcome,omitempty" empty:"nil"`
	Name                  *string `json:"name,omitempty" empty:"nil"`
	Pseudonym             *string `json:"pseudonym,omitempty" empty:"nil"`
	Bio                   *string `json:"bio,omitempty" empty:"nil"`
	ProfileImage          *string `json:"profileImage,omitempty" empty:"nil"`
	ProfileImageOptimized *string `json:"profileImageOptimized,omitempty" empty:"nil"`
}

func (v *Activity) UnmarshalJSON(b []byte) error {
	type plain Activity
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	if v.Side != nil {
		side := strings.ToUpper(*v.Side)
		if side == SideBuy || side == SideSell {
			v.Side = &side
		} else {
			v.Side = nil
		}
	}
	return nil
}

type Holder struct {
	ProxyWallet           string  `json:"proxyWallet"`
	Bio                   *string `json:"bio,omitempty" empty:"nil"`
	Asset                 Number  `json:"asset"`
	Pseudonym             *string `json:"pseudonym,omitempty" empty:"nil"`
	Amount                Number  `json:"amount"`
	DisplayUsernamePublic *bool   `json:"displayUsernamePublic,omitempty"`
	OutcomeIndex          int32   `json:"outcomeIndex"`
	Name                  *string `json:"name,omitempty" empty:"nil"`
	ProfileImage          *string `json:"profileImage,omitempty" empty:"nil"`
	ProfileImageOptimized *string `json:"profileImageOptimized,omitempty" empty:"nil"`
	Verified              *bool   `json:"verified,omitempty"`
}

func (v *Holder) UnmarshalJSON(b []byte) error {
	type plain Holder
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}

type MetaHolder struct {
	Token   Number   `json:"token"`
	Holders []Holder `json:"holders,omitempty"`
}

type Traded struct {
	User   string `json:"user"`
	Traded int32  `json:"traded"`
}

type Value struct {
	User  string `json:"user"`
	Value Number `json:"value"`
}

type OpenInterest struct {
	Market string `json:"market"`
	Value  Number `json:"value"`
}

type MarketVolume struct {
	Market string `json:"market"`
	Value  Number `json:"value"`
}

type LiveVolume struct {
	Total   Number         `json:"total"`
	Markets []MarketVolume `json:"markets,omitempty"`
}

// UnmarshalJSON treats missing or null market lists as empty, as the API does.
func (v *LiveVolume) UnmarshalJSON(b []byte) error {
	type plain LiveVolume
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	if v.Markets == nil {
		v.Markets = []MarketVolume{}
	}
	return nil
}

type BuilderLeaderboardEntry struct {
	Rank        QuotedInt `json:"rank"`
	Builder     string    `json:"builder"`
	Volume      Number    `json:"volume"`
	ActiveUsers int32     `json:"activeUsers"`
	Verified    bool      `json:"verified"`
	BuilderLogo *string   `json:"builderLogo,omitempty" empty:"nil"`
}

func (v *BuilderLeaderboardEntry) UnmarshalJSON(b []byte) error {
	type plain BuilderLeaderboardEntry
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}

type BuilderVolumeEntry struct {
	Dt          string    `json:"dt"`
	Builder     string    `json:"builder"`
	BuilderLogo *string   `json:"builderLogo,omitempty" empty:"nil"`
	Verified    bool      `json:"verified"`
	Volume      Number    `json:"volume"`
	ActiveUsers int32     `json:"activeUsers"`
	Rank        QuotedInt `json:"rank"`
}

func (v *BuilderVolumeEntry) UnmarshalJSON(b []byte) error {
	type plain BuilderVolumeEntry
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}

type TraderLeaderboardEntry struct {
	Rank          QuotedInt `json:"rank"`
	ProxyWallet   string    `json:"proxyWallet"`
	UserName      *string   `json:"userName,omitempty" empty:"nil"`
	Vol           Number    `json:"vol"`
	PnL           Number    `json:"pnl"`
	ProfileImage  *string   `json:"profileImage,omitempty" empty:"nil"`
	XUsername     *string   `json:"xUsername,omitempty" empty:"nil"`
	VerifiedBadge *bool     `json:"verifiedBadge,omitempty"`
}

func (v *TraderLeaderboardEntry) UnmarshalJSON(b []byte) error {
	type plain TraderLeaderboardEntry
	if err := decodeResponse(b, (*plain)(v)); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(v))
	return nil
}
