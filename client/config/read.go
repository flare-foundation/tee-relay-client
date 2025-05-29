package config

import (
	"fmt"

	"github.com/flare-foundation/go-flare-common/pkg/toml"
)

// ReadConfigs reads configs from toml file at filePath.
func ReadConfigs(filepath string) (*Config, error) {
	config, err := toml.ReadToml[Config](filepath, true)
	if err != nil {
		return nil, fmt.Errorf("configs: %v", err)
	}

	return &config, nil
}
