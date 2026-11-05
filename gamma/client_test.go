package gamma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRoutes(t *testing.T) {
	ctx := context.Background()
	var path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != path {
			t.Errorf("got %s %s, want GET %s", r.Method, r.URL.Path, path)
		}
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, srv.Client())
	cases := []struct {
		path, body string
		call       func() error
	}{
		{"/status", "OK", func() error { _, e := c.Status(ctx); return e }},
		{"/teams", "[]", func() error { _, e := c.Teams(ctx, TeamsRequest{}); return e }},
		{"/sports", "[]", func() error { _, e := c.Sports(ctx); return e }},
		{"/sports/market-types", `{"marketTypes":[]}`, func() error { _, e := c.SportsMarketTypes(ctx); return e }},
		{"/tags", "[]", func() error { _, e := c.Tags(ctx, TagsRequest{}); return e }},
		{"/tags/1", `{"id":"1"}`, func() error { _, e := c.TagByID(ctx, TagByIdRequest{ID: "1"}); return e }},
		{"/tags/slug/s", `{"id":"1"}`, func() error { _, e := c.TagBySlug(ctx, TagBySlugRequest{Slug: "s"}); return e }},
		{"/tags/1/related-tags", "[]", func() error { _, e := c.RelatedTagsByID(ctx, RelatedTagsByIdRequest{ID: "1"}); return e }},
		{"/tags/slug/s/related-tags", "[]", func() error { _, e := c.RelatedTagsBySlug(ctx, RelatedTagsBySlugRequest{Slug: "s"}); return e }},
		{"/tags/1/related-tags/tags", "[]", func() error { _, e := c.TagsRelatedToTagByID(ctx, RelatedTagsByIdRequest{ID: "1"}); return e }},
		{"/tags/slug/s/related-tags/tags", "[]", func() error { _, e := c.TagsRelatedToTagBySlug(ctx, RelatedTagsBySlugRequest{Slug: "s"}); return e }},
		{"/events", "[]", func() error { _, e := c.Events(ctx, EventsRequest{}); return e }},
		{"/events/1", `{"id":"1"}`, func() error { _, e := c.EventByID(ctx, EventByIdRequest{ID: "1"}); return e }},
		{"/events/slug/s", `{"id":"1"}`, func() error { _, e := c.EventBySlug(ctx, EventBySlugRequest{Slug: "s"}); return e }},
		{"/events/1/tags", "[]", func() error { _, e := c.EventTags(ctx, EventTagsRequest{ID: "1"}); return e }},
		{"/markets", "[]", func() error { _, e := c.Markets(ctx, MarketsRequest{}); return e }},
		{"/markets/1", `{"id":"1"}`, func() error { _, e := c.MarketByID(ctx, MarketByIdRequest{ID: "1"}); return e }},
		{"/markets/slug/s", `{"id":"1"}`, func() error { _, e := c.MarketBySlug(ctx, MarketBySlugRequest{Slug: "s"}); return e }},
		{"/markets/1/tags", "[]", func() error { _, e := c.MarketTags(ctx, MarketTagsRequest{ID: "1"}); return e }},
		{"/series", "[]", func() error { _, e := c.Series(ctx, SeriesListRequest{}); return e }},
		{"/series/1", `{"id":"1"}`, func() error { _, e := c.SeriesByID(ctx, SeriesByIdRequest{ID: "1"}); return e }},
		{"/comments", "[]", func() error { _, e := c.Comments(ctx, CommentsRequest{}); return e }},
		{"/comments/1", "[]", func() error { _, e := c.CommentsByID(ctx, CommentsByIdRequest{ID: "1"}); return e }},
		{"/comments/user_address/0x1234567890123456789012345678901234567890", "[]", func() error {
			_, e := c.CommentsByUserAddress(ctx, CommentsByUserAddressRequest{UserAddress: "0x1234567890123456789012345678901234567890"})
			return e
		}},
		{"/public-profile", "{}", func() error {
			_, e := c.PublicProfile(ctx, PublicProfileRequest{Address: "0x1234567890123456789012345678901234567890"})
			return e
		}},
		{"/public-search", "{}", func() error { _, e := c.Search(ctx, SearchRequest{}); return e }},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			path, body = tt.path, tt.body
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQueryAndNestedModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if !reflect.DeepEqual(q["slug"], []string{"a", "b"}) || q.Get("active") != "false" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `[{"id":"1","published_at":"legacy","markets":[{"id":"2","conditionId":"","outcomes":"[\"Yes\",\"No\"]","outcomePrices":"[\"0.1234567890123456789\",\"0.8\"]","clobTokenIds":"[\"123456789012345678901234567890\"]","twitterCardLastRefreshed":"1700000000000","submitted_by":"legacy"}]}]`)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	f := false
	got, err := c.Events(context.Background(), EventsRequest{Slug: []string{"a", "b"}, Active: &f})
	if err != nil {
		t.Fatal(err)
	}
	m := (*got[0].Markets)[0]
	if got[0].PublishedAt == nil || *got[0].PublishedAt != "legacy" || m.ConditionID != nil {
		t.Fatalf("alias or empty value not decoded: %+v %+v", got[0], m)
	}
	if m.TwitterCardLastRefreshed == nil || *m.TwitterCardLastRefreshed != 1700000000000 || m.SubmittedBy == nil || *m.SubmittedBy != "legacy" {
		t.Fatalf("nested market fields: %+v", m)
	}
	if (*m.Outcomes)[0] != "Yes" || (*m.OutcomePrices)[0] != "0.1234567890123456789" || (*m.CLOBTokenIDs)[0] != "123456789012345678901234567890" {
		t.Fatalf("%+v", m)
	}
}

func TestSportsAndEscapedPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sports":
			fmt.Fprint(w, `[{"sport":"nba","image":"","resolution":"","ordering":"","tags":"1,2,3","series":""}]`)
		case "/tags/slug/a/b":
			if r.URL.EscapedPath() != "/tags/slug/a%2Fb" {
				t.Errorf("path %q", r.URL.EscapedPath())
			}
			if r.URL.Query().Get("include_template") != "false" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"id":"x"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	ctx := context.Background()
	sports, err := c.Sports(ctx)
	if err != nil || len(sports) != 1 || !reflect.DeepEqual([]string(sports[0].Tags), []string{"1", "2", "3"}) {
		t.Fatalf("%v %v", sports, err)
	}
	f := false
	_, err = c.TagBySlug(ctx, TagBySlugRequest{Slug: "a/b", IncludeTemplate: &f})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMarketRefreshAndSubmittedBy(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       *int64
		alias      string
	}{
		{"quoted", `{"id":"1","twitterCardLastRefreshed":"1700000000000","submitted_by":"legacy"}`, ptrInt64(1700000000000), "legacy"},
		{"number", `{"id":"1","twitterCardLastRefreshed":1700000000000,"submittedBy":"current"}`, ptrInt64(1700000000000), "current"},
		{"null", `{"id":"1","twitterCardLastRefreshed":null}`, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var m Market
			if err := json.Unmarshal([]byte(tt.body), &m); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(m.TwitterCardLastRefreshed, tt.want) {
				t.Fatalf("refresh: %v", m.TwitterCardLastRefreshed)
			}
			if tt.alias != "" && (m.SubmittedBy == nil || *m.SubmittedBy != tt.alias) {
				t.Fatalf("submittedBy: %v", m.SubmittedBy)
			}
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var encoded map[string]any
			if err := json.Unmarshal(b, &encoded); err != nil {
				t.Fatal(err)
			}
			if tt.want != nil && encoded["twitterCardLastRefreshed"] != "1700000000000" {
				t.Fatalf("encoded: %s", b)
			}
		})
	}
	var m Market
	if err := json.Unmarshal([]byte(`{"id":"1","twitterCardLastRefreshed":"123oops"}`), &m); err == nil {
		t.Fatal("expected invalid timestamp error")
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestNullMarketResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "null") }))
	defer srv.Close()
	c, _ := NewClient(srv.URL)
	_, err := c.MarketByID(context.Background(), MarketByIdRequest{ID: "missing"})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("null response: %v", err)
	}
}

func TestPagination(t *testing.T) {
	var offsets []int32
	var items []int
	err := StreamData(context.Background(), 2, func(_ context.Context, limit, offset int32) ([]int, error) {
		offsets = append(offsets, offset)
		if offset == 0 {
			return []int{1, 2}, nil
		}
		return []int{3}, nil
	}, func(i int) error { items = append(items, i); return nil })
	if err != nil || !reflect.DeepEqual(offsets, []int32{0, 2}) || !reflect.DeepEqual(items, []int{1, 2, 3}) {
		t.Fatalf("%v %v %v", offsets, items, err)
	}
	sentinel := errors.New("stop")
	err = StreamData(context.Background(), 1, func(context.Context, int32, int32) ([]int, error) { return []int{1}, nil }, func(int) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
