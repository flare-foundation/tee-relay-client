// Package client exposes the relay client constructor for use in tests of other
// repositories. It is not a supported public API: New returns a type from
// internal/, which callers outside this module cannot name. Production builds use
// cmd/main, which calls internal/client directly.
package client

import (
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// New creates a new Client from cfg.
func New(cfg config.Config) (*client.Client, error) {
	return client.New(cfg)
}
