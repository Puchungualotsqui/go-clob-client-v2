package clob

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type UserInfo struct {
	Address                 Address `json:"address"`
	Username                string  `json:"username"`
	ProfilePicture          string  `json:"profile_picture"`
	OptimizedProfilePicture string  `json:"optimized_profile_picture"`
	Pseudonym               string  `json:"pseudonym"`
}

// Dates use ISO 8601 YYYY-MM-DD strings.
type UserEarningResponse struct {
	Date         string  `json:"date"`
	ConditionID  B256    `json:"condition_id"`
	AssetAddress Address `json:"asset_address"`
	MakerAddress Address `json:"maker_address"`
	Earnings     Decimal `json:"earnings"`
	AssetRate    Decimal `json:"asset_rate"`
}
type TotalUserEarningResponse struct {
	Date         string  `json:"date"`
	AssetAddress Address `json:"asset_address"`
	MakerAddress Address `json:"maker_address"`
	Earnings     Decimal `json:"earnings"`
	AssetRate    Decimal `json:"asset_rate"`
}
type RewardsConfig struct {
	AssetAddress Address `json:"asset_address"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	RatePerDay   Decimal `json:"rate_per_day"`
	TotalRewards Decimal `json:"total_rewards"`
}
type MarketRewardsConfig struct {
	ID           string  `json:"id"`
	AssetAddress Address `json:"asset_address"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
	RatePerDay   Decimal `json:"rate_per_day"`
	TotalRewards Decimal `json:"total_rewards"`
	TotalDays    Decimal `json:"total_days"`
}

