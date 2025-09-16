package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const (
	configPath string = "config.toml" // relative to project root
)

func main() {
	cfg, err := toml.Read[config.Config](configPath, true)
	if err != nil {
		logger.Panicf("cannot read configs: %s", err)
	}
	if err := cfg.CheckAddress(); err != nil {
		logger.Panicf("checking address %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	logger.Set(cfg.Logging)

	cl := client.New(cfg)
	cl.Run(ctx)

	select {
	case sig := <-signalChan:
		logger.Infof("Received %v signal, shutting down", sig)
	case <-ctx.Done():
		logger.Infof("Context canceled %v signal, shutting down", ctx.Err())
	}
	cancel()
}
