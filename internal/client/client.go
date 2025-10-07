package client

import (
	"context"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/internal/collector"
	"github.com/flare-foundation/tee-relay-client/internal/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/sender"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

type Client struct {
	collector *collector.Collector
	router    *instructions.Router
}

// Run starts collector, instruction processing, and sender.
func (c Client) Run(ctx context.Context) {
	cToR := make(chan []database.Log, 50)
	rToS := make(chan *instructions.Base, 50)

	collector.Run(ctx, c.collector, cToR)
	instructions.Run(ctx, c.router, cToR, rToS)
	sender.Run(ctx, rToS)
}

// New creates new Client from configs.
func New(cfg config.Config) *Client {
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		logger.Panic("Could not connect to database:", err)
	}

	sgnr, err := setSigner(&cfg.Signer)
	if err != nil {
		logger.Panic("Could not set signer:", err)
	}

	filterer, err := instructions.NewFilterer(cfg.IsCosigner, sgnr)
	if err != nil {
		logger.Panic("Could not set filterer:", err)
	}

	c := collector.New(db, cfg.TeeExtensionRegistry)
	r := instructions.NewRouter(sgnr, &cfg.FTDC, filterer)

	return &Client{
		collector: c,
		router:    r,
	}
}

func setSigner(cfg *config.Signer) (signer.Signer, error) {
	if cfg.Local {
		priv, err := config.PrivateKeyFromEnv(cfg.PrivateKeyVariable)
		if err != nil {
			return nil, err
		}

		return signer.NewLocal(priv), nil
	} else {
		if err := cfg.Check(); err != nil {
			return nil, err
		}

		return signer.Remote{
			Credentials: &cfg.Credentials,
		}, nil
	}
}
