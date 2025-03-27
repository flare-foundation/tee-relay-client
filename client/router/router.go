package router

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
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

type Credentials struct {
	KeyName string `toml:"key_name"`
	Key     string `toml:"key"`
	URL     string `toml:"url"`
}

func (c Credentials) ApiKey() call.APIKey {
	return call.APIKey{
		Name: c.KeyName,
		Key:  c.Key,
	}
}

// Router
type Router struct {
	signer Signer
	xrp    augmenterWallet
	btc    augmenterWallet
	// tdc    augmenter
}

// New creates new Router from config
func New(signerCred, xrpCred, btcCred *Credentials) *Router {
	return &Router{
		signer: Signer{signerCred},
		xrp:    augmenterWallet{xrpCred},
		btc:    augmenterWallet{btcCred},
	}
}

// Sign gets signatures of hashes from signer.
func (r Router) Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	return r.signer.FetchSignatures(ctx, hashes)
}

// Augment gets additional fixed and additional variable message
func (r Router) Augment(ctx context.Context, opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	switch opCommand {
	case xrpOP:
		return r.xrp.Augment(ctx, opType, opCommand, message)
	case btcOP:
		return r.btc.Augment(ctx, opType, opCommand, message)
	default:
		return nil, nil, fmt.Errorf("invalid augmentation OPCommand %v", string(opCommand[:]))
	}
}
