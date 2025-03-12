package router

import (
	"context"
	"fmt"

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

type Credentials struct {
	key utils.APIKey
	url string
}

func Pack(cfgCreds *config.Credentials) Credentials {
	return Credentials{
		key: utils.NewApiKey(cfgCreds.APIKeyName, cfgCreds.APIKey),
		url: cfgCreds.URL,
	}
}

// Router implements instructions.Router interface
type Router struct {
	signer Signer
	xrp    augmenterWallet
	btc    augmenterWallet
	// tdc    augmenter
}

// New creates new Router from config
func New(signerCred, xrpCred, btcCred *config.Credentials) Router {
	return Router{
		signer: Signer(Pack(signerCred)),
		xrp:    augmenterWallet(Pack(xrpCred)),
		btc:    augmenterWallet(Pack(btcCred)),
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
