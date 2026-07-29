// Package testutils provides signer helpers for tests, including tests in other
// repositories — which is why it sits under pkg/ rather than internal/.
//
// DO NOT USE IN PRODUCTION.
package testutils

import (
	"crypto/ecdsa"
	"fmt"
	"net"

	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// NewTestSigner creates a signer server on a free port that can be used in simulation.
//
// DO NOT USE IN PRODUCTION.
func NewTestSigner(prv *ecdsa.PrivateKey) (*signer.Signer, *config.Credentials, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(fmt.Sprintf("failed to find free port: %v", err))
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		panic("listener address is not TCP")
	}
	port := addr.Port
	l.Close() //nolint:errcheck // best-effort close of ephemeral listener

	cfg := signer.Config{
		Addr:       fmt.Sprintf(":%d", port),
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{"testkey"},
	}

	cred := config.Credentials{
		KeyName: cfg.APIKeyName,
		Key:     cfg.APIKeys[0],
		URL:     fmt.Sprintf("http://localhost:%d", port),
	}

	cfg.Addr = fmt.Sprintf("127.0.0.1%s", cfg.Addr)

	s, err := signer.New(cfg, prv)
	if err != nil {
		return nil, nil, fmt.Errorf("creating new signer %w", err)
	}

	return s, &cred, nil
}

// NilCred is a Credentials with empty fields.
var NilCred = &config.Credentials{
	KeyName: "",
	Key:     "",
	URL:     "",
}
