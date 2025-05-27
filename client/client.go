package client

import (
	"context"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/tee-relay-client/client/collector"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/client/sender"
)

type Client struct {
	collector *collector.Collector
	router    *instructions.Router
}

// Run starts collector, instruction processing, and sender.
func (c Client) Run(ctx context.Context) {
	cToR := make(chan []database.Log, 50) //todo buffer
	rToS := make(chan *instructions.Base, 50)

	collector.Run(ctx, c.collector, cToR)
	instructions.Run(ctx, c.router, cToR, rToS)
	sender.Run(ctx, rToS)
}

// New creates new Client from configs.
func New(cfg config.Config) Client {
	c := collector.New(&cfg.DB, cfg.TeeInstructions)
	r := instructions.NewRouter(&cfg.Signer, &cfg.FTDC)

	return Client{
		collector: c,
		router:    r,
	}
}
