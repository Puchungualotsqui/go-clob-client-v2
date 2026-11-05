package data

import (
	"fmt"
	"math/big"
	"reflect"
)

// Common filter values. String fields also accept values added by the API later.
const (
	SideBuy                       = "BUY"
	SideSell                      = "SELL"
	ActivityTrade                 = "TRADE"
	ActivitySplit                 = "SPLIT"
	ActivityMerge                 = "MERGE"
	ActivityRedeem                = "REDEEM"
	ActivityReward                = "REWARD"
	ActivityConversion            = "CONVERSION"
	ActivityYield                 = "YIELD"
	ActivityMakerRebate           = "MAKERREBATE"
	SortAscending                 = "ASC"
	SortDescending                = "DESC"
	PositionSortCurrent           = "CURRENT"
	PositionSortInitial           = "INITIAL"
	PositionSortTokens            = "TOKENS"
	PositionSortCashPnL           = "CASHPNL"
	PositionSortPercentPnL        = "PERCENTPNL"
	PositionSortTitle             = "TITLE"
	PositionSortResolving         = "RESOLVING"
	PositionSortPrice             = "PRICE"
	PositionSortAvgPrice          = "AVGPRICE"
	ClosedPositionSortRealizedPnL = "REALIZEDPNL"
	ClosedPositionSortTitle       = "TITLE"
	ClosedPositionSortPrice       = "PRICE"
	ClosedPositionSortAvgPrice    = "AVGPRICE"
	ClosedPositionSortTimestamp   = "TIMESTAMP"
	ActivitySortTimestamp         = "TIMESTAMP"
	ActivitySortTokens            = "TOKENS"
	ActivitySortCash              = "CASH"
	FilterCash                    = "CASH"
	FilterTokens                  = "TOKENS"
	PeriodDay                     = "DAY"
	PeriodWeek                    = "WEEK"
	PeriodMonth                   = "MONTH"
	PeriodAll                     = "ALL"
	CategoryOverall               = "OVERALL"
	CategoryPolitics              = "POLITICS"
	CategorySports                = "SPORTS"
	CategoryCrypto                = "CRYPTO"
	CategoryCulture               = "CULTURE"
	CategoryMentions              = "MENTIONS"
	CategoryWeather               = "WEATHER"
	CategoryEconomics             = "ECONOMICS"
	CategoryTech                  = "TECH"
	CategoryFinance               = "FINANCE"
	LeaderboardOrderPnL           = "PNL"
	LeaderboardOrderVolume        = "VOL"
)

// MarketFilter selects either markets or event IDs, but not both.
type MarketFilter struct {
	Markets  []string `query:"market,omitempty,comma"`
	EventIDs []string `query:"eventId,omitempty,comma"`
}

// TradeFilter specifies a minimum CASH or TOKENS amount.
type TradeFilter struct {
	FilterType   string `query:"filterType"`
	FilterAmount Number `query:"filterAmount"`
}

func validateQuery(req any) error {
	if req != nil {
		v := reflect.ValueOf(req)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil
			}
			return validateQuery(v.Elem().Interface())
		}
	}
	var user *string
	var markets []string
	var threshold *Number
	var filter *MarketFilter
	var trade *TradeFilter
	var limit, offset, minBalance *int32
	var limitMin, limitMax, offsetMax int32
	switch r := req.(type) {
	case PositionsRequest:
		filter, limit, offset, limitMax, offsetMax = r.Filter, r.Limit, r.Offset, 500, 10000
		user, threshold = &r.User, r.SizeThreshold
	case TradesRequest:
		filter, trade, limit, offset, limitMax, offsetMax = r.Filter, r.TradeFilter, r.Limit, r.Offset, 10000, 10000
		user = r.User
	case ActivityRequest:
		filter, limit, offset, limitMax, offsetMax = r.Filter, r.Limit, r.Offset, 500, 10000
		user = &r.User
	case HoldersRequest:
		limit, minBalance, limitMax = r.Limit, r.MinBalance, 20
		markets = r.Markets
	case TradedRequest:
		user = &r.User
	case ValueRequest:
		user, markets = &r.User, r.Markets
	case OpenInterestRequest:
		markets = r.Markets
	case ClosedPositionsRequest:
		filter, limit, offset, limitMax, offsetMax = r.Filter, r.Limit, r.Offset, 50, 100000
		user = &r.User
	case BuilderLeaderboardRequest:
		limit, offset, limitMax, offsetMax = r.Limit, r.Offset, 50, 1000
	case TraderLeaderboardRequest:
		limit, offset, limitMin, limitMax, offsetMax = r.Limit, r.Offset, 1, 50, 1000
		user = r.User
	}
	if filter != nil && len(filter.Markets) > 0 && len(filter.EventIDs) > 0 {
		return fmt.Errorf("market and eventId filters are mutually exclusive")
	}
	if user != nil {
		if err := validateHex(*user, 20); err != nil {
			return fmt.Errorf("user: %w", err)
		}
	}
	if filter != nil {
		markets = filter.Markets
	}
	for _, market := range markets {
		if err := validateHex(market, 32); err != nil {
			return fmt.Errorf("market: %w", err)
		}
	}
	if threshold != nil {
		if err := validateDecimal(string(*threshold)); err != nil {
			return fmt.Errorf("sizeThreshold: %w", err)
		}
	}
	if trade != nil {
		if validateDecimal(string(trade.FilterAmount)) != nil {
			return fmt.Errorf("filter amount must be a nonnegative decimal")
		}
		v, ok := new(big.Rat).SetString(string(trade.FilterAmount))
		if !ok || v.Sign() < 0 {
			return fmt.Errorf("filter amount must be a nonnegative decimal")
		}
		if trade.FilterType != "CASH" && trade.FilterType != "TOKENS" {
			return fmt.Errorf("filter type must be CASH or TOKENS")
		}
	}
	for _, b := range []struct {
		name     string
		value    *int32
		min, max int32
	}{{"limit", limit, limitMin, limitMax}, {"offset", offset, 0, offsetMax}, {"minBalance", minBalance, 0, 999999}} {
		if b.value != nil && (*b.value < b.min || *b.value > b.max) {
			return fmt.Errorf("%s must be between %d and %d", b.name, b.min, b.max)
		}
	}
	return nil
}