func (r *MarketRewardsConfig) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "id", "asset_address", "start_date", "end_date", "rate_per_day", "total_rewards", "total_days")
	if err != nil {
		return err
	}
	if err := responseAddress(fields, "asset_address"); err != nil {
		return err
	}
	if err := responseDates(fields, "start_date", "end_date"); err != nil {
		return err
	}
	type plain MarketRewardsConfig
	var v struct {
		*plain
		ID json.RawMessage `json:"id"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v.ID) > 0 {
		if v.ID[0] == '"' {
			if err := json.Unmarshal(v.ID, &p.ID); err != nil {
				return err
			}
		} else {
			s := string(v.ID)
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				p.ID = strconv.FormatInt(n, 10)
			} else if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				p.ID = strconv.FormatUint(n, 10)
			} else {
				return fmt.Errorf("invalid rewards config id %s", s)
			}
		}
	}
	*r = MarketRewardsConfig(p)
	return nil
}

type Earning struct {
	AssetAddress Address `json:"asset_address"`
	Earnings     Decimal `json:"earnings"`
	AssetRate    Decimal `json:"asset_rate"`
}
type UserRewardsEarningResponse struct {
	ConditionID           B256            `json:"condition_id"`
	Question              string          `json:"question"`
	MarketSlug            string          `json:"market_slug"`
	EventSlug             string          `json:"event_slug"`
	Image                 string          `json:"image"`
	RewardsMaxSpread      Decimal         `json:"rewards_max_spread"`
	RewardsMinSize        Decimal         `json:"rewards_min_size"`
	MarketCompetitiveness Decimal         `json:"market_competitiveness"`
	Tokens                []Token         `json:"tokens"`
	RewardsConfig         []RewardsConfig `json:"rewards_config"`
	MakerAddress          Address         `json:"maker_address"`
	EarningPercentage     Decimal         `json:"earning_percentage"`
	Earnings              []Earning       `json:"earnings"`
}

func (r *UserRewardsEarningResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"tokens", "rewards_config", "earnings"}, "condition_id", "question", "market_slug", "event_slug", "image", "rewards_max_spread", "rewards_min_size", "market_competitiveness", "maker_address", "earning_percentage")
	if err != nil {
		return err
	}
	if err := responseOptionalIdentifiers(fields, []string{"condition_id"}, []string{"maker_address"}); err != nil {
		return err
	}
	type plain UserRewardsEarningResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Tokens == nil {
		v.Tokens = []Token{}
	}
	if v.RewardsConfig == nil {
		v.RewardsConfig = []RewardsConfig{}
	}
	if v.Earnings == nil {
		v.Earnings = []Earning{}
	}
	*r = UserRewardsEarningResponse(v)
	return nil
}

type RewardsPercentagesResponse map[string]Decimal
type CurrentRewardResponse struct {
	ConditionID      B256            `json:"condition_id"`
	RewardsConfig    []RewardsConfig `json:"rewards_config"`
	RewardsMaxSpread Decimal         `json:"rewards_max_spread"`
	RewardsMinSize   Decimal         `json:"rewards_min_size"`
}

func (r *CurrentRewardResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"rewards_config"}, "condition_id", "rewards_max_spread", "rewards_min_size")
	if err != nil {
		return err
	}
	if err := responseHash(fields, "condition_id"); err != nil {
		return err
	}
	type plain CurrentRewardResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.RewardsConfig == nil {
		v.RewardsConfig = []RewardsConfig{}
	}
	*r = CurrentRewardResponse(v)
	return nil
}

type MarketRewardResponse struct {
	ConditionID           B256                  `json:"condition_id"`
	Question              string                `json:"question"`
	MarketSlug            string                `json:"market_slug"`
	EventSlug             string                `json:"event_slug"`
	Image                 string                `json:"image"`
	RewardsMaxSpread      Decimal               `json:"rewards_max_spread"`
	RewardsMinSize        Decimal               `json:"rewards_min_size"`
	MarketCompetitiveness Decimal               `json:"market_competitiveness"`
	Tokens                []Token               `json:"tokens"`
	RewardsConfig         []MarketRewardsConfig `json:"rewards_config"`
}

func (r *MarketRewardResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"tokens", "rewards_config"}, "condition_id", "question", "market_slug", "event_slug", "image", "rewards_max_spread", "rewards_min_size", "market_competitiveness")
	if err != nil {
		return err
	}
	if err := responseHash(fields, "condition_id"); err != nil {
		return err
	}
	type plain MarketRewardResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Tokens == nil {
		v.Tokens = []Token{}
	}
	if v.RewardsConfig == nil {
		v.RewardsConfig = []MarketRewardsConfig{}
	}
	*r = MarketRewardResponse(v)
	return nil
}

type BuilderAPIKeyResponse struct {
	Key       APIKey     `json:"key"`
	CreatedAt *time.Time `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt"`
}
type BuilderTradeResponse struct {
	ID              string          `json:"id"`
	TradeType       string          `json:"tradeType"`
	TakerOrderHash  B256            `json:"takerOrderHash"`
	Builder         Address         `json:"builder"`
	Market          B256            `json:"market"`
	AssetID         U256            `json:"assetId"`
	Side            Side            `json:"side"`
	Size            Decimal         `json:"size"`
	SizeUSDC        Decimal         `json:"sizeUsdc"`
	Price           Decimal         `json:"price"`
	Status          TradeStatusType `json:"status"`
	Outcome         string          `json:"outcome"`
	OutcomeIndex    uint32          `json:"outcomeIndex"`
	Owner           APIKey          `json:"owner"`
	Maker           Address         `json:"maker"`
	TransactionHash B256            `json:"transactionHash"`
	MatchTime       UnixSeconds     `json:"matchTime"`
	BucketIndex     uint32          `json:"bucketIndex"`
	Fee             Decimal         `json:"fee"`
	FeeUSDC         Decimal         `json:"feeUsdc"`
	ErrMsg          *string         `json:"errMsg"`
	CreatedAt       *time.Time      `json:"createdAt"`
	UpdatedAt       *time.Time      `json:"updatedAt"`
}

