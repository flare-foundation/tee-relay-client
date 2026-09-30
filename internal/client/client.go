// Package client wires together the collector, router, and sender.
package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/internal/collector"
	"github.com/flare-foundation/tee-relay-client/internal/router"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/sender"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Client collects logs, routes them, and sends the results.
type Client struct {
	collector       *collector.Collector
	router          *router.Router
	allowUnsafeURLs bool
	wg              sync.WaitGroup
}

// New creates new Client from configs.
func New(cfg config.Config) (*Client, error) {
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}
	logger.Infof("connected to indexer database")

	return newWithDB(cfg, db)
}

// newWithDB assembles the Client around an already-open database handle.
func newWithDB(cfg config.Config, db *gorm.DB) (*Client, error) {
	sgnr, err := setSigner(&cfg.Signer)
	if err != nil {
		return nil, fmt.Errorf("could not set signer: %w", err)
	}

	filterer, err := router.NewFilterer(cfg.IsCosigner, sgnr)
	if err != nil {
		return nil, fmt.Errorf("could not set filterer: %w", err)
	}

	if cf, ok := filterer.(*router.CosignerFilterer); ok {
		logger.Infof("running as cosigner %s", cf.Address)
	} else {
		logger.Infof("running as data provider")
	}

	c := collector.New(db, cfg.FlareTeeManager, cfg.Collector.StartInterval)
	r, err := router.NewRouter(sgnr, cfg.ChainID, &cfg.FDC, filterer, cfg.AllowUnsafeURLs)
	if err != nil {
		return nil, fmt.Errorf("could not create router: %w", err)
	}

	return &Client{
		collector:       c,
		router:          r,
		allowUnsafeURLs: cfg.AllowUnsafeURLs,
	}, nil
}

// Run starts collector, instruction processing, and sender.
func (c *Client) Run(ctx context.Context) error {
	cToR := make(chan []database.Log, 50)
	rToS := make(chan *instructions.Base, 50)

	err := c.collector.Run(ctx, &c.wg, cToR)
	if err != nil {
		return fmt.Errorf("starting collector: %w", err)
	}

	c.router.Run(ctx, &c.wg, cToR, rToS)
	sender.Run(ctx, &c.wg, rToS, c.allowUnsafeURLs)

	logger.Infof("relay pipeline started")

	return nil
}

// Wait blocks until the pipeline goroutines exit; in-flight instruction handlers are not tracked.
func (c *Client) Wait() {
	c.wg.Wait()
}

func setSigner(cfg *config.Signer) (signer.Signer, error) {
	if cfg.Local {
		priv, err := config.PrivateKeyFromEnv(cfg.PrivateKeyVariable)
		if err != nil {
			return nil, fmt.Errorf("loading private key from env: %w", err)
		}

		logger.Infof("using local signer")

		return signer.NewLocal(priv), nil
	}

	if err := cfg.Check(); err != nil {
		return nil, fmt.Errorf("checking signer credentials: %w", err)
	}

	logger.Infof("using remote signer at %s", cfg.URL)

	// WithOptions corrects the global logger's caller skip for lines emitted inside the signer package.
	return signer.NewRemoteWithLogger(&cfg.Credentials, logger.Logger().WithOptions(zap.AddCallerSkip(-1))), nil
}
