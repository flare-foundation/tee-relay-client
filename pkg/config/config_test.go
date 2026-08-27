package config

import (
	"encoding/hex"
	"math"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/flare-foundation/go-flare-common/pkg/priority"
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
	require.NoError(t, cfg.CheckRelayCutover())
	require.NoError(t, cfg.CheckStartInterval())
	require.NoError(t, cfg.CheckQueues())
	require.True(t, cfg.Signer.Local, "example must select the local signer — the external one is not implemented")
}

func TestRelayCutoverChainBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		starting int64
		epoch    uint32
		want     bool
	}{
		{name: "unset binds every epoch", starting: 0, epoch: 0, want: true},
		{name: "unset binds a late epoch", starting: 0, epoch: 5451, want: true},
		{name: "epoch below the boundary", starting: 417, epoch: 416, want: false},
		{name: "epoch at the boundary", starting: 417, epoch: 417, want: true},
		{name: "epoch past the boundary", starting: 417, epoch: 418, want: true},
		{name: "unscheduled binds nothing", starting: CutoverUnscheduled, epoch: 0, want: false},
		{name: "unscheduled binds no late epoch", starting: CutoverUnscheduled, epoch: 5451, want: false},
		// the boundary is expressible above the uint32 range CheckRelayCutover rejects
		{name: "largest reward epoch at the boundary", starting: math.MaxUint32, epoch: math.MaxUint32, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c := RelayCutover{StartingRewardEpoch: test.starting}
			require.Equal(t, test.want, c.ChainBound(test.epoch))
		})
	}
}

func TestCheckRelayCutover(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		starting int64
		errPart  string
	}{
		{name: "unset", starting: 0},
		{name: "scheduled", starting: 5451},
		{name: "unscheduled sentinel", starting: CutoverUnscheduled},
		{name: "largest reward epoch", starting: math.MaxUint32},
		{name: "other negative", starting: -2, errPart: "unscheduled"},
		{name: "far negative", starting: -5451, errPart: "unscheduled"},
		{name: "above the uint32 range", starting: math.MaxUint32 + 1, errPart: "exceeds"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{RelayCutover: RelayCutover{StartingRewardEpoch: test.starting}}
			err := cfg.CheckRelayCutover()

			if test.errPart == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.errPart)
		})
	}
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
