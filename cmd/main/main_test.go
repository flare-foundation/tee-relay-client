package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
		require.Nil(t, cfg.RelayCutover)
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

	// the cutover is over; a leftover block loads and is only flagged for a warning
	t.Run("retired relay cutover", func(t *testing.T) {
		for name, block := range map[string]string{
			"scheduled":   "[relay_cutover]\nstarting_reward_epoch = 5451\n",
			"unscheduled": "[relay_cutover]\nstarting_reward_epoch = -1\n",
			"empty":       "[relay_cutover]\n",
		} {
			t.Run(name, func(t *testing.T) {
				cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`+block))
				require.NoError(t, err)
				require.NotNil(t, cfg.RelayCutover)
				require.Equal(t, uint64(14), cfg.ChainID)
			})
		}
	})

	t.Run("health custom", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[health]
port = 9090
max_indexer_lag = "45s"
`))
		require.NoError(t, err)
		require.False(t, cfg.Health.Disabled)
		require.Equal(t, 9090, cfg.Health.Port)
		require.Equal(t, 45*time.Second, cfg.Health.MaxIndexerLag)
	})

	// omitted section: the server is on, on the port the image EXPOSEs
	t.Run("health omitted", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
`))
		require.NoError(t, err)
		require.False(t, cfg.Health.Disabled)
		require.Equal(t, config.DefaultHealthPort, cfg.Health.Port)
		require.Equal(t, config.DefaultMaxIndexerLag, cfg.Health.MaxIndexerLag)
	})

	t.Run("health disabled", func(t *testing.T) {
		cfg, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[health]
disabled = true
`))
		require.NoError(t, err)
		require.True(t, cfg.Health.Disabled)
	})

	t.Run("health port out of range", func(t *testing.T) {
		for _, port := range []string{"0", "-1", "65536"} {
			t.Run(port, func(t *testing.T) {
				_, err := loadConfig(writeConfig(t, `flare_tee_manager = "`+validManager+`"
chain_id = 14
[health]
port = `+port+`
`))
				require.ErrorContains(t, err, "checking health")
			})
		}
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

func TestHealthWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		health config.Health
		want   string // substring; empty means no warning
	}{
		{name: "default", health: config.Health{Port: config.DefaultHealthPort}},
		{name: "disabled", health: config.Health{Disabled: true, Port: config.DefaultHealthPort}, want: "disabled"},
		{name: "disabled wins over port", health: config.Health{Disabled: true, Port: 9090}, want: "disabled"},
		{name: "custom port", health: config.Health{Port: 9090}, want: "health port 9090 is not the default 8080"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := healthWarning(tc.health)
			if tc.want == "" {
				require.Empty(t, got)
				return
			}
			require.Contains(t, got, tc.want)
		})
	}
}
