package gamma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Common filter values. The string fields also allow unknown future values.
const (
	RelatedTagsActive = "active"
	RelatedTagsClosed = "closed"
	RelatedTagsAll    = "all"
	ParentEvent       = "Event"
	ParentSeries      = "Series"
	ParentMarket      = "market"
)

// StringList and NumberList decode the JSON-encoded arrays returned in market fields.
type StringList []string
type NumberList []Number

func decodeList(b []byte, dst any) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		b = []byte(s)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(b, &elements); err != nil {
		return err
	}
	if elements == nil {
		return fmt.Errorf("expected array, got null")
	}
	for i, element := range elements {
		if bytes.Equal(bytes.TrimSpace(element), []byte("null")) {
			return fmt.Errorf("array element %d is null", i)
		}
	}
	return json.Unmarshal(b, dst)
}
func (l *StringList) UnmarshalJSON(b []byte) error {
	type plain StringList
	return decodeList(b, (*plain)(l))
}
func (l *NumberList) UnmarshalJSON(b []byte) error {
	type plain NumberList
	return decodeList(b, (*plain)(l))
}
func (l StringList) MarshalJSON() ([]byte, error) {
	if l == nil {
		l = StringList{}
	}
	b, err := json.Marshal([]string(l))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(b))
}
func (l NumberList) MarshalJSON() ([]byte, error) {
	if l == nil {
		l = NumberList{}
	}
	b, err := json.Marshal([]Number(l))
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(b))
}

// UnmarshalJSON accepts the legacy published_at spelling as well as publishedAt.
func (e *Event) UnmarshalJSON(b []byte) error {
	if err := validateResponse(b, "Event"); err != nil {
		return err
	}
	type plain Event
	var decoded plain
	if err := validateResponseLists(b, &decoded, "Event"); err != nil {
		return err
	}
	var v struct {
		*plain
		PublishedAtAlias *string `json:"published_at"`
	}
	v.plain = &decoded
	clean, err := exactResponseFields(b, &decoded, map[string]string{"published_at": "publishedAt"})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(clean, &v); err != nil {
		return err
	}
	if decoded.PublishedAt == nil {
		decoded.PublishedAt = v.PublishedAtAlias
	}
	if err := validateResponseDecimals(&decoded, "Event"); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(&decoded))
	*e = Event(decoded)
	return nil
}

// UnmarshalJSON accepts the legacy submitted_by spelling and a quoted refresh timestamp.
func (m *Market) UnmarshalJSON(b []byte) error {
	if err := validateResponse(b, "Market"); err != nil {
		return err
	}
	type plain Market
	var decoded plain
	if err := validateResponseLists(b, &decoded, "Market"); err != nil {
		return err
	}
	var v struct {
		*plain
		SubmittedByAlias *string         `json:"submitted_by"`
		Refresh          json.RawMessage `json:"twitterCardLastRefreshed"`
	}
	v.plain = &decoded
	clean, err := exactResponseFields(b, &decoded, map[string]string{"submitted_by": "submittedBy"})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(clean, &v); err != nil {
		return err
	}
	if decoded.SubmittedBy == nil {
		decoded.SubmittedBy = v.SubmittedByAlias
	}
	if v.Refresh != nil {
		var n *json.Number
		if err := json.Unmarshal(v.Refresh, &n); err != nil {
			return err
		}
		decoded.TwitterCardLastRefreshed = nil
		if n != nil {
			i, err := strconv.ParseInt(n.String(), 10, 64)
			if err != nil {
				return fmt.Errorf("twitterCardLastRefreshed: %w", err)
			}
			decoded.TwitterCardLastRefreshed = &i
		}
	}
	if err := validateResponseDecimals(&decoded, "Market"); err != nil {
		return err
	}
	if decoded.CLOBTokenIDs != nil {
		for i, id := range *decoded.CLOBTokenIDs {
			if err := validateUint256(string(id)); err != nil {
				return fmt.Errorf("Market.clobTokenIds[%d]: %w", i, err)
			}
		}
	}
	normalizeEmpty(reflect.ValueOf(&decoded))
	*m = Market(decoded)
	return nil
}

// MarshalJSON encodes the refresh timestamp as a string, matching the API format.
func (m Market) MarshalJSON() ([]byte, error) {
	type plain Market
	return json.Marshal(struct {
		plain
		Refresh *int64 `json:"twitterCardLastRefreshed,omitempty,string"`
	}{plain: plain(m), Refresh: m.TwitterCardLastRefreshed})
}

// CommaStrings decodes the comma-separated tags field in sports metadata.
type CommaStrings []string

func (l *CommaStrings) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*l = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*l = []string{}
	} else {
		*l = strings.Split(s, ",")
	}
	return nil
}

// MarshalJSON encodes sports tags in the comma-separated API format.
func (l CommaStrings) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.Join(l, ","))
}

// StreamData invokes fetch for consecutive pages, passing each item to yield.
// A non-nil yield error stops iteration. The page size is capped at 500.
func StreamData[T any](ctx context.Context, limit int32, fetch func(context.Context, int32, int32) ([]T, error), yield func(T) error) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	if limit > 500 {
		limit = 500
	}
	var offset int32
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := fetch(ctx, limit, offset)
		if err != nil {
			return err
		}
		for _, item := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := yield(item); err != nil {
				return err
			}
		}
		if len(page) < int(limit) {
			return nil
		}
		if int64(offset)+int64(len(page)) > 2147483647 {
			return fmt.Errorf("pagination offset overflow")
		}
		offset += int32(len(page))
	}
}
