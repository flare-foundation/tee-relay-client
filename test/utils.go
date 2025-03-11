package test

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
)

type TestRouter struct {
	Prv *ecdsa.PrivateKey
}

func (tr TestRouter) Sign(hashes []common.Hash) ([]hexutil.Bytes, error) {
	out := make([]hexutil.Bytes, len(hashes))

	var err error

	for j := range hashes {
		out[j], err = instruction.SignInstructionHash(hashes[j], tr.Prv)
		if err != nil {
			return nil, err
		}
	}

	return out, err
}

func (tr TestRouter) Augment(opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	return hexutil.Bytes{}, hexutil.Bytes{}, nil
}
