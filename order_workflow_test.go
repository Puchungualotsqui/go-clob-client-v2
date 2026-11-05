package clob

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

type workflowPost struct {
	Owner string `json:"owner"`
	Order struct {
		Salt          uint64        `json:"salt"`
		Maker         string        `json:"maker"`
		Signer        string        `json:"signer"`
		Taker         string        `json:"taker"`
		TokenID       string        `json:"tokenId"`
		MakerAmount   string        `json:"makerAmount"`
		TakerAmount   string        `json:"takerAmount"`
		Expiration    string        `json:"expiration"`
		Nonce         string        `json:"nonce"`
		FeeRateBPS    string        `json:"feeRateBps"`
		Timestamp     string        `json:"timestamp"`
		Metadata      string        `json:"metadata"`
		Builder       string        `json:"builder"`
		Side          string        `json:"side"`
		SignatureType SignatureType `json:"signatureType"`
		Signature     string        `json:"signature"`
	} `json:"order"`
}

// Verify the signature over the fields actually sent over HTTP, including the
// version-specific domain. Fixed salt and timestamp make stale signatures visible.
func workflowVerifyPost(t *testing.T, post workflowPost, version int, signer Address) {
	t.Helper()
	integer := func(s string) *big.Int {
		t.Helper()
		n, ok := new(big.Int).SetString(s, 10)
		if !ok {
			t.Fatalf("invalid integer %q in post", s)
		}
		return n
	}
	addr := func(s string) Address {
		t.Helper()
		a, err := ParseAddress(s)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	hash := func(s string) Hash {
		t.Helper()
		b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil || len(b) != 32 {
			t.Fatalf("invalid hash %q", s)
		}
		var h Hash
		copy(h[:], b)
		return h
	}
	p := post.Order
	if post.Owner != "owner" || addr(p.Maker) != signer || addr(p.Signer) != signer || p.Side != "BUY" || p.SignatureType != SignatureEOA {
		t.Fatalf("unexpected posted identity: %+v", post)
	}
	o := Order{Version: version, Salt: p.Salt, Maker: addr(p.Maker), Signer: addr(p.Signer), TokenID: integer(p.TokenID), MakerAmount: integer(p.MakerAmount), TakerAmount: integer(p.TakerAmount), Expiration: integer(p.Expiration), SignatureType: p.SignatureType, Side: Buy}
	if version == 1 {
		o.Taker, o.Nonce, o.FeeRateBPS = addr(p.Taker), integer(p.Nonce), integer(p.FeeRateBPS)
		if p.Timestamp != "" {
			t.Fatal("V1 payload has V2 timestamp")
		}
	} else {
		o.Timestamp, o.Metadata, o.Builder = integer(p.Timestamp), hash(p.Metadata), hash(p.Builder)
		if p.Taker != "" || p.Nonce != "" || p.FeeRateBPS != "" {
			t.Fatal("V2/V3 payload has V1 fields")
		}
	}
	digest, err := OrderDigest(o, Polygon, false)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(p.Signature, "0x"))
	if err != nil || len(sig) != 65 || (sig[64] != 27 && sig[64] != 28) {
		t.Fatalf("invalid signature %q", p.Signature)
	}
	sig[64] -= 27
	pub, err := ethcrypto.SigToPub(digest[:], sig)
	if err != nil {
		t.Fatal(err)
	}
	if got := ethcrypto.PubkeyToAddress(*pub).Bytes(); !reflect.DeepEqual(got, signer[:]) {
		t.Fatalf("signature recovered %x, want %s", got, signer)
	}
}

type workflowFixture struct {
	mu           sync.Mutex
	versions     []string
	postStatuses []int
	postBodies   []string
	bookBodies   []string
	counts       map[string]int
	posts        []workflowPost
	methods      []string
}

