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
	"github.com/flare-foundation/go-flare-common/pkg/safeurl"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const bytesPerSignature = 140   // TODO: make this more restrictive?
// const maxRespSize = 1 << 20     // 1 MB for maximal response size of the server

var safeTransport = safeurl.NewTransport()

// Signer holds credentials for the signer server.
type Remote struct{ *config.Credentials }

var _ Signer = &Remote{}

// Sign sends hashes to signer and returns the corresponding signatures.
func (r *Remote) Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := signer.SignBody{Hashes: hashes}

	response, err := call.PostWithRetry[signer.SignBody, signer.SignedBody](ctx, r.URL+"/sign", r.APIKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: 50 + int64(bytesPerSignature*len(hashes)),
		Transport:       safeTransport,
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     10 * time.Second,
		})
	if err != nil {
		return nil, fmt.Errorf("post call to %v rejected: %w", r.URL, err)
	}

	if len(hashes) != len(response.Message.Signatures) {
		return nil, fmt.Errorf("wrong number of signatures, requested %d, got %d", len(hashes), len(response.Message.Signatures))
	}

	return response.Message.Signatures, nil
}

// Decrypt sends cipher to signer for decryption.
func (r *Remote) Decrypt(ctx context.Context, cipher []byte) (hexutil.Bytes, error) {
	req := signer.EncryptedBody{Cipher: cipher}

	response, err := call.PostWithRetry[signer.EncryptedBody, signer.DecryptedBody](ctx, r.URL+"/decrypt", r.APIKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: int64(10 * len(cipher)),
		Transport:       safeTransport,
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     10 * time.Second,
		})
	if err != nil {
		return nil, fmt.Errorf("post call to %v rejected: %w", r.URL, err)
	}

	return response.Message.Plain, nil
}

func idCallFactory(client *http.Client, req *http.Request) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		resp, err := client.Do(req)

		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unsuccessful status code %d", resp.StatusCode)
		}

		return resp.Body, nil
	}
}

// Identify retrieves identity of the signer.
func (r *Remote) Identify(ctx context.Context) (types.PublicKey, error) {
	client := safeurl.NewClient(10 * time.Second)

	pk := types.PublicKey{}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+"/id", nil)
	if err != nil {
		return pk, fmt.Errorf("creating identify request: %w", err)
	}
	request.Header.Set(r.KeyName, r.Key)
	request.Header.Set("Content-Type", "application/json")

	re := retry.Execute(ctx, idCallFactory(client, request), retry.Params{
		MaxAttempts: 3,
		Delay:       5 * time.Second,
		Timeout:     20 * time.Second,
	})
	if !re.Success {
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

	return pk, nil
}
