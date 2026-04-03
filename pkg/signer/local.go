package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-node/pkg/types"
)

type Local struct {
	priv *ecdsa.PrivateKey
}

var _ Signer = &Local{}

// Sign computes textHash of each hash and returns a slice of ecdsa signatures.
func (l *Local) Sign(_ context.Context, hashes []common.Hash) ([]hexutil.Bytes, error) {
	signatures := make([]hexutil.Bytes, len(hashes))

	for i, hash := range hashes {
		sig, err := crypto.Sign(accounts.TextHash(hash.Bytes()), l.priv)
		if err != nil {
			return nil, fmt.Errorf("signing hash: %w", err)
		}
		signatures[i] = sig
	}

	return signatures, nil
}

// Decrypt decrypts the cipher.
func (l *Local) Decrypt(_ context.Context, cipher []byte) (hexutil.Bytes, error) {
	privKeyDecryption, err := signer.ECDSAPrivKeyToECIES(l.priv)
	if err != nil {
		return nil, fmt.Errorf("converting private key to ECIES: %w", err)
	}
	plainText, err := privKeyDecryption.Decrypt(cipher, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plainText, nil
}

// Identify retrieves identity of the signer.
func (l *Local) Identify(_ context.Context) (types.PublicKey, error) {
	pk := types.PubKeyToStruct(&l.priv.PublicKey)
	return pk, nil
}

// NewLocal creates a new LocalSigner with the given private key.
func NewLocal(privateKey *ecdsa.PrivateKey) *Local {
	return &Local{
		priv: privateKey,
	}
}