func (f *workflowFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[r.URL.Path]++
	f.methods = append(f.methods, r.Method+" "+r.URL.Path)
	pick := func(values []string, n int, fallback string) string {
		if n <= len(values) {
			return values[n-1]
		}
		return fallback
	}
	switch r.URL.Path {
	case "/version":
		body := pick(f.versions, f.counts[r.URL.Path], `{"version":2}`)
		if body == "FAIL" {
			http.Error(w, "refresh failed", 503)
			return
		}
		fmt.Fprint(w, body)
	case "/tick-size":
		fmt.Fprint(w, `{"minimum_tick_size":"0.01"}`)
	case "/fee-rate":
		fmt.Fprint(w, `{"base_fee":42}`)
	case "/neg-risk":
		fmt.Fprint(w, `{"neg_risk":false}`)
	case "/book":
		body := pick(f.bookBodies, f.counts[r.URL.Path], `{"asset_id":"1","timestamp":"1000","tick_size":"0.01","neg_risk":false,"asks":[{"price":"0.5","size":"100"}],"bids":[]}`)
		if body == "FAIL" {
			http.Error(w, "book failed", 503)
			return
		}
		fmt.Fprint(w, body)
	case "/order":
		var post workflowPost
		if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.posts = append(f.posts, post)
		n := len(f.posts)
		status := 200
		if n <= len(f.postStatuses) {
			status = f.postStatuses[n-1]
		}
		body := pick(f.postBodies, n, `{"makingAmount":"0","takingAmount":"0","orderID":"id","status":"LIVE","success":true}`)
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	default:
		http.Error(w, "unexpected route", 404)
	}
}

