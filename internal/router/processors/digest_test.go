package processors

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
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

		require.Equal(t, golden, relayPrefixedHash(14, true, digestMessageHash))
		require.Equal(t, golden, chainBoundPreimageHash(t, 14, digestMessageHash))
	})

	t.Run("binds the chain", func(t *testing.T) {
		t.Parallel()

		require.NotEqual(t, relayPrefixedHash(14, true, digestMessageHash), relayPrefixedHash(16, true, digestMessageHash))
	})

	t.Run("pre-cutover form ignores the chain", func(t *testing.T) {
		t.Parallel()

		unbound := fdc.RelayPrefixedHash(digestMessageHash)

		require.Equal(t, unbound, relayPrefixedHash(14, false, digestMessageHash))
		require.Equal(t, unbound, relayPrefixedHash(16, false, digestMessageHash))
		require.NotEqual(t, unbound, relayPrefixedHash(14, true, digestMessageHash))
	})

	// The form follows the cutover for every chain and epoch, so the assertion holds
	// whatever a deployment configures.
	t.Run("form follows the cutover", func(t *testing.T) {
		t.Parallel()

		unbound := fdc.RelayPrefixedHash(digestMessageHash)
		var sawBound, sawUnbound bool

		for _, cutover := range []config.RelayCutover{
			{StartingRewardEpoch: config.CutoverUnscheduled},
			{StartingRewardEpoch: 0},
			{StartingRewardEpoch: 5451},
		} {
			for _, chainID := range []uint64{14, 16, 19, 114, 31337} {
				for _, epoch := range []uint32{0, 1, 417, 5451, 5452} {
					chainBound := cutover.ChainBound(epoch)
					got := relayPrefixedHash(chainID, chainBound, digestMessageHash)

					if chainBound {
						sawBound = true
						require.Equal(t, chainBoundPreimageHash(t, chainID, digestMessageHash), got)
						require.NotEqual(t, unbound, got)
					} else {
						sawUnbound = true
						require.Equal(t, unbound, got)
					}
				}
			}
		}

		// guards against a one-sided sweep that would pass with either branch broken
		require.True(t, sawBound, "no chain/epoch selected the chain-bound digest")
		require.True(t, sawUnbound, "no chain/epoch selected the pre-cutover digest")
	})
}
