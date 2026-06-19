package signer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/stretchr/testify/require"
)

// jsonServer serves a fixed JSON body for the given path and 404 elsewhere.
func jsonServer(path, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
}

func remoteFor(url string) *Remote {
	return NewRemote(&config.Credentials{URL: url, KeyName: "X-API-KEY", Key: "k"})
}

func TestRemoteSignCountMismatch(t *testing.T) {
	t.Parallel()
	srv := jsonServer("/sign", `{"signatures":[]}`) // zero signatures for one hash
	defer srv.Close()

	_, err := remoteFor(srv.URL).Sign(context.Background(), []common.Hash{common.HexToHash("0x1")})
	require.ErrorContains(t, err, "wrong number of signatures")
}

func TestRemoteIdentifyUnknownField(t *testing.T) {
	t.Parallel()
	body := `{"x":"0x0000000000000000000000000000000000000000000000000000000000000001",` +
		`"y":"0x0000000000000000000000000000000000000000000000000000000000000002","z":1}`
	srv := jsonServer("/id", body)
	defer srv.Close()

	_, err := remoteFor(srv.URL).Identify(context.Background())
	require.ErrorContains(t, err, "decoding identify response")
}

func TestRemoteDecrypt(t *testing.T) {
	t.Parallel()
	srv := jsonServer("/decrypt", `{"plain":"0x1234"}`)
	defer srv.Close()

	plain, err := remoteFor(srv.URL).Decrypt(context.Background(), []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	require.NoError(t, err)
	require.Equal(t, []byte{0x12, 0x34}, []byte(plain))
}
