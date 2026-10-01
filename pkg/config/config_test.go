package config

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/stretchr/testify/require"
)

const managerHex = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"

// TestConfig guards the quickstart: config.toml.example must be a config the relay
// actually accepts, not merely one that parses.
func TestConfig(t *testing.T) {
	const path = "../../config.toml.example"

	cfg := Default()
	require.NoError(t, toml.ReadTo(path, &cfg, false))
	require.NoError(t, cfg.CheckAddress())
	require.NoError(t, cfg.CheckChainID())
	require.NoError(t, cfg.CheckStartInterval())
	require.NoError(t, cfg.CheckQueues())
	require.NoError(t, cfg.CheckHealth())
	require.True(t, cfg.Signer.Local, "example must select the local signer — the external one is not implemented")
	require.False(t, cfg.Health.Disabled, "example must keep the health server on — probes need it")
	require.Equal(t, DefaultHealthPort, cfg.Health.Port, "example must keep the port the image EXPOSEs")
}

func TestCheckHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		health  Health
		wantErr string
	}{
		{name: "default port", health: Health{Port: DefaultHealthPort, MaxIndexerLag: DefaultMaxIndexerLag}},
		{name: "custom port", health: Health{Port: 9090, MaxIndexerLag: DefaultMaxIndexerLag}},
		{name: "highest port", health: Health{Port: 65535, MaxIndexerLag: DefaultMaxIndexerLag}},
		{name: "disabled", health: Health{Disabled: true}},
		// port and lag are inert while disabled, so bad ones must not fail a port-less deployment
		{name: "disabled ignores port and lag", health: Health{Disabled: true, Port: -1, MaxIndexerLag: -time.Second}},
		{name: "zero port", health: Health{MaxIndexerLag: DefaultMaxIndexerLag}, wantErr: "health port"},
		{name: "negative port", health: Health{Port: -1, MaxIndexerLag: DefaultMaxIndexerLag}, wantErr: "health port"},
		{name: "port above range", health: Health{Port: 65536, MaxIndexerLag: DefaultMaxIndexerLag}, wantErr: "health port"},
		{name: "zero lag", health: Health{Port: DefaultHealthPort}, wantErr: "max_indexer_lag"},
		{name: "negative lag", health: Health{Port: DefaultHealthPort, MaxIndexerLag: -time.Second}, wantErr: "max_indexer_lag"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{Health: tc.health}

			err := cfg.CheckHealth()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// The defaults must pass CheckHealth, or a config without [health] fails to load.
func TestDefaultHealth(t *testing.T) {
	t.Parallel()

	cfg := Default()
	require.False(t, cfg.Health.Disabled)
	require.Equal(t, DefaultHealthPort, cfg.Health.Port)
	require.Equal(t, DefaultMaxIndexerLag, cfg.Health.MaxIndexerLag)
	require.Equal(t, ":8080", cfg.Health.Address())
	require.NoError(t, cfg.CheckHealth())
}

func TestCheckQueues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  priority.Params
		wantErr string
	}{
		{
			name:   "retries with time off",
			params: priority.Params{MaxAttempts: 3, TimeOff: 2 * time.Second},
		},
		{
			name:   "single attempt needs no time off",
			params: priority.Params{MaxAttempts: 1},
		},
		{
			name:    "zero max attempts",
			params:  priority.Params{TimeOff: 2 * time.Second},
			wantErr: "max_attempts",
		},
		{
			name:    "retries without time off",
			params:  priority.Params{MaxAttempts: 3},
			wantErr: "time_off",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{FDC: FDC{Queues: map[string]priority.Params{"q": tc.params}}}

			err := cfg.CheckQueues()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			require.ErrorContains(t, err, `"q"`)
		})
	}
}

func TestCredentialsCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		creds   Credentials
		wantErr string
	}{
		{
			name:  "valid with key",
			creds: Credentials{URL: "https://verifier.example.com", KeyName: "X-API-KEY", Key: "secret"},
		},
		{
			name:  "valid without key",
			creds: Credentials{URL: "http://localhost:8080"},
		},
		{
			name:    "empty URL",
			creds:   Credentials{},
			wantErr: "URL not set",
		},
		{
			name:    "missing scheme parses as scheme",
			creds:   Credentials{URL: "localhost:8080"},
			wantErr: "scheme",
		},
		{
			name:    "unsupported scheme",
			creds:   Credentials{URL: "ftp://host"},
			wantErr: "scheme",
		},
		{
			name:    "missing host",
			creds:   Credentials{URL: "http://"},
			wantErr: "host",
		},
		{
			name:    "key without name",
			creds:   Credentials{URL: "http://host", Key: "secret"},
			wantErr: "unnamed api key",
		},
		{
			name:    "spaced key name",
			creds:   Credentials{URL: "http://host", KeyName: "X API KEY", Key: "secret"},
			wantErr: "header name",
		},
		{
			name:    "newline in key",
			creds:   Credentials{URL: "http://host", KeyName: "X-API-KEY", Key: "se\ncret"},
			wantErr: "CR or LF",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.creds.Check()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// unsetManagerEnv clears FlareTeeManagerVariable for the duration of the test — an
// ambient value would otherwise decide the outcome of the cases that assert on its absence.
func unsetManagerEnv(t *testing.T) {
	t.Helper()
	t.Setenv(FlareTeeManagerVariable, "") // registers the restore of the original value
	require.NoError(t, os.Unsetenv(FlareTeeManagerVariable))
}

func TestApplyFlareTeeManagerEnv(t *testing.T) {
	manager := common.HexToAddress(managerHex)
	other := common.HexToAddress("0x1111111111111111111111111111111111111111")

	tests := []struct {
		name     string
		setEnv   bool
		envValue string
		cfgAddr  common.Address
		want     common.Address
		fail     bool
	}{
		{name: "not set keeps the config address", cfgAddr: manager, want: manager},
		{name: "not set and no config address", want: zeroAddress},
		{name: "env only", setEnv: true, envValue: managerHex, want: manager},
		{name: "env matches config", setEnv: true, envValue: managerHex, cfgAddr: manager, want: manager},
		{name: "checksum ignored", setEnv: true, envValue: "0xde25c06982ab8e4b6b4f910896e3f93ac77fb44d", cfgAddr: manager, want: manager},
		{name: "padding trimmed", setEnv: true, envValue: " " + managerHex + "\n", want: manager},
		{name: "env contradicts config", setEnv: true, envValue: other.Hex(), cfgAddr: manager, fail: true},
		{name: "empty", setEnv: true, envValue: "", fail: true},
		{name: "whitespace only", setEnv: true, envValue: "  ", fail: true},
		{name: "no 0x prefix", setEnv: true, envValue: managerHex[2:], fail: true},
		{name: "too short", setEnv: true, envValue: managerHex[:20], fail: true},
		{name: "right length but not hex", setEnv: true, envValue: "0x" + strings.Repeat("z", 2*common.AddressLength), fail: true},
		{name: "zero address", setEnv: true, envValue: zeroAddress.Hex(), fail: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(FlareTeeManagerVariable, tt.envValue)
			} else {
				unsetManagerEnv(t)
			}

			cfg := Config{FlareTeeManager: tt.cfgAddr}
			err := cfg.ApplyFlareTeeManagerEnv()

			if tt.fail {
				require.Error(t, err)
				require.Equal(t, tt.cfgAddr, cfg.FlareTeeManager, "a rejected value must not be applied")

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.FlareTeeManager)
		})
	}
}

func TestPrivateKeyFromEnv(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	keyHex := hex.EncodeToString(crypto.FromECDSA(key))

	tests := []struct {
		name         string
		variableName string
		envVarToSet  string
		envValue     string
		fail         bool
	}{
		{
			name:         "valid key with specific env var",
			variableName: "TEST_PK",
			envVarToSet:  "TEST_PK",
			envValue:     keyHex,
			fail:         false,
		},
		{
			name:         "valid key with 0x prefix",
			variableName: "TEST_PK_0X",
			envVarToSet:  "TEST_PK_0X",
			envValue:     "0x" + keyHex,
			fail:         false,
		},
		{
			name:         "valid key with 0X prefix",
			variableName: "TEST_PK_0X_CAPS",
			envVarToSet:  "TEST_PK_0X_CAPS",
			envValue:     "0X" + keyHex,
			fail:         false,
		},
		{
			name:         "empty variable name uses default",
			variableName: "",
			envVarToSet:  DefaultPrivateKeyVariable,
			envValue:     keyHex,
			fail:         false,
		},
		{
			name:         "not set",
			variableName: "UNSET_VAR",
			fail:         true,
		},
		{
			name:         "too short odd",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "123",
			fail:         true,
		},
		{
			name:         "too short even",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "1231",
			fail:         true,
		},
		{
			name:         "too short even prefixed",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "0x1231",
			fail:         true,
		},
		{
			name:         "too long odd",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "0x" + keyHex + "123",
			fail:         true,
		},
		{
			name:         "too long even",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "0x" + keyHex + "1234",
			fail:         true,
		},
		{
			name:         "not hex",
			variableName: "INVALID_KEY_VAR",
			envVarToSet:  "INVALID_KEY_VAR",
			envValue:     "not-a-hex-key",
			fail:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVarToSet != "" {
				t.Setenv(tt.envVarToSet, tt.envValue)
			}

			pk, err := PrivateKeyFromEnv(tt.variableName)
			require.Equal(t, tt.fail, err != nil)
			if !tt.fail {
				require.NotNil(t, pk)
				require.Equal(t, key, pk)
			}
		})
	}
}