func workflowSetup(t *testing.T, f *workflowFixture, prefix string) (*AuthenticatedClient, *LocalSigner) {
	t.Helper()
	f.counts = make(map[string]int)
	handler := http.Handler(f)
	if prefix != "" {
		handler = http.StripPrefix(prefix, handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := NewClient(server.URL+prefix, DefaultConfig(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewLocalSigner("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	creds := Credentials{Key: "owner", Secret: "c2VjcmV0"}
	a, err := c.Authenticate(context.Background(), signer, Polygon, AuthenticationOptions{Credentials: &creds})
	if err != nil {
		t.Fatal(err)
	}
	return a, signer
}

func workflowParams() OrderParams {
	salt, timestamp := uint64(12345), uint64(1700000000000)
	return OrderParams{AssetID: "1", Price: "0.5", Salt: &salt, TimestampMS: &timestamp}
}

func workflowMismatch(t *testing.T, err error, path string) {
	t.Helper()
	var status StatusError
	if !errors.As(err, &status) || status.StatusCode != 400 || status.Method != http.MethodPost || status.Path != path || !strings.Contains(status.Message, "order_version_mismatch") {
		t.Fatalf("expected POST mismatch at %s, got %v", path, err)
	}
}

const workflowMismatchBody = `{"error":"order_version_mismatch"}`

func TestWorkflowLimitVersionRetry(t *testing.T) {
	for _, versions := range [][]string{{`{"version":1}`, `{"version":2}`}, {`{"version":2}`, `{"version":1}`}} {
		t.Run(strings.Join(versions, "_"), func(t *testing.T) {
			f := &workflowFixture{versions: versions, postStatuses: []int{400, 200}, postBodies: []string{workflowMismatchBody}}
			a, signer := workflowSetup(t, f, "/api")
			response, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, workflowParams(), "10")
			if err != nil || !response.Success {
				t.Fatalf("response %+v, error %v", response, err)
			}
			if len(f.posts) != 2 || f.counts["/version"] != 2 {
				t.Fatalf("counts %v posts %d", f.counts, len(f.posts))
			}
			first, second := 1, 2
			if strings.Contains(versions[0], "2") {
				first, second = 2, 1
			}
			workflowVerifyPost(t, f.posts[0], first, signer.Address())
			workflowVerifyPost(t, f.posts[1], second, signer.Address())
			if f.posts[0].Order.Signature == f.posts[1].Order.Signature || reflect.DeepEqual(f.posts[0].Order, f.posts[1].Order) {
				t.Fatal("retry reused payload or signature")
			}
			for _, method := range f.methods {
				if strings.HasSuffix(method, " /order") && method != "POST /order" {
					t.Fatalf("wrong method: %s", method)
				}
			}
		})
	}
}

func TestWorkflowNoRetry(t *testing.T) {
	for _, tc := range []struct {
		name             string
		versions         []string
		pversion         int
		status           int
		body             string
		wantVersions     int
		wantSuccessFalse bool
	}{
		{"same version", []string{`{"version":2}`, `{"version":2}`}, 0, 400, workflowMismatchBody, 2, false},
		{"refresh failure", []string{`{"version":2}`, "FAIL"}, 0, 400, workflowMismatchBody, 2, false},
		{"unsupported refreshed version", []string{`{"version":2}`, `{"version":3}`}, 0, 400, workflowMismatchBody, 2, false},
		{"unrelated status", []string{`{"version":2}`}, 0, 503, `{"error":"unavailable"}`, 1, false},
		{"server failure with marker", []string{`{"version":2}`}, 0, 500, workflowMismatchBody, 1, false},
		{"unrelated message containing marker", []string{`{"version":2}`}, 0, 400, `{"error":"not order_version_mismatch"}`, 1, false},
		{"duplicate error field", []string{`{"version":2}`}, 0, 400, `{"error":"other","error":"order_version_mismatch"}`, 1, false},
		{"marker in unrelated field", []string{`{"version":2}`}, 0, 400, `{"error":"other","note":"order_version_mismatch"}`, 1, false},
		{"success false", []string{`{"version":2}`}, 0, 200, `{"makingAmount":"0","takingAmount":"0","orderID":"","status":"","success":false,"errorMsg":"order_version_mismatch"}`, 1, true},
		{"position V3", []string{`{"version":1}`}, 3, 400, workflowMismatchBody, -1, false},
		{"pinned V1", []string{`{"version":2}`}, 1, 400, workflowMismatchBody, -1, false},
		{"pinned V2", []string{`{"version":1}`}, 2, 400, workflowMismatchBody, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &workflowFixture{versions: tc.versions, postStatuses: []int{tc.status}, postBodies: []string{tc.body}}
			a, signer := workflowSetup(t, f, "")
			p := workflowParams()
			p.Version = tc.pversion
			response, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, p, "10")
			if tc.wantSuccessFalse {
				if err != nil || response.Success || response.ErrorMsg == nil || *response.ErrorMsg != "order_version_mismatch" {
					t.Fatalf("response %+v, err %v", response, err)
				}
			} else if tc.status == 400 && tc.body == workflowMismatchBody {
				workflowMismatch(t, err, "/order")
			} else {
				var status StatusError
				if !errors.As(err, &status) || status.StatusCode != tc.status || status.Method != "POST" || status.Path != "/order" {
					t.Fatalf("unexpected error %v", err)
				}
			}
			if tc.status >= 500 {
				var submission *SubmissionError
				if !errors.As(err, &submission) || submission.Order.Signature == "" {
					t.Fatalf("server failure missing reconciliation payload: %v", err)
				}
			}
			if len(f.posts) != 1 || (tc.wantVersions >= 0 && f.counts["/version"] != tc.wantVersions) {
				t.Fatalf("counts %v posts %d", f.counts, len(f.posts))
			}
			v := tc.pversion
			if v == 0 {
				v = 2
			}
			workflowVerifyPost(t, f.posts[0], v, signer.Address())
		})
	}
}

func TestWorkflowDoesNotDropV2Metadata(t *testing.T) {
	for _, field := range []string{"metadata", "builder", "configured builder"} {
		t.Run(field, func(t *testing.T) {
			f := &workflowFixture{versions: []string{`{"version":2}`, `{"version":1}`}, postStatuses: []int{400}, postBodies: []string{workflowMismatchBody}}
			a, signer := workflowSetup(t, f, "")
			p := workflowParams()
			switch field {
			case "metadata":
				p.Metadata[31] = 1
			case "builder":
				p.Builder[31] = 1
			case "configured builder":
				code := B256("0x" + strings.Repeat("0", 63) + "1")
				a.publicClient.config.BuilderCode = &code
			}
			_, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, p, "10")
			workflowMismatch(t, err, "/order")
			if len(f.posts) != 1 {
				t.Fatalf("submitted %d orders", len(f.posts))
			}
		})
	}
}

func TestWorkflowMismatchPreservesPrefixedPath(t *testing.T) {
	f := &workflowFixture{versions: []string{`{"version":2}`, `{"version":2}`}, postStatuses: []int{400}, postBodies: []string{workflowMismatchBody}}
	a, signer := workflowSetup(t, f, "/api")
	_, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, workflowParams(), "10")
	workflowMismatch(t, err, "/api/order")
	if len(f.posts) != 1 || f.counts["/version"] != 2 {
		t.Fatalf("counts %v posts %d", f.counts, len(f.posts))
	}
}

