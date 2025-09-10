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
func NewTestSigner(cfg signer.Config, prv *ecdsa.PrivateKey) (*signer.Signer, *config.Credentials) {
	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}

	url := fmt.Sprintf("http://localhost%s/sign", cfg.Addr)

	cred := config.Credentials{
		KeyName: cfg.APIKeyName,
		Key:     apiKey,
		URL:     url,
	}
	return signer.New(cfg, prv), &cred
}

var NilCred = &config.Credentials{
	KeyName: "",
	Key:     "",
	URL:     "",
}
