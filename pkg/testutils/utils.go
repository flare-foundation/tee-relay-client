package testutils

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// NewTestSigner creates a signer server that can be used in simulation.
//
// DO NOT USE IN PRODUCTION.
func NewTestSigner(cfg signer.Config, prv *ecdsa.PrivateKey) (*signer.Signer, *config.Credentials, error) {
	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}

	url := fmt.Sprintf("http://localhost%s", cfg.Addr)

	cred := config.Credentials{
		KeyName: cfg.APIKeyName,
		Key:     apiKey,
		URL:     url,
	}

	cfg.Addr = fmt.Sprintf("127.0.0.1%s", cfg.Addr)

	s, err := signer.New(cfg, prv)
	if err != nil {
		return nil, nil, fmt.Errorf("creating new signer %w", err)
	}

	return s, &cred, nil
}

var NilCred = &config.Credentials{
	KeyName: "",
	Key:     "",
	URL:     "",
}
