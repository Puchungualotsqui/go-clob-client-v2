// Package gamma provides a client for the Polymarket Gamma API.
package gamma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const DefaultHost = "https://gamma-api.polymarket.com"

// Client is safe for concurrent use provided its HTTPClient is not modified concurrently.
type Client struct {
	Host       string
	HTTPClient *http.Client
}

// NewClient creates a client. An empty host selects the production endpoint.
func NewClient(host string, clients ...*http.Client) (*Client, error) {
	if host == "" {
		host = DefaultHost
	}
	u, err := url.Parse(host)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid API host %q", host)
	}
	if len(clients) > 1 {
		return nil, fmt.Errorf("at most one HTTP client may be supplied")
	}
	h := http.DefaultClient
	if len(clients) == 1 && clients[0] != nil {
		h = clients[0]
	}
	return &Client{Host: strings.TrimRight(host, "/"), HTTPClient: h}, nil
}

// HTTPError includes the response body for unsuccessful API requests.
type HTTPError struct {
	StatusCode        int
	Method, URL, Body string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

func queryValues(req any) (url.Values, error) {
	rv := reflect.ValueOf(req)
	if rv.IsValid() && !(rv.Kind() == reflect.Pointer && rv.IsNil()) {
		if validator, ok := req.(interface{ validateRequest() error }); ok {
			if err := validator.validateRequest(); err != nil {
				return nil, err
			}
		}
	}
	v := url.Values{}
	var add func(reflect.Value) error
	add = func(r reflect.Value) error {
		if !r.IsValid() {
			return nil
		}
		if r.Kind() == reflect.Pointer {
			if r.IsNil() {
				return nil
			}
			r = r.Elem()
		}
		if r.Kind() != reflect.Struct {
			return fmt.Errorf("query must be a struct")
		}
		for i := 0; i < r.NumField(); i++ {
			f := r.Type().Field(i)
			tag := strings.Split(f.Tag.Get("query"), ",")
			if len(tag) == 0 || tag[0] == "-" {
				continue
			}
			x := r.Field(i)
			opts := strings.Join(tag[1:], ",")
			if strings.Contains(opts, "inline") {
				if err := add(x); err != nil {
					return err
				}
				continue
			}
			if x.Kind() == reflect.Pointer {
				if x.IsNil() {
					continue
				}
				x = x.Elem()
			}
			if x.Kind() == reflect.Slice {
				if x.Len() == 0 {
					continue
				}
				var values []string
				for j := 0; j < x.Len(); j++ {
					value := fmt.Sprint(x.Index(j).Interface())
					if r.Type() == reflect.TypeOf(MarketsRequest{}) && f.Name == "CLOBTokenIDs" {
						id, err := parseUint256(value)
						if err != nil {
							return fmt.Errorf("clob_token_ids[%d]: %w", j, err)
						}
						value = id.String()
					}
					values = append(values, value)
				}
				if strings.Contains(opts, "comma") {
					v.Add(tag[0], strings.Join(values, ","))
				} else {
					for _, s := range values {
						v.Add(tag[0], s)
					}
				}
				continue
			}
			v.Add(tag[0], fmt.Sprint(x.Interface()))
		}
		return nil
	}
	return v, add(reflect.ValueOf(req))
}

func (c *Client) do(ctx context.Context, method, path string, query any, body any, dst any, text bool) error {
	u := c.Host + "/" + strings.TrimPrefix(path, "/")
	if query != nil {
		v, err := queryValues(query)
		if err != nil {
			return err
		}
		if s := v.Encode(); s != "" {
			u += "?" + s
		}
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "*/*")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "go-clob-client-v2")
	resp, err := c.HTTPClient.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		return &HTTPError{resp.StatusCode, method, u, string(b)}
	}
	if text {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		*dst.(*string) = string(b)
		return nil
	}
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return &HTTPError{http.StatusNotFound, method, u, "Unable to find requested resource"}
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return err
	}
	normalizeEmpty(reflect.ValueOf(dst))
	return nil
}

func normalizeEmpty(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		normalizeEmpty(v.Elem())
		return
	}
	switch v.Kind() {
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			normalizeEmpty(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if v.Type().Field(i).Tag.Get("empty") == "nil" && f.Kind() == reflect.Pointer && !f.IsNil() && f.Elem().Kind() == reflect.String && f.Elem().Len() == 0 {
				f.SetZero()
				continue
			}
			normalizeEmpty(f)
		}
	}
}

// Number preserves decimal and U256 text without converting through float64.
// JSON strings and numbers are accepted and it marshals as a JSON string.
// Response fields validate decimal or U256 constraints according to their model.
type Number string

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func (n *Number) UnmarshalJSON(b []byte) error {
	s, err := stringOrNumber(b)
	if err != nil {
		return err
	}
	if !decimalPattern.MatchString(s) && !(strings.HasPrefix(s, "0x") && validateUint256(s) == nil) {
		return fmt.Errorf("invalid number %q", s)
	}
	*n = Number(s)
	return nil
}

func (n Number) MarshalJSON() ([]byte, error) {
	if !decimalPattern.MatchString(string(n)) && !(strings.HasPrefix(string(n), "0x") && validateUint256(string(n)) == nil) {
		return nil, fmt.Errorf("invalid number %q", n)
	}
	return json.Marshal(string(n))
}

// StringFromAny accepts arbitrary strings and JSON numbers, as used for Gamma IDs.
type StringFromAny string

func (s *StringFromAny) UnmarshalJSON(b []byte) error {
	value, err := stringOrNumber(b)
	if err != nil {
		return err
	}
	if len(b) > 0 && b[0] != '"' {
		if strings.HasPrefix(value, "-") {
			_, err = strconv.ParseInt(value, 10, 64)
		} else {
			_, err = strconv.ParseUint(value, 10, 64)
		}
		if err != nil {
			return fmt.Errorf("expected string or integer: %w", err)
		}
	}
	*s = StringFromAny(value)
	return nil
}

func stringOrNumber(b []byte) (string, error) {
	if bytes.Equal(b, []byte("null")) {
		return "", fmt.Errorf("expected string or number, got null")
	}
	if len(b) > 0 && b[0] == '"' {
		var value string
		err := json.Unmarshal(b, &value)
		return value, err
	}
	var x json.Number
	if err := json.Unmarshal(b, &x); err != nil {
		return "", err
	}
	return x.String(), nil
}

// QuotedInt accepts an integer encoded either as a JSON string or number.
type QuotedInt int32

func (n *QuotedInt) UnmarshalJSON(b []byte) error {
	var s Number
	if err := s.UnmarshalJSON(b); err != nil {
		return err
	}
	i, err := strconv.ParseInt(string(s), 10, 32)
	if err != nil {
		return err
	}
	*n = QuotedInt(i)
	return nil
}
