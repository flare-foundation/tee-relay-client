package router

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-relay-client/utils"
)

type Router interface {
	Sign([]common.Hash) ([][]byte, error)
	Augment(common.Hash, common.Hash, hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error)
}

// move to common

type Request struct {
	Hashes []common.Hash `json:"hashes"`
}
type Response struct {
	Signatures []hexutil.Bytes `json:"signatures"`
}

type SignerConfig struct {
	APIKeyName string `toml:"api_key_name"`
	APIKey     string `toml:"api_key"`
	URL        string `toml:"url"`
}

type Signer struct {
	key utils.APIKey
	url string
}

func (s Signer) FetchSignatures(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := Request{hashes}
	encodedBody, err := json.Marshal(req)

	if err != nil {
		return nil, err
	}

	response := Response{}

	err = utils.POST(ctx, s.url, s.key, encodedBody, &response)
	if err != nil {
		return nil, err
	}

	if len(hashes) != len(response.Signatures) {
		return nil, fmt.Errorf("wrong number of signatures, requested %d, got %d", len(hashes), len(response.Signatures))
	}

	return response.Signatures, nil
}
