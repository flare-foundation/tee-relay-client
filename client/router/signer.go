package router

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
)

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const bytesPerSignature = 1000  // TODO: make this more restrictive
const maxRespSize = (1 << 20)   // 1 MB for maximal response size of the server

type Signer struct{ *Credentials }

func (s Signer) FetchSignatures(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := signing.RequestBody{Hashes: hashes}

	response, err := call.PostWithRetry[signing.RequestBody, signing.ResponseBody](ctx, s.URL, s.ApiKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: int64(bytesPerSignature * len(hashes)),
	}, retry.Params{
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
