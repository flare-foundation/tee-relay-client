package health_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flare-foundation/tee-relay-client/internal/health"
	"github.com/stretchr/testify/require"
)

// get performs a GET and returns the status code, body and response headers.
func get(t *testing.T, client *http.Client, url string) (int, string, http.Header) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // test helper; a close error is not what is under test

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body), resp.Header
}

func TestHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(*health.Status)
		startup     int
		ready       int
		readyReason string // prefix of the 503 body; the stale reason ends in a wall-clock age
	}{
		{
			name:        "fresh",
			setup:       func(*health.Status) {},
			startup:     http.StatusServiceUnavailable,
			ready:       http.StatusServiceUnavailable,
			readyReason: "startup not finished yet\n",
		},
		{
			name:        "started",
			setup:       func(s *health.Status) { s.SetStarted() },
			startup:     http.StatusOK,
			ready:       http.StatusServiceUnavailable,
			readyReason: "no indexer state observed yet\n",
		},
		{
			name: "ready",
			setup: func(s *health.Status) {
				s.SetStarted()
				s.SetIndexer(7, time.Now())
			},
			startup: http.StatusOK,
			ready:   http.StatusOK,
		},
		{
			name: "stale",
			setup: func(s *health.Status) {
				s.SetStarted()
				s.SetIndexer(7, time.Now().Add(-time.Hour))
			},
			startup:     http.StatusOK,
			ready:       http.StatusServiceUnavailable,
			readyReason: "indexer block too old: block 7 is ",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			s := health.NewStatus(30 * time.Second)
			test.setup(s)

			srv := httptest.NewServer(health.Handler(s))
			defer srv.Close()

			code, body, headers := get(t, srv.Client(), srv.URL+"/healthy")
			require.Equal(t, http.StatusOK, code)
			require.Empty(t, body)
			require.Equal(t, "0", headers.Get("Content-Length"))
			require.Equal(t, "no-store", headers.Get("Cache-Control"))

			code, body, _ = get(t, srv.Client(), srv.URL+"/startup")
			require.Equal(t, test.startup, code)
			if test.startup == http.StatusOK {
				require.Empty(t, body)
			}

			code, body, headers = get(t, srv.Client(), srv.URL+"/ready")
			require.Equal(t, test.ready, code)
			require.Equal(t, "no-store", headers.Get("Cache-Control"))
			if test.ready == http.StatusOK {
				require.Empty(t, body)
			} else {
				require.True(t, strings.HasPrefix(body, test.readyReason), body)
				require.Contains(t, headers.Get("Content-Type"), "text/plain")
			}
		})
	}
}

func TestHandlerRejects(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(health.Handler(health.NewStatus(time.Second)))
	defer srv.Close()

	code, _, _ := get(t, srv.Client(), srv.URL+"/")
	require.Equal(t, http.StatusNotFound, code)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/healthy", nil)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestListenStartClose(t *testing.T) {
	t.Parallel()

	s := health.NewStatus(time.Second)
	hs, err := health.Listen("127.0.0.1:0", s)
	require.NoError(t, err)
	require.NotEqual(t, "127.0.0.1:0", hs.Addr(), "Addr must report the resolved port")

	hs.Start()

	client := &http.Client{Timeout: 2 * time.Second}
	code, body, _ := get(t, client, "http://"+hs.Addr()+"/healthy")
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, body)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, hs.Close(ctx))

	// closed: the probe sees a transport error, not a status code
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+hs.Addr()+"/healthy", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	if err == nil {
		require.NoError(t, resp.Body.Close())
	}
	require.Error(t, err)
}

// A busy address is a startup failure, surfaced by Listen rather than by a goroutine later.
func TestListenOccupied(t *testing.T) {
	t.Parallel()

	first, err := health.Listen("127.0.0.1:0", nil)
	require.NoError(t, err)

	_, err = health.Listen(first.Addr(), nil)
	require.ErrorContains(t, err, "binding health server")

	// Close releases the bind even though Start never ran
	require.NoError(t, first.Close(t.Context()))
	second, err := health.Listen(first.Addr(), nil)
	require.NoError(t, err)
	require.NoError(t, second.Close(t.Context()))
}
