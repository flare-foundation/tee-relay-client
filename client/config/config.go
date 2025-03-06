package config

import (
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

type Config struct {
	DB      database.Config `toml:"db"`
	Logging logger.Config   `toml:"logger"`
	Signer  Credentials     `toml:"signer"` // credentials for signer
	XRP     Credentials     `toml:"xrp"`    // credentials for xrp augmenter
	BTC     Credentials     `toml:"btc"`    // credentials for btc augmenter
}

type Credentials struct {
	APIKeyName string `toml:"api_key_name"`
	APIKey     string `toml:"api_key"`
	URL        string `toml:"url"`
}
