package processors

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVerifier returns a Verifier for url with the retry delay shrunk for test speed.
func testVerifier(url string) *Verifier {
	v := NewVerifier(&config.Credentials{URL: url, KeyName: "X-API-KEY", Key: "secret"})
	v.params.Retry.Delay = time.Millisecond

	return v
}

// verifierServer serves a fixed JSON body and counts requests.
func verifierServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv, calls
}

func TestVerifierResponseWire(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// assert, not require: FailNow must not run outside the test goroutine
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "secret", r.Header.Get("X-API-KEY"))

		var got map[string]string
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, map[string]string{
			"attestationType": common.Hash(req.Header.AttestationType).Hex(),
			"sourceId":        common.Hash(req.Header.SourceId).Hex(),
			"requestBody":     hexutil.Encode(req.RequestBody),
		}, got)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"VERIFIED","responseBody":"0x0102"}`)
	}))
	t.Cleanup(srv.Close)

	res, err := testVerifier(srv.URL).Response(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, VerifierResponse{Status: StatusVerified, ResponseBody: hexutil.Bytes{0x01, 0x02}}, res)
}

func TestVerifierResponseStatuses(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	tests := []struct {
		name          string
		body          string
		wantErr       error
		wantDecodeErr bool // malformed JSON ahead of validate(): no sentinel to match against
		want          VerifierResponse
	}{
		{
			name: "rejected keeps message",
			body: `{"status":"REJECTED","message":"unsupported source"}`,
			want: VerifierResponse{Status: StatusRejected, Message: "unsupported source"},
		},
		{
			name: "retry keeps message",
			body: `{"status":"RETRY","message":"round not finalized"}`,
			want: VerifierResponse{Status: StatusRetry, Message: "round not finalized"},
		},
		{
			name: "spurious body on rejected is not an error",
			body: `{"status":"REJECTED","responseBody":"0x01","message":"no"}`,
			want: VerifierResponse{Status: StatusRejected, ResponseBody: hexutil.Bytes{0x01}, Message: "no"},
		},
		{
			name: "spurious message on verified is not an error",
			body: `{"status":"VERIFIED","responseBody":"0x01","message":"note"}`,
			want: VerifierResponse{Status: StatusVerified, ResponseBody: hexutil.Bytes{0x01}, Message: "note"},
		},
		{
			name: "null body on rejected decodes",
			body: `{"status":"REJECTED","responseBody":null,"message":"bad request"}`,
			want: VerifierResponse{Status: StatusRejected, Message: "bad request"},
		},
		{
			name: "null body on retry decodes",
			body: `{"status":"RETRY","responseBody":null,"message":"not final"}`,
			want: VerifierResponse{Status: StatusRetry, Message: "not final"},
		},
		{
			name:    "verified with empty body",
			body:    `{"status":"VERIFIED","responseBody":"0x"}`,
			wantErr: ErrEmptyResponseBody,
		},
		{
			name:    "verified with null body",
			body:    `{"status":"VERIFIED","responseBody":null}`,
			wantErr: ErrEmptyResponseBody,
		},
		{
			name:    "verified with oversized body",
			body:    `{"status":"VERIFIED","responseBody":"0x` + strings.Repeat("00", maxResponseBodyLen+1) + `"}`,
			wantErr: ErrResponseBodyTooBig,
		},
		{
			name: "verified at the max body size boundary",
			body: `{"status":"VERIFIED","responseBody":"0x` + strings.Repeat("00", maxResponseBodyLen) + `"}`,
			want: VerifierResponse{Status: StatusVerified, ResponseBody: make(hexutil.Bytes, maxResponseBodyLen)},
		},
		{
			name:          "malformed response body is a decode error",
			body:          `{"status":"VERIFIED","responseBody":"0xZZ"}`,
			wantDecodeErr: true,
		},
		{
			name:    "verified without body",
			body:    `{"status":"VERIFIED"}`,
			wantErr: ErrEmptyResponseBody,
		},
		{
			name:    "unknown status",
			body:    `{"status":"MAYBE","message":"?"}`,
			wantErr: ErrUnknownStatus,
		},
		{
			name:    "missing status",
			body:    `{"responseBody":"0x01"}`,
			wantErr: ErrUnknownStatus,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, calls := verifierServer(t, tc.body)

			res, err := testVerifier(srv.URL).Response(context.Background(), req)
			require.EqualValues(t, 1, calls.Load()) // contract violations must not retry
			if tc.wantDecodeErr {
				require.ErrorContains(t, err, "decoding response")
				return
			}
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, res)
		})
	}
}

// TestVerifierResponseHTTPRetry smoke-tests the post.WithRetry wiring;
// the full status matrix is covered in internal/post.
func TestVerifierResponseHTTPRetry(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	calls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"VERIFIED","responseBody":"0x01"}`)
	}))
	t.Cleanup(srv.Close)

	res, err := testVerifier(srv.URL).Response(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, StatusVerified, res.Status)
	require.EqualValues(t, 2, calls.Load())
}

func TestVerifierResponseNoHTTPRetry(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	calls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	_, err := testVerifier(srv.URL).Response(context.Background(), req)
	require.ErrorContains(t, err, strconv.Itoa(http.StatusBadRequest))
	require.EqualValues(t, 1, calls.Load())
}

func TestVerifierResponseMessageTruncated(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	long := strings.Repeat("m", 2*maxMessageLen)
	srv, _ := verifierServer(t, `{"status":"RETRY","message":"`+long+`"}`)

	res, err := testVerifier(srv.URL).Response(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, res.Message, maxMessageLen+len("..."))
	require.True(t, strings.HasSuffix(res.Message, "..."))
}

func TestVerifierResponseStatusTruncated(t *testing.T) {
	t.Parallel()
	req, _ := fdcRequest()

	long := "X" + strings.Repeat("s", 2*maxStatusLen) // leading X anchors the cut position
	srv, _ := verifierServer(t, `{"status":"`+long+`","message":"?"}`)

	_, err := testVerifier(srv.URL).Response(context.Background(), req)
	require.ErrorIs(t, err, ErrUnknownStatus)
	require.ErrorContains(t, err, "X"+strings.Repeat("s", maxStatusLen-1)+"...")
	require.NotContains(t, err.Error(), long)
}
