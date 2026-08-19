package processors

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/stretchr/testify/require"
)

var digestMessageHash = common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")

// chainBoundPreimageHash mirrors Fdc2ProofVerification.toCosignersMessageHash
// independently of relayPrefixedHash.
func chainBoundPreimageHash(t *testing.T, chainID uint64, messageHash common.Hash) common.Hash {
	t.Helper()

	// big.Int keeps the word construction independent of the helper's binary.BigEndian
	word := common.LeftPadBytes(new(big.Int).SetUint64(chainID).Bytes(), 32)
	preimage := append(word, common.FromHex("0x010000000000")...)
	preimage = append(preimage, messageHash.Bytes()...)
	require.Len(t, preimage, 70)

	return crypto.Keccak256Hash(preimage)
}

func TestRelayPrefixedHash(t *testing.T) {
	t.Parallel()

	t.Run("matches the contract digest", func(t *testing.T) {
		t.Parallel()

		// evaluated on the EVM from Fdc2ProofVerification.toCosignersMessageHash:
		// keccak256(bytes.concat(bytes32(block.chainid), hex"010000000000", _messageHash))
		// with block.chainid 14 and digestMessageHash.
		golden := common.HexToHash("0x452299ddddf6ab40ffe351e672e786ab155216ed644c7eb9cfbcac88efd027f4")

		require.Equal(t, golden, relayPrefixedHash(14, 0, digestMessageHash))
		require.Equal(t, golden, chainBoundPreimageHash(t, 14, digestMessageHash))
	})

	t.Run("binds the chain", func(t *testing.T) {
		t.Parallel()

		require.NotEqual(t, relayPrefixedHash(14, 0, digestMessageHash), relayPrefixedHash(16, 0, digestMessageHash))
	})

	// The form follows chainBoundDigest for every chain and epoch, so the assertion holds
	// whatever the table says — filling in a breaking epoch cannot make it stale.
	t.Run("form follows the schedule", func(t *testing.T) {
		t.Parallel()

		unbound := fdc.RelayPrefixedHash(digestMessageHash)
		var sawBound, sawUnbound bool

		for _, chainID := range []uint64{14, 16, 19, 114, 31337} {
			for _, epoch := range []uint32{0, 1, 417, 5451, epochUnscheduled - 1} {
				got := relayPrefixedHash(chainID, epoch, digestMessageHash)

				if chainBoundDigest(chainID, epoch) {
					sawBound = true
					require.Equal(t, chainBoundPreimageHash(t, chainID, digestMessageHash), got)
					require.NotEqual(t, unbound, got)
				} else {
					sawUnbound = true
					require.Equal(t, unbound, got)
				}
			}
		}

		// guards against a one-sided sweep that would pass with either branch broken
		require.True(t, sawBound, "no chain/epoch selected the chain-bound digest")
		require.True(t, sawUnbound, "no chain/epoch selected the pre-cutover digest")
	})
}

func TestChainBoundFromEpoch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		breaking      uint32
		rewardEpochID uint32
		want          bool
	}{
		{name: "epoch below the boundary", breaking: 417, rewardEpochID: 416, want: false},
		{name: "epoch at the boundary", breaking: 417, rewardEpochID: 417, want: true},
		{name: "epoch past the boundary", breaking: 417, rewardEpochID: 418, want: true},
		{name: "boundary zero is always bound", breaking: 0, rewardEpochID: 0, want: true},
		{name: "unscheduled is never bound", breaking: epochUnscheduled, rewardEpochID: 5451, want: false},
		{name: "unscheduled is not bound at the sentinel itself", breaking: epochUnscheduled, rewardEpochID: epochUnscheduled, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, test.want, chainBoundFromEpoch(test.breaking, test.rewardEpochID))
		})
	}
}

func TestChainBoundDigest(t *testing.T) {
	t.Parallel()

	t.Run("flare is bound from the first epoch", func(t *testing.T) {
		t.Parallel()

		require.True(t, chainBoundDigest(14, 0))
		require.True(t, chainBoundDigest(14, 5451))
	})

	t.Run("an unlisted chain is bound at every epoch", func(t *testing.T) {
		t.Parallel()

		require.NotContains(t, breakingRewardEpochs, uint64(31337))
		require.True(t, chainBoundDigest(31337, 0))
		require.True(t, chainBoundDigest(31337, 5451))
	})

	// Fails once a real epoch replaces the placeholder — update it together with the table.
	t.Run("transitioning chains are still unscheduled", func(t *testing.T) {
		t.Parallel()

		for _, chainID := range []uint64{16, 19, 114} {
			require.Equal(t, epochUnscheduled, breakingRewardEpochs[chainID])
			require.False(t, chainBoundDigest(chainID, 5451))
		}
	})
}
