package client

import (
	"context"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/tee-relay-client/client/collector"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/client/sender"
)

type Client struct {
	collector collector.Collector
	router    instructions.Router
	sender    sender.Sender
}

func (c Client) Run(ctx context.Context) {

	cToR := make(chan []database.Log, 50) //todo buffer
	rToS := make(chan *instructions.InstructionBase, 50)

	c.collector.Run(ctx, cToR)
	c.router.Run(ctx, cToR, rToS)
	c.sender.Run(ctx, rToS)

}
