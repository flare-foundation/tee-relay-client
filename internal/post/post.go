// Package post sends JSON POST requests, retrying attempts that fail transiently.
package post

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
)

// Params bundles the HTTP call and retry configuration for WithRetry.
type Params struct {
	Call  call.Params
	Retry retry.Params
}

// WithRetry posts body to url and decodes the JSON response into T.
// Attempts that fail with a transient status — no HTTP response, 408, 429, or
// 5xx — are retried per p.Retry; any other failure is returned immediately.
// After retries the error also carries the attempt count and first failure.
// Returned error text is capped at maxErrLen bytes; non-200 reasons can
// carry up to 64 KiB of server-controlled text.
func WithRetry[S, T any](ctx context.Context, url string, key call.APIKey, body S, p Params) (*T, error) {
	// terminal statuses stop the loop but must still surface their error
	type attempt struct {
		res call.Response[T]
		err error
	}

	// mu guards attempts/firstErr: retry.Execute may abandon a stalled
	// attempt whose goroutine finishes later
	var mu sync.Mutex
	var firstErr error
	attempts := 0

	wrapped := func() (attempt, error) {
		res, err := call.Post[S, T](ctx, url, key, body, p.Call)
		mu.Lock()
		attempts++
		if err != nil && firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		switch {
		case err == nil:
			return attempt{res: res}, nil
		case retryable(res.Status):
			return attempt{res: res}, err
		default:
			return attempt{res: res, err: err}, nil
		}
	}

	st := retry.Execute(ctx, wrapped, p.Retry)

	mu.Lock()
	n, first := attempts, firstErr
	mu.Unlock()

	if st.Value.err != nil {
		return nil, capError(withHistory(st.Value.err, n, first))
	}
	if st.Err != nil {
		return nil, capError(withHistory(st.Err, n, first))
	}
	if st.Value.res.Message == nil { // call.Post sets Message on success; guards callers from a nil deref
		return nil, errors.New("response missing body")
	}

	return st.Value.res.Message, nil
}

// maxErrLen caps server-controlled text carried in returned errors.
const maxErrLen = 1024

// maxFirstErrLen caps the first failure inside withHistory so a huge first
// reason cannot push the final cause past the capError cut.
const maxFirstErrLen = 256

// withHistory prefixes err with the attempt count and first failure once retries happened.
func withHistory(err error, attempts int, first error) error {
	if attempts <= 1 || first == nil {
		return err
	}

	f := first.Error()
	if len(f) > maxFirstErrLen {
		f = f[:maxFirstErrLen] + "..."
	}

	return fmt.Errorf("%d attempts, first error: %s; %w", attempts, f, err)
}

// capError rebuilds an oversized err with its text capped, dropping the error chain.
func capError(err error) error {
	s := err.Error()
	if len(s) <= maxErrLen {
		return err
	}

	return errors.New(s[:maxErrLen] + "...")
}

// retryable reports whether an HTTP status may be retried.
// Zero means the call produced no HTTP response (transport error or timeout).
func retryable(status int) bool {
	switch {
	case status == 0, status == http.StatusRequestTimeout, status == http.StatusTooManyRequests:
		return true
	case status >= 500 && status <= 599:
		return true
	default:
		return false
	}
}
