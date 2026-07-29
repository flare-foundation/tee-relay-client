package config

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/flare-foundation/go-flare-common/pkg/toml"
	"github.com/stretchr/testify/require"
)

// TestConfig guards the quickstart: config.toml.example must be a config the relay
// actually accepts, not merely one that parses.
func TestConfig(t *testing.T) {
	const path = "../../config.toml.example"

	cfg := Default()
	require.NoError(t, toml.ReadTo(path, &cfg, false))
	require.NoError(t, cfg.CheckAddress())
	require.NoError(t, cfg.CheckChainID())
	require.NoError(t, cfg.CheckStartInterval())
	require.True(t, cfg.Signer.Local, "example must select the local signer — the external one is not implemented")
	require.False(t, cfg.AllowUnsafeURLs)
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