func TestWorkflowRepeatedMismatch(t *testing.T) {
	f := &workflowFixture{versions: []string{`{"version":1}`, `{"version":2}`, `{"version":1}`}, postStatuses: []int{400, 400}, postBodies: []string{workflowMismatchBody, workflowMismatchBody}}
	a, signer := workflowSetup(t, f, "")
	_, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, workflowParams(), "10")
	workflowMismatch(t, err, "/order")
	if len(f.posts) != 2 {
		t.Fatalf("posted %d times, want 2", len(f.posts))
	}
	workflowVerifyPost(t, f.posts[0], 1, signer.Address())
	workflowVerifyPost(t, f.posts[1], 2, signer.Address())
}

func TestWorkflowMarketRefetch(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("second_book_failure=%v", fail), func(t *testing.T) {
			second := `{"asset_id":"1","timestamp":"1000","tick_size":"0.01","neg_risk":false,"asks":[{"price":"0.7","size":"100"}],"bids":[]}`
			if fail {
				second = "FAIL"
			}
			f := &workflowFixture{versions: []string{`{"version":1}`, `{"version":2}`}, postStatuses: []int{400, 200}, postBodies: []string{workflowMismatchBody}, bookBodies: []string{`{"asset_id":"1","timestamp":"1000","tick_size":"0.01","neg_risk":false,"asks":[{"price":"0.5","size":"100"}],"bids":[]}`, second}}
			a, signer := workflowSetup(t, f, "")
			p := workflowParams()
			p.Price = ""
			response, err := a.BuildSignAndPostMarketOrder(context.Background(), signer, p, MarketAmount{Value: "2"})
			if fail {
				if err == nil || !strings.Contains(err.Error(), "resolve order book") {
					t.Fatalf("expected book failure, got %v", err)
				}
			} else if err != nil || !response.Success {
				t.Fatalf("response %+v err %v", response, err)
			}
			wantPosts := 2
			if fail {
				wantPosts = 1
			}
			if f.counts["/book"] != 2 || len(f.posts) != wantPosts {
				t.Fatalf("counts %v posts %d", f.counts, len(f.posts))
			}
			workflowVerifyPost(t, f.posts[0], 1, signer.Address())
			if !fail {
				workflowVerifyPost(t, f.posts[1], 2, signer.Address())
				if f.posts[0].Order.MakerAmount == f.posts[1].Order.MakerAmount || f.posts[0].Order.Signature == f.posts[1].Order.Signature {
					t.Fatal("retry did not use updated market price and signature")
				}
			}
		})
	}
}

type workflowRoundTripper func(*http.Request) (*http.Response, error)

func (f workflowRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkflowPostErrorsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		postErr error
	}{
		{"transport", errors.New("transport order_version_mismatch")},
		{"wrong method", NewStatusError(400, "GET", "/order", "order_version_mismatch")},
		{"wrong path", NewStatusError(400, "POST", "/orders", "order_version_mismatch")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &workflowFixture{versions: []string{`{"version":2}`}}
			a, signer := workflowSetup(t, f, "")
			original := a.httpClient.Transport
			calls := 0
			a.httpClient.Transport = workflowRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/order" {
					calls++
					_, _ = io.Copy(io.Discard, r.Body)
					return nil, tc.postErr
				}
				return original.RoundTrip(r)
			})
			_, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, workflowParams(), "10")
			if !errors.Is(err, tc.postErr) || calls != 1 || f.counts["/version"] != 1 {
				t.Fatalf("error %v, calls %d, counts %v", err, calls, f.counts)
			}
			if tc.name == "transport" {
				var submission *SubmissionError
				if !errors.As(err, &submission) || submission.Order.Signature == "" || submission.Order.Order.TokenID.String() != "1" {
					t.Fatalf("missing reconciliation payload: %v", err)
				}
			}
		})
	}
	t.Run("cancelled context", func(t *testing.T) {
		f := &workflowFixture{}
		a, signer := workflowSetup(t, f, "")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.BuildSignAndPostLimitOrder(ctx, signer, workflowParams(), "10")
		if !errors.Is(err, context.Canceled) || len(f.posts) != 0 {
			t.Fatalf("err %v, posts %d", err, len(f.posts))
		}
	})
}

