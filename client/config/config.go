package config

import (
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

type User struct {
	DB      database.Config `toml:"db"`
	Logging logger.Config   `toml:"logger"`
}
