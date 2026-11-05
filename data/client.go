// Package data provides a client for the Polymarket Data API.
package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

const DefaultHost = "https://data-api.polymarket.com"

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
					values = append(values, fmt.Sprint(x.Index(j).Interface()))
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
	if err := validateQuery(query); err != nil {
		return err
	}
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

// Number preserves the exact representation of decimal and large integer values.
// Both JSON strings and JSON numbers are accepted. It marshals as a JSON string.
type Number string

// MarshalJSON retains the existing quoted wire form while rejecting malformed values.
func (n Number) MarshalJSON() ([]byte, error) {
	if err := validateNumberText(string(n)); err != nil {
		return nil, err
	}
	return json.Marshal(string(n))
}

func (n *Number) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return fmt.Errorf("number cannot be null")
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if err := validateNumberText(s); err != nil {
			return err
		}
		*n = Number(s)
		return nil
	}
	var x json.Number
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	if err := validateNumberText(x.String()); err != nil {
		return err
	}
	*n = Number(x.String())
	return nil
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
