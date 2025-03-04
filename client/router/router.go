package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Router interface {
	Sign([]common.Hash) ([][]byte, error)
	Augment(common.Hash, common.Hash, hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error)
}

type apiKey struct {
	name string
	key  string
}

// move to common

type Request struct {
	Hashes []common.Hash `json:"hashes"`
}
type Response struct {
	Signatures []hexutil.Bytes `json:"signatures"`
}

type Signer struct {
	key apiKey
	url string
}

func (s Signer) FetchSignatures(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := Request{hashes}
	encodedBody, err := json.Marshal(req)

	if err != nil {
		return nil, err
	}

	response := Response{}

	err = SendPost(ctx, s.url, s.key, encodedBody, &response)
	if err != nil {
		return nil, err
	}

	if len(hashes) != len(response.Signatures) {
		return nil, fmt.Errorf("wrong number of signatures, requested %d, got %d", len(hashes), len(response.Signatures))
	}

	return response.Signatures, nil
}
