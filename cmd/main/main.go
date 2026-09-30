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

// fileConfig accepts the retired [relay_cutover] block so older configs still load.
type fileConfig struct {
	config.Config
	RelayCutover map[string]any `toml:"relay_cutover"` // non-nil iff the block is present
}

// loadConfig reads and validates the relay configuration from path and applies the
// FLARE_TEE_MANAGER_CONTRACT_ADDRESS and ALLOW_UNSAFE_URLS environment overrides.
func loadConfig(path string) (fileConfig, error) {
	cfg := fileConfig{Config: config.Default()}
	// Reject unknown keys: a misspelled is_cosigner would otherwise be discarded silently,
	// leaving a cosigner deployment processing every instruction it sees.
	if err := toml.ReadTo(path, &cfg, false); err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}
	// before CheckAddress — the env variable may be the only source of the address
	if err := cfg.ApplyFlareTeeManagerEnv(); err != nil {
		return cfg, fmt.Errorf("reading manager address from env: %w", err)
	}
	if err := cfg.CheckAddress(); err != nil {
		return cfg, fmt.Errorf("checking address: %w", err)
	}
	if err := cfg.CheckChainID(); err != nil {
		return cfg, fmt.Errorf("checking chain id: %w", err)
	}
	if err := cfg.CheckStartInterval(); err != nil {
		return cfg, fmt.Errorf("checking start interval: %w", err)
	}
	if err := cfg.CheckQueues(); err != nil {
		return cfg, fmt.Errorf("checking fdc queues: %w", err)
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
	if cfg.RelayCutover != nil {
		logger.Warnf("ignoring the retired [relay_cutover] block in %s: FDC2 responses are always signed with the chain-bound digest; remove it", configPath)
	}

	cl, err := client.New(cfg.Config)
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
