package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const allowUnsafeURLsEnv = "ALLOW_UNSAFE_URLS"

const (
	configPath string = "config.toml" // relative to project root
)

// loadConfig reads and validates the relay configuration from path and applies
// the ALLOW_UNSAFE_URLS environment override.
func loadConfig(path string) (config.Config, error) {
	cfg := config.Default()
	if err := toml.ReadTo(path, &cfg, true); err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	if err := cfg.CheckAddress(); err != nil {
		return cfg, fmt.Errorf("checking address: %w", err)
	}
	if err := cfg.CheckChainID(); err != nil {
		return cfg, fmt.Errorf("checking chain id: %w", err)
	}

	if v, ok := os.LookupEnv(allowUnsafeURLsEnv); ok && v == "true" {
		cfg.AllowUnsafeURLs = true
	}

	return cfg, nil
}

func main() {
	cfg, err := loadConfig(configPath)
	if err != nil {
		logger.Panicf("loading config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	logger.Set(cfg.Logging)

	logger.Infof("tee-relay starting: chain %d, FlareTeeManager %s, cosigner %t", cfg.ChainID, cfg.FlareTeeManager, cfg.IsCosigner)

	if cfg.AllowUnsafeURLs {
		logger.Warnf("SSRF protection is disabled via %s — do not use in production", allowUnsafeURLsEnv)
	}

	cl, err := client.New(cfg)
	if err != nil {
		logger.Panicf("creating client: %v", err)
	}

	err = cl.Run(ctx)
	if err != nil {
		logger.Panicf("running client: %v", err)
	}

	sig := <-signalChan
	logger.Infof("received %v signal, shutting down", sig)

	cancel()

	// Wait for the pipeline goroutines to log their "closing" lines, then flush the file core.
	cl.Wait()
	if cfg.Logging.File != "" {
		logger.SyncFileLogger()
	}
}
