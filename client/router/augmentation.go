package router

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
)

type augmenterWallet struct{ *Credentials }

type AugRequest struct {
	OPType    common.Hash   `json:"opType"`
	OPCommand common.Hash   `json:"opCommand"`
	Message   hexutil.Bytes `json:"message"`
}

type AugResponse struct {
	FixedMessage    hexutil.Bytes `json:"fixedMessage"`
	VariableMessage hexutil.Bytes `json:"variableMessage"`
}

func (a augmenterWallet) Augment(ctx context.Context, opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	req := AugRequest{
		OPType:    opType,
		OPCommand: opCommand,
		Message:   message,
	}

	response, err := call.PostWithRetry[AugRequest, AugResponse](ctx, a.URL, a.ApiKey(), req, call.Params{
		Timeout:         timeout,
		MaxResponseSize: maxRespSize,
	}, retry.Params{
		MaxAttempts: 3,
		Delay:       10 * time.Second,
		Timeout:     time.Minute,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed on retries: %v", err)
	}

	return response.FixedMessage, response.VariableMessage, nil
}
