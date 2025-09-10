package client

import (
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

func New(cfg config.Config) *client.Client {
	return client.New(cfg)
}
