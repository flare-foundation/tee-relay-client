package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/tee-relay-client/client"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/test"
)

const (
	configPath string = "config.toml" // relative to project root
)

func main() {
	cfg, err := config.ReadConfigs(configPath)
	if err != nil {
		logger.Panicf("cannot read configs: %s", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.TestPrivateKey != nil {
		logger.Warn(`test private key used: DO NOT USE IN PRODUCTION
		\n
		for production remove test_private_key from config.toml
		`)

		x := common.Hash{}
		_, err := rand.Read(x[:])
		if err != nil {
			logger.Panicf("can not create api key: %v", err)
		}
		key := x.Hex()

		scfg := signing.Config{
			Addr:       ":8791",
			APIKeyName: "X-API-KEY",
			APIKeys:    []string{key},
		}

		prv, err := crypto.ToECDSA(cfg.TestPrivateKey.Bytes())
		if err != nil {
			logger.Panicf("can not create private key from TestPrivateKey : %v", err)
		}

		signer, credentials := test.NewTestSigner(scfg, prv)
		cfg.Signer = *credentials

		go func() {
			err := signer.Run(ctx)
			logger.Warnf("signer exited: %v", err)
		}()
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	logger.Set(cfg.Logging)

	cl := client.New(*cfg)
	cl.Run(ctx)

	go func() {
		select {
		case sig := <-signalChan:
			logger.Infof("Received %v signal, shutting down", sig)
		case <-ctx.Done():
			logger.Infof("Context canceled %v signal, shutting down", ctx.Err())
		}
		cancel()
	}()
}
