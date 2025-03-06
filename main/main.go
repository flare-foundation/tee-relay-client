package main

import (
	"github.com/flare-foundation/go-flare-common/pkg/logger"
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

}
