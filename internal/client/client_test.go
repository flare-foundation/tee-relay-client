package client

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testKeyHex(t *testing.T) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	return hexutil.Encode(crypto.FromECDSA(key))
}

func memDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestSetSigner(t *testing.T) {
	t.Run("local with valid key", func(t *testing.T) {
		t.Setenv("PRIVATE_KEY", testKeyHex(t))
		s, err := setSigner(&config.Signer{Local: true, PrivateKeyVariable: "PRIVATE_KEY"})
		require.NoError(t, err)
		require.IsType(t, &signer.Local{}, s)
	})

	t.Run("local with missing key", func(t *testing.T) {
		_, err := setSigner(&config.Signer{Local: true, PrivateKeyVariable: "TEE_RELAY_UNSET_KEY_VAR"})
		require.ErrorContains(t, err, "private key")
	})

	t.Run("remote with valid credentials", func(t *testing.T) {
		s, err := setSigner(&config.Signer{
			Credentials: config.Credentials{URL: "http://signer", KeyName: "X-API-KEY", Key: "k"},
		})
		require.NoError(t, err)
		require.IsType(t, &signer.Remote{}, s)
	})

	t.Run("remote with empty URL", func(t *testing.T) {
		_, err := setSigner(&config.Signer{})
		require.ErrorContains(t, err, "URL not set")
	})
}

func TestNewWithDB(t *testing.T) {
	manager := common.HexToAddress("0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d")

	t.Run("provider mode", func(t *testing.T) {
		t.Setenv("PRIVATE_KEY", testKeyHex(t))
		cfg := config.Config{
			ChainID:         14,
			FlareTeeManager: manager,
			Signer:          config.Signer{Local: true, PrivateKeyVariable: "PRIVATE_KEY"},
		}
		c, err := newWithDB(cfg, memDB(t))
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("cosigner mode identifies the local signer", func(t *testing.T) {
		t.Setenv("PRIVATE_KEY", testKeyHex(t))
		cfg := config.Config{
			ChainID:         14,
			FlareTeeManager: manager,
			IsCosigner:      true,
			Signer:          config.Signer{Local: true, PrivateKeyVariable: "PRIVATE_KEY"},
		}
		c, err := newWithDB(cfg, memDB(t))
		require.NoError(t, err)
		require.NotNil(t, c)
	})

	t.Run("signer misconfiguration surfaces", func(t *testing.T) {
		cfg := config.Config{ChainID: 14, FlareTeeManager: manager} // remote signer, empty URL
		_, err := newWithDB(cfg, memDB(t))
		require.ErrorContains(t, err, "could not set signer")
	})
}
