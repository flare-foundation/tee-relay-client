package test

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/tee-relay-client/client/router"
)

// NewTestSigner creates a signer server that can be used in simulation.
//
// DO NOT USE IN PRODUCTION
func NewTestSigner(cfg signing.Config, prv *ecdsa.PrivateKey) (*signing.Signer, *router.Credentials) {
	apiKey := ""
	if len(cfg.APIKeys) > 0 {
		apiKey = cfg.APIKeys[0]
	}

	url := fmt.Sprintf("http://localhost%s/sign", cfg.Addr)

	cred := router.Credentials{
		KeyName: cfg.APIKeyName,
		Key:     apiKey,
		URL:     url,
	}
	return signing.New(cfg, prv), &cred
}

var NilCred = &router.Credentials{
	KeyName: "",
	Key:     "",
	URL:     "",
}
