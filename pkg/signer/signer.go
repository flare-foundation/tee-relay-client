package signer

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-node/pkg/types"
)

type Signer interface {
	// Sign signs each hash.
	Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error)

	// Decrypt decrypts the cipher.
	Decrypt(ctx context.Context, cipher []byte) (hexutil.Bytes, error)

	// Identify retrieves identity of the signer.
	Identify(ctx context.Context) (types.PublicKey, error)
}
