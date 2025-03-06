package main

import (
	"context"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/client"
	"github.com/flare-foundation/tee-relay-client/client/config"
)

const (
	configPath string = "config.toml" // relative to project root
)

func main() {
	cfg, err := config.ReadConfigs(configPath)
	if err != nil {
		logger.Panicf("cannot read configs: %s", err)
	}

	logger.Set(cfg.Logging)

	ctx := context.Background()

	cl := client.New(*cfg)

	cl.Run(ctx)
}
