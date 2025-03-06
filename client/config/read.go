package config

import "github.com/flare-foundation/go-flare-common/toml"

// ReadConfigs reads user and system configurations from filePath
func ReadConfigs(filepath string) (*Config, error) {
	config, err := toml.ReadToml[Config](filepath)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
