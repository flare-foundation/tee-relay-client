package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

const validManager = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"

const otherManager = "0x1111111111111111111111111111111111111111"

// writeConfig writes body to a temporary config.toml and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// unsetManagerEnv clears FLARE_TEE_MANAGER_CONTRACT_ADDRESS for the duration of the test —
// an ambient value supplies the address the cases below expect to come from the config file.
func unsetManagerEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.FlareTeeManagerVariable, "") // registers the restore of the original value
	require.NoError(t, os.Unsetenv(config.FlareTeeManagerVariable))
}

func TestLoadConfig(t *testing.T) {
	unsetManagerEnv(t) // subtests that want the variable set it themselves

	t.Run("valid", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.Equal(t, uint64(14), cfg.ChainID)
		require.False(t, cfg.AllowUnsafeURLs)
		require.Equal(t, int64(100), cfg.Collector.StartInterval) // the default README documents
	})

	t.Run("start interval override", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[collector]
start_interval = 7
`))
		require.NoError(t, err)
		require.Equal(t, int64(7), cfg.Collector.StartInterval)
	})

	// an explicit zero must survive, not fall back to the default
	t.Run("start interval explicit zero", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[collector]
start_interval = 0
`))
		require.NoError(t, err)
		require.Zero(t, cfg.Collector.StartInterval)
	})

	// a negative value would decode to 2^64-1 if the field were unsigned
	t.Run("start interval negative", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[collector]
start_interval = -1
`))
		require.ErrorContains(t, err, "checking start interval")
	})

	t.Run("unknown key", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
is_cosignerr = true
`))
		require.ErrorContains(t, err, "unknown field")
	})

	// AllowUnsafeURLs carries toml:"-", so the config file cannot reach it
	t.Run("AllowUnsafeURLs not settable from file", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
AllowUnsafeURLs = true
`))
		require.ErrorContains(t, err, "unknown field")
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

	t.Run("invalid relay cutover", func(t *testing.T) {
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14

[relay_cutover]
starting_reward_epoch = -2
`))
		require.ErrorContains(t, err, "checking relay cutover")
	})

	t.Run("ALLOW_UNSAFE_URLS override", func(t *testing.T) {
		t.Setenv("ALLOW_UNSAFE_URLS", "true")
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.True(t, cfg.AllowUnsafeURLs)
	})

	// the env variable alone must satisfy CheckAddress, so it has to be applied before it
	t.Run("manager from env only", func(t *testing.T) {
		t.Setenv(config.FlareTeeManagerVariable, validManager)
		cfg, err := loadConfig(writeConfig(t, "chain_id = 14\n"))
		require.NoError(t, err)
		require.Equal(t, common.HexToAddress(validManager), cfg.FlareTeeManager)
	})

	t.Run("manager from env agreeing with the config", func(t *testing.T) {
		t.Setenv(config.FlareTeeManagerVariable, validManager)
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.Equal(t, common.HexToAddress(validManager), cfg.FlareTeeManager)
	})

	t.Run("manager from env contradicting the config", func(t *testing.T) {
		t.Setenv(config.FlareTeeManagerVariable, otherManager)
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.ErrorContains(t, err, "reading manager address from env")
	})

	t.Run("manager from env not an address", func(t *testing.T) {
		t.Setenv(config.FlareTeeManagerVariable, "nonsense")
		_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.ErrorContains(t, err, "reading manager address from env")
	})
}
