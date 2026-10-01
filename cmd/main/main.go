package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/internal/health"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const allowUnsafeURLsEnv = "ALLOW_UNSAFE_URLS"

const (
	configPath string = "config.toml" // relative to project root
)

// healthShutdownTimeout bounds the wait for in-flight probes at shutdown.
const healthShutdownTimeout = 2 * time.Second

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
	if err := cfg.CheckHealth(); err != nil {
		return cfg, fmt.Errorf("checking health: %w", err)
	}

	if v, ok := os.LookupEnv(allowUnsafeURLsEnv); ok && v == "true" {
		cfg.AllowUnsafeURLs = true
	}

	return cfg, nil
}

// healthWarning returns the startup warning for a [health] section that disables the server
// or moves it off the default port, or "" when there is none.
func healthWarning(h config.Health) string {
	if h.Disabled {
		return "health endpoints disabled by [health] disabled = true: health probes get connection refused"
	}
	if h.Port != config.DefaultHealthPort {
		return fmt.Sprintf("health port %d is not the default %d, which the provided Dockerfile EXPOSEs: probes and port mappings must use %d", h.Port, config.DefaultHealthPort, h.Port)
	}

	return ""
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
	if w := healthWarning(cfg.Health); w != "" {
		logger.Warn(w)
	}

	// Bound before the client so the probes answer through the DB connect and the indexer
	// sync wait; a busy port fails startup here instead of after them.
	var status *health.Status
	var hs *health.Server
	if !cfg.Health.Disabled {
		status = health.NewStatus(cfg.Health.MaxIndexerLag)
		hs, err = health.Listen(cfg.Health.Address(), status)
		if err != nil {
			logger.Panicf("starting health server: %v", err)
		}
		hs.Start()
		logger.Infof("health endpoints listening on %s", hs.Addr())
	}

	cl, err := client.New(cfg.Config, status)
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

	// Probes are refused while the pipeline drains — accurate, and nothing is being routed.
	if hs != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), healthShutdownTimeout)
		if err := hs.Close(shutdownCtx); err != nil {
			logger.Warnf("closing health server: %v", err)
		}
		shutdownCancel()
	}

	// Wait for the pipeline goroutines to log their "closing" lines, then flush the file core.
	cl.Wait()
	if cfg.Logging.File != "" {
		logger.SyncFileLogger()
	}
}
