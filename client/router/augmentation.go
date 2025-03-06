package router

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-relay-client/utils"
)

type augmenter credentials

type AugRequest struct {
	OPType    common.Hash   `json:"opType"`
	OPCommand common.Hash   `json:"opCommand"`
	Message   hexutil.Bytes `json:"message"`
}

type AugResponse struct {
	FixedMessage    hexutil.Bytes `json:"fixedMessage"`
	VariableMessage hexutil.Bytes `json:"variableMessage"`
}

func (a augmenter) Augment(ctx context.Context, opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	req := AugRequest{
		OPType:    opType,
		OPCommand: opCommand,
		Message:   message,
	}

	encodedBody, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}

	response := AugResponse{}

	err = utils.POST(ctx, a.url, a.key, encodedBody, &response)
	if err != nil {
		return nil, nil, err
	}

	return response.FixedMessage, response.VariableMessage, nil
}
