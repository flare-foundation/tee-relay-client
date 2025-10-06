package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/flare-foundation/tee-node/pkg/types"
)

type Local struct {
	priv *ecdsa.PrivateKey
}

// Sign computes textHash of each hash and returns slice of ecdsa signatures.
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
	privKeyDecryption := ecies.ImportECDSA(l.priv)
	plainText, err := privKeyDecryption.Decrypt(cipher, nil, nil)
	if err != nil {
		return nil, err
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
