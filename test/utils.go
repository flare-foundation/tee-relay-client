package test

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/tee-relay-client/client/config"
)

func NewTestSigner(cfg signing.Config, prv *ecdsa.PrivateKey) (*signing.Signer, *config.Credentials) {
	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}

	url := fmt.Sprintf("http://localhost%s/sign", cfg.Addr)

	cred := config.Credentials{
		APIKeyName: cfg.APIKeyName,
		APIKey:     apiKey,
		URL:        url,
	}
	return signing.New(cfg, prv), &cred
}
