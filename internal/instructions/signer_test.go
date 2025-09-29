package instructions

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestSigner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	cfg := signer.Config{
		Addr:       ":8080",
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{"123"},
	}

	signerServer, cred := testutils.NewTestSigner(cfg, prv)

	go func() {
		err := signerServer.Run(ctx)
		require.Error(t, err)
	}()

	t.Cleanup(func() {
		cancel()
	})

	s := Signer{cred}

	t.Run("identify", func(t *testing.T) {
		id, err := s.Identify(ctx)
		require.NoError(t, err)
		require.Equal(t, types.PubKeyToStruct(&prv.PublicKey), *id)
	})

	t.Run("sign", func(t *testing.T) {
		numsOfHashes := []int{1, 10, 50, 100}

		for _, j := range numsOfHashes {
			hashes := make([]common.Hash, j)
			for i := range j {
				hashes[i] = crypto.Keccak256Hash([]byte(strconv.FormatUint(uint64(i), 10)))
			}

			sigs, err := s.FetchSignatures(ctx, hashes)
			require.NoError(t, err)

			require.Len(t, sigs, j)

			testSigIndex := j / 2

			pk, err := crypto.SigToPub(accounts.TextHash(hashes[testSigIndex][:]), sigs[testSigIndex])
			require.NoError(t, err)
			require.Equal(t, prv.PublicKey, *pk)
		}
	})

	t.Run("decrypt", func(t *testing.T) {
		plaintext := []byte("plaintextThatIsShort")

		pke := ecies.ImportECDSAPublic(&prv.PublicKey)

		cipher, err := ecies.Encrypt(rand.Reader, pke, plaintext, nil, nil)
		require.NoError(t, err)

		dec, err := s.Decrypt(ctx, cipher)
		require.NoError(t, err)

		require.Equal(t, plaintext, []byte(dec))
	})
}
