// Package signer provides local and remote implementations for signing, decryption and identification.
package signer

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-node/pkg/types"
)

// Signer signs hashes, decrypts ciphers and reports its identity.
type Signer interface {
	// Sign signs each hash.
	Sign(ctx context.Context, hashes []common.Hash) ([]hexutil.Bytes, error)

	// Decrypt decrypts the cipher.
	Decrypt(ctx context.Context, cipher []byte) (hexutil.Bytes, error)

	// Identify retrieves identity of the signer.
	Identify(ctx context.Context) (types.PublicKey, error)
}

// Logger is the logging interface injected into signers. logger.Nop and *zap.SugaredLogger satisfy it.
type Logger interface {
	Debugf(string, ...any)
	Infof(string, ...any)
	Warnf(string, ...any)
	Errorf(string, ...any)
}

// nopLogger is the silent default used when no logger is injected.
type nopLogger struct{}

var _ Logger = nopLogger{}

func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}
