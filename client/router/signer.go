package router

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/tee-relay-client/utils"
)

type Signer Credentials

func (s Signer) FetchSignatures(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := signing.RequestBody{Hashes: hashes}
	encodedBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	response, err := utils.PostWithRetry[signing.ResponseBody](ctx, s.url, s.key, encodedBody, utils.RetryParams{
		MaxAttempts: 3,
		Delay:       10 * time.Second,
		Timeout:     time.Minute,
	})
	if err != nil {
		return nil, err
	}

	if len(hashes) != len(response.Signatures) {
		return nil, fmt.Errorf("wrong number of signatures, requested %d, got %d", len(hashes), len(response.Signatures))
	}

	return response.Signatures, nil
}
