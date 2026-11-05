package clob

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveVersionConcurrentCacheWrites(t *testing.T) {
	tests := []struct {
		name                 string
		firstForce           bool
		firstCompletesFirst  bool
		firstVersion         uint32
		secondVersion        uint32
		secondResponse       string
		secondStatus         int
		invalidateBeforeNext bool
		invalidateAfterNext  bool
		wantCached           uint32
		wantRequests         int32
	}{
		{name: "delayed cache miss", wantCached: 2, wantRequests: 2},
		{name: "delayed forced refresh", firstForce: true, wantCached: 2, wantRequests: 2},
		{name: "newer response may have lower version", firstVersion: 2, secondVersion: 1, wantCached: 1, wantRequests: 2},
		{name: "earlier response completes first", firstCompletesFirst: true, wantCached: 2, wantRequests: 2},
		{name: "failed newer refresh does not block older success", secondStatus: http.StatusInternalServerError, wantCached: 1, wantRequests: 2},
		{name: "malformed newer refresh does not block older success", secondResponse: `{}`, wantCached: 1, wantRequests: 2},
		{name: "invalidation before newer request", invalidateBeforeNext: true, wantCached: 2, wantRequests: 2},
		{name: "invalidation after newer response", invalidateAfterNext: true, wantCached: 3, wantRequests: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstVersion, secondVersion := tt.firstVersion, tt.secondVersion
			if firstVersion == 0 {
				firstVersion = 1
			}
			if secondVersion == 0 {
				secondVersion = 2
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			started := make(chan int32, 3)
			release := []chan struct{}{make(chan struct{}, 1), make(chan struct{}, 1)}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := requests.Add(1)
				started <- n
				if n <= 2 {
					select {
					case <-release[n-1]:
					case <-r.Context().Done():
						return
					}
				}
				if n == 2 && tt.secondStatus != 0 {
					http.Error(w, "refresh failed", tt.secondStatus)
					return
				}
				if n == 2 && tt.secondResponse != "" {
					fmt.Fprint(w, tt.secondResponse)
					return
				}
				version := uint32(n)
				if n == 1 {
					version = firstVersion
				} else if n == 2 {
					version = secondVersion
				}
				fmt.Fprintf(w, `{"version":%d}`, version)
			}))
			defer server.Close()
			defer cancel()

			client, err := NewClient(server.URL, Config{}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			type result struct {
				version uint32
				err     error
			}
			fetch := func(force bool) <-chan result {
				done := make(chan result, 1)
				go func() {
					v, err := client.ResolveVersion(ctx, force)
					done <- result{v, err}
				}()
				return done
			}
			awaitStart := func(want int32) {
				t.Helper()
				select {
				case got := <-started:
					if got != want {
						t.Fatalf("request number = %d, want %d", got, want)
					}
				case <-ctx.Done():
					t.Fatalf("waiting for request %d: %v", want, ctx.Err())
				}
			}
			awaitResult := func(ch <-chan result, want uint32, wantErr bool) {
				t.Helper()
				select {
				case got := <-ch:
					if (got.err != nil) != wantErr || (!wantErr && got.version != want) {
						t.Fatalf("ResolveVersion = (%d, %v), want (%d, error=%t)", got.version, got.err, want, wantErr)
					}
				case <-ctx.Done():
					t.Fatalf("waiting for response: %v", ctx.Err())
				}
			}

			first := fetch(tt.firstForce)
			awaitStart(1)
			if tt.invalidateBeforeNext {
				client.InvalidateInternalCaches()
			}
			second := fetch(true)
			awaitStart(2)
			if tt.firstCompletesFirst {
				release[0] <- struct{}{}
				awaitResult(first, firstVersion, false)
			}
			release[1] <- struct{}{}
			awaitResult(second, secondVersion, tt.secondStatus != 0 || tt.secondResponse != "")
			if tt.invalidateAfterNext {
				client.InvalidateInternalCaches()
			}
			if !tt.firstCompletesFirst {
				release[0] <- struct{}{}
				awaitResult(first, firstVersion, false)
			}
			got, err := client.Version(ctx)
			if err != nil || got != tt.wantCached {
				t.Fatalf("cached Version = (%d, %v), want %d", got, err, tt.wantCached)
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tt.wantRequests)
			}
		})
	}
}
