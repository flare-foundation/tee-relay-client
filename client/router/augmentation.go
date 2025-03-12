package router

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-relay-client/utils"
)

type augmenterWallet Credentials

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

	encodedBody, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}

	response, err := utils.PostWithRetry[AugResponse](ctx, a.url, a.key, encodedBody, utils.RetryParams{
		MaxAttempts: 3,
		Delay:       10 * time.Second,
		Timeout:     time.Minute,
	})
	if err != nil {
		return nil, nil, err
	}

	return response.FixedMessage, response.VariableMessage, nil
}