func TestWorkflowMarketPinnedAndInitialFailure(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("pinned_V%d", version), func(t *testing.T) {
			refreshed := 3 - version
			f := &workflowFixture{versions: []string{fmt.Sprintf(`{"version":%d}`, refreshed)}, postStatuses: []int{400}, postBodies: []string{workflowMismatchBody}}
			a, signer := workflowSetup(t, f, "")
			p := workflowParams()
			p.Version = version
			p.Price = ""
			_, err := a.BuildSignAndPostMarketOrder(context.Background(), signer, p, MarketAmount{Value: "2"})
			workflowMismatch(t, err, "/order")
			if len(f.posts) != 1 || f.counts["/book"] != 1 {
				t.Fatalf("counts %v posts %d", f.counts, len(f.posts))
			}
			workflowVerifyPost(t, f.posts[0], version, signer.Address())
		})
	}
	t.Run("first book fails", func(t *testing.T) {
		f := &workflowFixture{versions: []string{`{"version":2}`}, bookBodies: []string{"FAIL"}}
		a, signer := workflowSetup(t, f, "")
		p := workflowParams()
		p.Price = ""
		_, err := a.BuildSignAndPostMarketOrder(context.Background(), signer, p, MarketAmount{Value: "2"})
		if err == nil || !strings.Contains(err.Error(), "resolve order book") || len(f.posts) != 0 || f.counts["/version"] != 1 {
			t.Fatalf("err %v counts %v posts %d", err, f.counts, len(f.posts))
		}
	})
}

func TestWorkflowRetryTransportFailureRetainsSecondOrder(t *testing.T) {
	f := &workflowFixture{versions: []string{`{"version":1}`, `{"version":2}`}, postStatuses: []int{400}, postBodies: []string{workflowMismatchBody}}
	a, signer := workflowSetup(t, f, "")
	original := a.httpClient.Transport
	transportErr := errors.New("connection lost after submission")
	calls := 0
	a.httpClient.Transport = workflowRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/order" {
			calls++
			if calls == 2 {
				return nil, transportErr
			}
		}
		return original.RoundTrip(r)
	})
	_, err := a.BuildSignAndPostLimitOrder(context.Background(), signer, workflowParams(), "10")
	var submission *SubmissionError
	if !errors.Is(err, transportErr) || !errors.As(err, &submission) || submission.Order.Order.Version != 2 || submission.Order.Signature == "" || calls != 2 {
		t.Fatalf("retry error %v, calls %d", err, calls)
	}
}

func TestWorkflowCancellationDuringRefresh(t *testing.T) {
	f := &workflowFixture{versions: []string{`{"version":2}`}, postStatuses: []int{400}, postBodies: []string{workflowMismatchBody}}
	a, signer := workflowSetup(t, f, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc := *a.publicClient.httpClient
	original := hc.Transport
	versionCalls := 0
	hc.Transport = workflowRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/version" {
			versionCalls++
			if versionCalls == 2 {
				cancel()
				return nil, r.Context().Err()
			}
		}
		return original.RoundTrip(r)
	})
	a.publicClient.httpClient = &hc
	_, err := a.BuildSignAndPostLimitOrder(ctx, signer, workflowParams(), "10")
	workflowMismatch(t, err, "/order")
	if ctx.Err() != context.Canceled || versionCalls != 2 || len(f.posts) != 1 {
		t.Fatalf("context %v, version calls %d, posts %d", ctx.Err(), versionCalls, len(f.posts))
	}
}

func TestWorkflowWrongSigner(t *testing.T) {
	for _, market := range []bool{false, true} {
		t.Run(fmt.Sprintf("market=%v", market), func(t *testing.T) {
			f := &workflowFixture{versions: []string{`{"version":2}`}}
			a, _ := workflowSetup(t, f, "")
			other, err := NewLocalSigner("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
			if err != nil {
				t.Fatal(err)
			}
			if market {
				_, err = a.BuildSignAndPostMarketOrder(context.Background(), other, workflowParams(), MarketAmount{Value: "2"})
			} else {
				_, err = a.BuildSignAndPostLimitOrder(context.Background(), other, workflowParams(), "10")
			}
			if err == nil || len(f.posts) != 0 {
				t.Fatalf("wrong signer: err %v posts %d", err, len(f.posts))
			}
		})
	}
}
