package client

import (
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// New creates a new Client from cfg.
func New(cfg config.Config) (*client.Client, error) {
	return client.New(cfg)
}
