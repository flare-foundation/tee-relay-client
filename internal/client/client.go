package client

import (
	"context"
	"fmt"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/tee-relay-client/internal/collector"
	"github.com/flare-foundation/tee-relay-client/internal/router"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/sender"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

type Client struct {
	collector *collector.Collector
	router    *router.Router
}

// New creates new Client from configs.
func New(cfg config.Config) (*Client, error) {
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	sgnr, err := setSigner(&cfg.Signer)
	if err != nil {
		return nil, fmt.Errorf("could not set signer: %w", err)
	}

	filterer, err := router.NewFilterer(cfg.IsCosigner, sgnr)
	if err != nil {
		return nil, fmt.Errorf("could not set filterer: %w", err)
	}

	c := collector.New(db, cfg.FlareTeeManager)
	r, err := router.NewRouter(sgnr, &cfg.FDC, filterer)
	if err != nil {
		return nil, fmt.Errorf("could not create router: %w", err)
	}

	return &Client{
		collector: c,
		router:    r,
	}, nil
}

// Run starts collector, instruction processing, and sender.
func (c *Client) Run(ctx context.Context) error {
	cToR := make(chan []database.Log, 50)
	rToS := make(chan *instructions.Base, 50)

	err := c.collector.Run(ctx, cToR)
	if err != nil {
		return fmt.Errorf("starting collector: %w", err)
	}

	c.router.Run(ctx, cToR, rToS)
	sender.Run(ctx, rToS)

	return nil
}

func setSigner(cfg *config.Signer) (signer.Signer, error) {
	if cfg.Local {
		priv, err := config.PrivateKeyFromEnv(cfg.PrivateKeyVariable)
		if err != nil {
			return nil, fmt.Errorf("loading private key from env: %w", err)
		}

		return signer.NewLocal(priv), nil
	} else {
		if err := cfg.Check(); err != nil {
			return nil, fmt.Errorf("checking signer credentials: %w", err)
		}

		return &signer.Remote{
			Credentials: &cfg.Credentials,
		}, nil
	}
}
