package config

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/client/router"
)

type Config struct {
	DB              database.Config    `toml:"db"`
	Logging         logger.Config      `toml:"logger"`
	TeeInstructions common.Address     `toml:"tee_instructions"`
	Signer          router.Credentials `toml:"signer"` // credentials for signer
	XRP             router.Credentials `toml:"xrp"`    // credentials for xrp augmenter
	BTC             router.Credentials `toml:"btc"`    // credentials for btc augmenter
}
