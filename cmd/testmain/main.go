package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/flare-foundation/tee-relay-client/internal/client"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/testutils"
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
		logger.Panicf("checking address: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	logger.Set(cfg.Logging)

	pk, err := privateKeyFromEnv("PRIVATE_KEY")
	if err != nil {
		logger.Panicf("private key from env: %s", err)
	}

	apiKey, err := generateRandomHexString(32)
	if err != nil {
		logger.Panicf("getting random string: %s", err)
	}

	sCfg := signer.Config{
		Addr:       ":8080",
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{apiKey},
	}

	sigServer, cred := testutils.NewTestSigner(sCfg, pk)

	cfg.Signer.Credentials = *cred

	go sigServer.ListenAndServe() //nolint:errcheck

	cl := client.New(cfg)
	cl.Run(ctx)

	select {
	case sig := <-signalChan:
		logger.Infof("Received %v signal, shutting down", sig)
	case <-ctx.Done():
		logger.Infof("Context canceled %v signal, shutting down", ctx.Err())
	}

	sigServer.Close() //nolint:errcheck
	cancel()
}

func privateKeyFromEnv(variableName string) (*ecdsa.PrivateKey, error) {
	if len(variableName) == 0 {
		variableName = "PRIVATE_KEY"
	}
	pkStr := os.Getenv(variableName)

	if len(pkStr) == 0 {
		return nil, errors.New("private key not set")
	}

	pkStr, _ = strings.CutPrefix(pkStr, "0x")
	pkStr, _ = strings.CutPrefix(pkStr, "0X")

	pkB, err := hex.DecodeString(pkStr)
	if err != nil {
		return nil, errors.New("invalid string for private key")
	}

	return crypto.ToECDSA(pkB)
}

func generateRandomHexString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}
