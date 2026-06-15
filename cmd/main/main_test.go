package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const validManager = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"

// writeConfig writes body to a temporary config.toml and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.Equal(t, uint64(14), cfg.ChainID)
		require.False(t, cfg.AllowUnsafeURLs)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := loadConfig(filepath.Join(t.TempDir(), "absent.toml"))
		require.ErrorContains(t, err, "reading config")
	})

	t.Run("unset FlareTeeManager", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, "chain_id = 14\n"))
		require.ErrorContains(t, err, "checking address")
	})

	t.Run("zero chain id", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"`+"\n"))
		require.ErrorContains(t, err, "checking chain id")
	})

	t.Run("ALLOW_UNSAFE_URLS override", func(t *testing.T) {
		t.Setenv("ALLOW_UNSAFE_URLS", "true")
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.True(t, cfg.AllowUnsafeURLs)
	})
}
