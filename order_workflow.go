package clob

import (
	"context"

	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// BuildSignAndPostLimitOrder builds, signs and submits a limit order using the
// identity established by Client.Authenticate. On an order-version mismatch it
// retries at most once, after a successful refresh confirms that the token
// protocol version differs from the version actually submitted. Explicitly pinned
// versions and position (V3) orders are never retried. A retry rebuilds and
// re-signs the order. Inspect PostOrderResponse.Success even when err is nil.
//
// A network or decoding error may occur after submission. Such errors are wrapped
// in SubmissionError, which retains the signed order for reconciliation. The
// caller must not mutate p's pointer-backed fields while the call is running,
// and concurrent calls require a concurrency-safe signer.
func (a *AuthenticatedClient) BuildSignAndPostLimitOrder(ctx context.Context, signer HashSigner, p OrderParams, size string) (PostOrderResponse, error) {
	return a.buildSignAndPostOrder(ctx, signer, p, func(p OrderParams) (SignableOrder, error) {
		return a.BuildLimitOrder(ctx, p, size)
	})
}

// BuildSignAndPostMarketOrder builds, signs and submits a market order using
// the identity established by Client.Authenticate. Its retry and ambiguous
// submission behavior is the same as BuildSignAndPostLimitOrder. If a retry is
// warranted, market data and any balance-aware sizing are resolved again.
func (a *AuthenticatedClient) BuildSignAndPostMarketOrder(ctx context.Context, signer HashSigner, p OrderParams, amount MarketAmount) (PostOrderResponse, error) {
	return a.buildSignAndPostOrder(ctx, signer, p, func(p OrderParams) (SignableOrder, error) {
		return a.BuildMarketOrder(ctx, p, amount)
	})
}

// SubmissionError indicates that an order submission may have reached the server
// despite a transport, server, or response-decoding error. Order contains the actual signed
// payload; do not log its signature or blindly submit another order. Reconcile
// order and trade state first. Unwrap exposes the underlying error.
type SubmissionError struct {
	Order SignedOrder
	Err   error
}

func (e *SubmissionError) Error() string {
	return fmt.Sprintf("order submission outcome unknown: %v", e.Err)
}
func (e *SubmissionError) Unwrap() error { return e.Err }

// A confirmed mismatch must be a client-error response with the exact mismatch
// code, not arbitrary text from a proxy, an unrelated error, or a server failure.
func isOrderVersionRejection(status StatusError) bool {
	if status.StatusCode != http.StatusBadRequest {
		return false
	}
	body := strings.TrimSpace(status.Message)
	if body == "order_version_mismatch" {
		return true
	}
	fields, err := responseFields([]byte(body), nil, "error")
	if err != nil {
		return false
	}
	return string(fields["error"]) == `"order_version_mismatch"`
}

func (a *AuthenticatedClient) buildSignAndPostOrder(ctx context.Context, signer HashSigner, p OrderParams, build func(OrderParams) (SignableOrder, error)) (PostOrderResponse, error) {
	var response PostOrderResponse
	order, err := build(p)
	if err != nil {
		return response, err
	}
	signed, err := a.SignOrder(ctx, signer, order)
	if err != nil {
		return response, err
	}
	if err = a.validateSignedOrderIdentity(signed); err != nil {
		return response, err
	}

	// PostOrder refreshes on mismatch but discards refresh errors. Submit here
	// so a retry depends on the result of the actual refresh, not a stale cache.
	err = a.do(ctx, http.MethodPost, "order", nil, nil, signed, &response)
	if err == nil {
		responses := []PostOrderResponse{response}
		a.resolveTransactionHashes(ctx, responses, []SignedOrder{signed})
		return responses[0], nil
	}
	var status StatusError
	endpoint, parseErr := url.Parse(a.host + "/order")
	if !errors.As(err, &status) {
		return response, &SubmissionError{Order: signed, Err: err}
	}
	if status.StatusCode >= http.StatusInternalServerError || status.StatusCode == http.StatusRequestTimeout {
		return response, &SubmissionError{Order: signed, Err: err}
	}
	if parseErr != nil || status.Method != http.MethodPost || status.Path != endpoint.EscapedPath() || !isOrderVersionRejection(status) {
		return response, err
	}
	if p.Version != 0 || order.Order.Version == 3 {
		return response, err
	}

	version, refreshErr := a.publicClient.ResolveVersion(ctx, true)
	if refreshErr != nil {
		return response, errors.Join(err, fmt.Errorf("refresh order version: %w", refreshErr))
	}
	if (version != 1 && version != 2) || int(version) == order.Order.Version {
		return response, err
	}
	// V1 cannot represent V2 metadata or builder attribution. Do not silently
	// remove these fields on a protocol transition.
	if version == 1 && (order.Order.Metadata != (Hash{}) || order.Order.Builder != (Hash{})) {
		return response, err
	}

	// Pin the confirmed version even if another goroutine changes the cache.
	p.Version = int(version)
	order, buildErr := build(p)
	if buildErr != nil {
		return PostOrderResponse{}, errors.Join(err, fmt.Errorf("rebuild order: %w", buildErr))
	}
	signed, signErr := a.SignOrder(ctx, signer, order)
	if signErr != nil {
		return PostOrderResponse{}, errors.Join(err, fmt.Errorf("re-sign order: %w", signErr))
	}
	response, postErr := a.PostOrder(ctx, signed)
	if postErr != nil {
		var postStatus StatusError
		if !errors.As(postErr, &postStatus) || postStatus.StatusCode >= http.StatusInternalServerError || postStatus.StatusCode == http.StatusRequestTimeout {
			return response, &SubmissionError{Order: signed, Err: postErr}
		}
	}
	return response, postErr
}
