package config

import (
	"testing"

	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	const path = "../../config.toml.example"

	_, err := toml.Read[Config](path, true)

	require.NoError(t, err)
}
