package router

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/utils"
)

var xrpOP common.Hash
var btcOP common.Hash

func init() {
	const xrp = "XRP"
	const btc = "BTC"
	var err error

	xrpOP, err = utils.ToBytes32(xrp)
	if err != nil {
		logger.Panicf("invalid xrp OPType: %v", err)
	}

	btcOP, err = utils.ToBytes32(btc)
	if err != nil {
		logger.Panicf("invalid btc OPType: %v", err)
	}
}

// move to common

type Request struct {
	Hashes []common.Hash `json:"hashes"`
}
type Response struct {
	Signatures []hexutil.Bytes `json:"signatures"`
}

type credentials struct {
	key utils.APIKey
	url string
}

func Pack(cfgCreds config.Credentials) credentials {
	return credentials{
		key: utils.NewApiKey(cfgCreds.APIKeyName, cfgCreds.APIKey),
		url: cfgCreds.URL,
	}
}

type signer credentials

func (s signer) FetchSignatures(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	req := Request{hashes}
	encodedBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	response, err := utils.PostWithRetry[Response](ctx, s.url, s.key, encodedBody, utils.RetryParams{
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

// implements Router interface
type Router struct {
	signer signer
	xrp    augmenter
	btc    augmenter
}

func New(signerCred, xrpCred, btcCred config.Credentials) Router {
	return Router{
		signer: signer(Pack(signerCred)),
		xrp:    augmenter(Pack(xrpCred)),
		btc:    augmenter(Pack(btcCred)),
	}
}

func (r Router) Sign(hashes []common.Hash) ([]hexutil.Bytes, error) {
	return r.signer.FetchSignatures(context.TODO(), hashes)
}

func (r Router) Augment(opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	ctx := context.TODO()

	switch opCommand {
	case xrpOP:
		return r.xrp.Augment(ctx, opType, opCommand, message)
	case btcOP:
		return r.btc.Augment(ctx, opType, opCommand, message)
	default:
		return nil, nil, fmt.Errorf("invalid augmentation OPCommand %v", string(opCommand[:]))
	}
}
