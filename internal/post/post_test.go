package post

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/stretchr/testify/require"
)

type reply struct {
	Value string `json:"value"`
}

func testParams() Params {
	return Params{
		Call:  call.Params{Timeout: time.Second, MaxResponseSize: 1 << 20},
		Retry: retry.Params{MaxAttempts: 3, Delay: time.Millisecond, Timeout: 5 * time.Second},
	}
}

// server serves status for the first fail requests, then 200 with body, counting calls.
func server(t *testing.T, status, fail int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if int(calls.Add(1)) <= fail {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv, calls
}

func TestWithRetryTransient(t *testing.T) {
	t.Parallel()

	codes := []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
	for _, code := range codes {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()
			srv, calls := server(t, code, 1, `{"value":"ok"}`)

			res, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
			require.NoError(t, err)
			require.Equal(t, "ok", res.Value)
			require.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestWithRetryTerminal(t *testing.T) {
	t.Parallel()

	codes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusUnprocessableEntity,
	}
	for _, code := range codes {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()
			srv, calls := server(t, code, 1, `{"value":"ok"}`)

			_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
			require.ErrorContains(t, err, strconv.Itoa(code))
			require.NotContains(t, err.Error(), "attempts") // no history prefix on a single attempt
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

// historyServer answers the first request with firstStatus/firstBody and later
// requests with status/body, all text/plain, counting calls.
func historyServer(t *testing.T, firstStatus int, firstBody string, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if calls.Add(1) == 1 {
			w.WriteHeader(firstStatus)
			_, _ = io.WriteString(w, firstBody)
			return
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv, calls
}

func TestWithRetryErrorHistory(t *testing.T) {
	t.Parallel()
	srv, calls := historyServer(t, http.StatusServiceUnavailable, "first-cause", http.StatusBadGateway, "later-cause")

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, "3 attempts")
	require.ErrorContains(t, err, "first-cause")
	require.ErrorContains(t, err, "later-cause")
	require.EqualValues(t, 3, calls.Load())
}

func TestWithRetryErrorHistoryTerminal(t *testing.T) {
	t.Parallel()
	srv, calls := historyServer(t, http.StatusServiceUnavailable, "flap", http.StatusBadRequest, "bad request body")

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, "2 attempts")
	require.ErrorContains(t, err, "flap")
	require.ErrorContains(t, err, "bad request body")
	require.EqualValues(t, 2, calls.Load())
}

func TestWithRetryErrorHistoryHugeFirstKeepsLast(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 64<<10)
	srv, _ := historyServer(t, http.StatusServiceUnavailable, long, http.StatusBadGateway, "later-cause")

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, "later-cause") // final cause survives a huge first reason
	require.LessOrEqual(t, len(err.Error()), maxErrLen+len("..."))
}

func TestWithRetryExhausted(t *testing.T) {
	t.Parallel()
	srv, calls := server(t, http.StatusServiceUnavailable, 10, `{"value":"ok"}`)

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, strconv.Itoa(http.StatusServiceUnavailable))
	require.EqualValues(t, 3, calls.Load())
}

func TestWithRetryBadJSON(t *testing.T) {
	t.Parallel()
	srv, calls := server(t, http.StatusOK, 0, `not-json`)

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, "decoding response")
	require.EqualValues(t, 1, calls.Load())
}

func TestWithRetryErrorTextCapped(t *testing.T) {
	t.Parallel()

	// call.PostRaw embeds up to 64 KiB of a text/plain non-200 body in its error
	long := strings.Repeat("x", 64<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, long)
	}))
	t.Cleanup(srv.Close)

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, testParams())
	require.Error(t, err)
	require.LessOrEqual(t, len(err.Error()), maxErrLen+len("..."))
}

// TestWithRetryBudgetExpiry expires the retry budget while an attempt is still
// in flight: retry.Execute abandons the attempt goroutine, which finishes after
// WithRetry returned — srv.Close in Cleanup blocks on the handler, so the late
// counter write lands inside the race detector's window.
func TestWithRetryBudgetExpiry(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	p := Params{
		Call:  call.Params{Timeout: time.Second, MaxResponseSize: 1 << 20},
		Retry: retry.Params{MaxAttempts: 100, Delay: time.Millisecond, Timeout: 50 * time.Millisecond},
	}

	_, err := WithRetry[struct{}, reply](context.Background(), srv.URL, call.NoAPIKey, struct{}{}, p)
	require.Error(t, err)
}

func TestWithRetryNoServer(t *testing.T) {
	t.Parallel()

	// closed server → transport error on every attempt
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := WithRetry[struct{}, reply](context.Background(), url, call.NoAPIKey, struct{}{}, testParams())
	require.ErrorContains(t, err, "all attempts failed")
}
