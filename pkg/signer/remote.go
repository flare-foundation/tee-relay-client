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

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const bytesPerSignature = 140   // TODO: make this more restrictive?
// const maxRespSize = 1 << 20     // 1 MB for maximal response size of the server

// Signer holds credentials for the signer server.
type Remote struct{ *config.Credentials }

// Sign sends hashes to signer and returns the corresponding signatures.
func (r *Remote) Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := signer.SignBody{Hashes: hashes}

	response, err := call.PostWithRetry[signer.SignBody, signer.SignedBody](ctx, r.URL+"/sign", r.APIKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: 50 + int64(bytesPerSignature*len(hashes)),
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     10 * time.Second,
		})
	if err != nil {
		return nil, fmt.Errorf("post call to %v rejected %v", r.URL, err)
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
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     10 * time.Second,
		})
	if err != nil {
		return nil, fmt.Errorf("post call to %v rejected %v", r.URL, err)
	}

	return response.Message.Plain, nil
}

// Identify retrieves identity of the signer.
func (r *Remote) Identify(ctx context.Context) (types.PublicKey, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	pk := types.PublicKey{}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+"/id", nil)
	if err != nil {
		return pk, err
	}
	request.Header.Set(r.KeyName, r.Key)
	request.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(request)
	if err != nil {
		return pk, err
	}

	if resp.StatusCode != http.StatusOK {
		return pk, fmt.Errorf("unsuccessful status code %d", resp.StatusCode)
	}

	defer resp.Body.Close() //nolint:errcheck

	respLimited := &io.LimitedReader{R: resp.Body, N: 200}

	decoder := json.NewDecoder(respLimited)
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&pk)
	if err != nil {
		return pk, err
	}

	return pk, nil
}
