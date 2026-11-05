package clob

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type MidpointRequest struct {
	TokenID U256 `json:"token_id"`
}
type PriceRequest struct {
	TokenID U256 `json:"token_id"`
	Side    Side `json:"side"`
}
type SpreadRequest struct {
	TokenID U256  `json:"token_id"`
	Side    *Side `json:"side,omitempty"`
}
type OrderBookSummaryRequest = SpreadRequest
type LastTradePriceRequest = MidpointRequest

// TimeRange represents either an interval or a pair of Unix timestamps.
type TimeRange struct {
	Interval       *Interval
	StartTS, EndTS *int64
}

func IntervalRange(i Interval) TimeRange        { return TimeRange{Interval: &i} }
func TimestampRange(start, end int64) TimeRange { return TimeRange{StartTS: &start, EndTS: &end} }

type PriceHistoryRequest struct {
	Market    U256
	TimeRange TimeRange
	Fidelity  *uint32
}

func (r PriceHistoryRequest) MarshalJSON() ([]byte, error) {
	m := map[string]any{"market": r.Market}
	t := r.TimeRange
	if t.Interval != nil && t.StartTS == nil && t.EndTS == nil {
		m["interval"] = *t.Interval
	} else if t.Interval == nil && t.StartTS != nil && t.EndTS != nil {
		m["startTs"] = *t.StartTS
		m["endTs"] = *t.EndTS
	} else {
		return nil, fmt.Errorf("time range must contain an interval or start and end timestamps")
	}
	if r.Fidelity != nil {
		m["fidelity"] = *r.Fidelity
	}
	return json.Marshal(m)
}

type CancelMarketOrderRequest struct {
	Market  *B256 `json:"market"`
	AssetID *U256 `json:"asset_id"`
}
type TradesRequest struct {
	ID           *string  `json:"id,omitempty"`
	TakerAddress *Address `json:"taker,omitempty"`
	MakerAddress *Address `json:"maker,omitempty"`
	Market       *B256    `json:"market,omitempty"`
	AssetID      *U256    `json:"asset_id,omitempty"`
	Before       *int64   `json:"before,omitempty"`
	After        *int64   `json:"after,omitempty"`
}
type OrdersRequest struct {
	OrderID *string `json:"id,omitempty"`
	Market  *B256   `json:"market,omitempty"`
	AssetID *U256   `json:"asset_id,omitempty"`
}
type DeleteNotificationsRequest struct {
	NotificationIDs []string `json:"-"`
}

func (r DeleteNotificationsRequest) MarshalJSON() ([]byte, error) {
	if len(r.NotificationIDs) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(struct {
		IDs string `json:"ids"`
	}{strings.Join(r.NotificationIDs, ",")})
}

type BalanceAllowanceRequest struct {
	AssetType     AssetType      `json:"asset_type"`
	TokenID       *U256          `json:"token_id,omitempty"`
	SignatureType *SignatureType `json:"signature_type,omitempty"`
}
type UpdateBalanceAllowanceRequest = BalanceAllowanceRequest
type UserRewardsEarningRequest struct {
	Date          string `json:"date"`
	OrderBy       string `json:"order_by"`
	Position      string `json:"position"`
	NoCompetition bool   `json:"no_competition"`
}

// QueryParams serializes a flat request as URL-encoded parameters. Arrays use repeated keys.
func QueryParams(request any, cursor *string) (string, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	q := url.Values{}
	var add func(string, json.RawMessage) error
	add = func(k string, v json.RawMessage) error {
		if string(v) == "null" {
			return nil
		}
		if len(v) > 0 && v[0] == '[' {
			var a []json.RawMessage
			if err := json.Unmarshal(v, &a); err != nil {
				return err
			}
			for _, x := range a {
				if err := add(k, x); err != nil {
					return err
				}
			}
			return nil
		}
		var s string
		if len(v) > 0 && v[0] == '"' {
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
		} else if len(v) > 0 && v[0] == '{' {
			return fmt.Errorf("nested query parameter %s", k)
		} else {
			s = string(v)
		}
		q.Add(k, s)
		return nil
	}
	for k, v := range m {
		if err := add(k, v); err != nil {
			return "", err
		}
	}
	if cursor != nil {
		q.Add("next_cursor", *cursor)
	}
	s := q.Encode()
	if s != "" {
		s = "?" + s
	}
	return s, nil
}
