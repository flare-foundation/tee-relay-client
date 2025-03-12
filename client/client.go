package client

import (
	"context"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/tee-relay-client/client/collector"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/client/router"
	"github.com/flare-foundation/tee-relay-client/client/sender"
)

type Client struct {
	collector collector.Collector
	router    router.Router
	sender    sender.Sender
}

func (c Client) Run(ctx context.Context) {
	cToR := make(chan []database.Log, 50) //todo buffer
	rToS := make(chan *instructions.InstructionBase, 50)

	collector.Run(ctx, c.collector, cToR)
	go instructions.Run(ctx, c.router, cToR, rToS)
	go c.sender.Run(ctx, rToS)
}

func New(cfg config.Config) Client {
	c := collector.New(&cfg.DB, cfg.TeeInstructions)
	r := router.New(&cfg.Signer, &cfg.XRP, &cfg.BTC)
	s := sender.Sender{}

	return Client{
		collector: *c,
		router:    r,
		sender:    s,
	}
}