func (r *BuilderTradeResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, []string{"errMsg", "err_msg", "createdAt", "updatedAt"}, "id", "tradeType", "takerOrderHash", "builder", "market", "assetId", "side", "size", "sizeUsdc", "price", "status", "outcome", "outcomeIndex", "owner", "maker", "transactionHash", "matchTime", "bucketIndex", "fee", "feeUsdc")
	if err != nil {
		return err
	}
	if _, ok := fields["errMsg"]; ok {
		if _, ok := fields["err_msg"]; ok {
			return fmt.Errorf("errMsg: duplicate field via alias err_msg")
		}
	}
	if err := responseOptionalIdentifiers(fields, []string{"takerOrderHash", "market", "transactionHash"}, []string{"builder", "maker"}); err != nil {
		return err
	}
	if err := responseUUID(fields, "owner"); err != nil {
		return err
	}
	type plain BuilderTradeResponse
	var v struct {
		*plain
		ErrMsgAlias *string `json:"err_msg"`
	}
	var p plain
	v.plain = &p
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if p.ErrMsg == nil {
		p.ErrMsg = v.ErrMsgAlias
	}
	*r = BuilderTradeResponse(p)
	return nil
}

func (r *RewardsConfig) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "asset_address", "start_date", "end_date", "rate_per_day", "total_rewards")
	if err != nil {
		return err
	}
	if err := responseAddress(fields, "asset_address"); err != nil {
		return err
	}
	if err := responseDates(fields, "start_date", "end_date"); err != nil {
		return err
	}
	type plain RewardsConfig
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = RewardsConfig(v)
	return nil
}

func (r *Earning) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "asset_address", "earnings", "asset_rate")
	if err != nil {
		return err
	}
	if err := responseAddress(fields, "asset_address"); err != nil {
		return err
	}
	type plain Earning
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Earning(v)
	return nil
}

func (r *UserEarningResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "date", "condition_id", "asset_address", "maker_address", "earnings", "asset_rate")
	if err != nil {
		return err
	}
	if err := responseOptionalIdentifiers(fields, []string{"condition_id"}, []string{"asset_address", "maker_address"}); err != nil {
		return err
	}
	if err := responseDates(fields, "date"); err != nil {
		return err
	}
	type plain UserEarningResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = UserEarningResponse(v)
	return nil
}

func (r *TotalUserEarningResponse) UnmarshalJSON(b []byte) error {
	fields, err := responseFields(b, nil, "date", "asset_address", "maker_address", "earnings", "asset_rate")
	if err != nil {
		return err
	}
	if err := responseOptionalIdentifiers(fields, nil, []string{"asset_address", "maker_address"}); err != nil {
		return err
	}
	if err := responseDates(fields, "date"); err != nil {
		return err
	}
	type plain TotalUserEarningResponse
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = TotalUserEarningResponse(v)
	return nil
}

type NotificationResponse struct {
	Type    uint32              `json:"type"`
	Owner   APIKey              `json:"owner"`
	Payload NotificationPayload `json:"payload"`
}
type NotificationPayload struct {
	AssetID         U256      `json:"asset_id"`
	ConditionID     B256      `json:"condition_id"`
	EventSlug       string    `json:"eventSlug"`
	Icon            string    `json:"icon"`
	Image           string    `json:"image"`
	Market          B256      `json:"market"`
	MarketSlug      string    `json:"market_slug"`
	MatchedSize     Decimal   `json:"matched_size"`
	OrderID         string    `json:"order_id"`
	OriginalSize    Decimal   `json:"original_size"`
	Outcome         string    `json:"outcome"`
	OutcomeIndex    uint64    `json:"outcome_index"`
	Owner           APIKey    `json:"owner"`
	Price           Decimal   `json:"price"`
	Question        string    `json:"question"`
	RemainingSize   Decimal   `json:"remaining_size"`
	SeriesSlug      string    `json:"seriesSlug"`
	Side            Side      `json:"side"`
	TradeID         string    `json:"trade_id"`
	TransactionHash B256      `json:"transaction_hash"`
	OrderType       OrderType `json:"order_type"`
}
