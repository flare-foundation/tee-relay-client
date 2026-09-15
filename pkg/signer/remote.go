package signer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const timeout = 5 * time.Second   // maximal duration for the server to resolve the query
const bytesPerSignature = 140     // TODO: make this more restrictive?
const errBodyDrainLimit = 4 << 10 // 4 KiB cap when draining an error response body for connection reuse
// const maxRespSize = 1 << 20     // 1 MB for maximal response size of the server

// Remote holds credentials for the signer server.
// The signer URL is operator-controlled config, so no SSRF protection is applied.
type Remote struct {
	*config.Credentials
	log Logger
}

var _ Signer = &Remote{}

// NewRemote creates a Remote signer.
func NewRemote(creds *config.Credentials) *Remote {
	return &Remote{Credentials: creds, log: nopLogger{}}
}

// NewRemoteWithLogger creates a Remote signer that logs through log.
func NewRemoteWithLogger(creds *config.Credentials, log Logger) *Remote {
	return &Remote{Credentials: creds, log: log}
}

// Sign sends hashes to signer and returns the corresponding signatures.
func (r *Remote) Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := signer.SignBody{Hashes: hashes}

	start := time.Now()
	response, err := call.PostWithRetry[signer.SignBody, signer.SignedBody](ctx, r.URL+"/sign", r.APIKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: 50 + int64(bytesPerSignature*len(hashes)),
	}, []int{},
		// Timeout covers the whole schedule (3 x 5s calls + 2 x 10s delays = 35s)
		// with slack; a shorter budget expires mid-delay and silently drops retries.
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     40 * time.Second,
		})
	if err != nil {
		r.log.Debugf("sign of %d hashes failed in %s", len(hashes), time.Since(start))
		return nil, fmt.Errorf("post call to %v rejected: %w", r.URL, err)
	}

	if len(hashes) != len(response.Message.Signatures) {
		return nil, fmt.Errorf("wrong number of signatures, requested %d, got %d", len(hashes), len(response.Message.Signatures))
	}

	r.log.Debugf("signed %d hashes in %s", len(hashes), time.Since(start))

	return response.Message.Signatures, nil
}

// Decrypt sends cipher to signer for decryption.
func (r *Remote) Decrypt(ctx context.Context, cipher []byte) (hexutil.Bytes, error) {
	req := signer.EncryptedBody{Cipher: cipher}

	start := time.Now()
	response, err := call.PostWithRetry[signer.EncryptedBody, signer.DecryptedBody](ctx, r.URL+"/decrypt", r.APIKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: int64(10 * (len(cipher) + 1)),
	}, []int{},
		// Timeout covers the whole schedule (3 x 5s calls + 2 x 10s delays = 35s)
		// with slack; a shorter budget expires mid-delay and silently drops retries.
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     40 * time.Second,
		})
	if err != nil {
		r.log.Debugf("decrypt of %d cipher bytes failed in %s", len(cipher), time.Since(start))
		return nil, fmt.Errorf("post call to %v rejected: %w", r.URL, err)
	}

	r.log.Debugf("decrypted %d cipher bytes in %s", len(cipher), time.Since(start))

	return response.Message.Plain, nil
}

func idCallFactory(client *http.Client, req *http.Request) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		resp, err := client.Do(req)

		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			// drain (bounded) and close so the connection can be reused on retry
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, errBodyDrainLimit))
			resp.Body.Close() //nolint:errcheck // closing response body, error is not actionable
			return nil, fmt.Errorf("unsuccessful status code %d", resp.StatusCode)
		}

		return resp.Body, nil
	}
}

// Identify retrieves identity of the signer.
func (r *Remote) Identify(ctx context.Context) (types.PublicKey, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	pk := types.PublicKey{}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+"/id", nil)
	if err != nil {
		return pk, fmt.Errorf("creating identify request: %w", err)
	}
	request.Header.Set(r.KeyName, r.Key)
	request.Header.Set("Content-Type", "application/json")

	start := time.Now()
	// Timeout covers the whole schedule (3 x 10s calls + 2 x 5s delays = 40s) with slack.
	re := retry.Execute(ctx, idCallFactory(client, request), retry.Params{
		MaxAttempts: 3,
		Delay:       5 * time.Second,
		Timeout:     45 * time.Second,
	})
	if !re.Success {
		r.log.Debugf("identify failed in %s", time.Since(start))
		return pk, re.Err
	}

	defer re.Value.Close() //nolint:errcheck // closing response body, error is not actionable

	respLimited := &io.LimitedReader{R: re.Value, N: 200}

	decoder := json.NewDecoder(respLimited)
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&pk)
	if err != nil {
		return pk, fmt.Errorf("decoding identify response: %w", err)
	}

	r.log.Debugf("identified signer in %s", time.Since(start))

	return pk, nil
}
