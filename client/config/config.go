package config

import (
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/client/router"
)

type User struct {
	DB      database.Config     `toml:"db"`
	Logging logger.Config       `toml:"logger"`
	Signer  router.SignerConfig `toml:"signer"`
}
