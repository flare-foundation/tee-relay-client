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
// independently of fdc.ChainBoundRelayPrefixedHash.
func chainBoundPreimageHash(t *testing.T, chainID uint64, messageHash common.Hash) common.Hash {
	t.Helper()

	// big.Int keeps the word construction independent of the helper's binary.BigEndian
	word := common.LeftPadBytes(new(big.Int).SetUint64(chainID).Bytes(), 32)
	preimage := append(word, common.FromHex("0x010000000000")...)
	preimage = append(preimage, messageHash.Bytes()...)
	require.Len(t, preimage, 70)

	return crypto.Keccak256Hash(preimage)
}

func TestRelayDigest(t *testing.T) {
	t.Parallel()

	t.Run("matches the contract digest", func(t *testing.T) {
		t.Parallel()

		// evaluated on the EVM from Fdc2ProofVerification.toCosignersMessageHash:
		// keccak256(bytes.concat(bytes32(block.chainid), hex"010000000000", _messageHash))
		// with block.chainid 14 and digestMessageHash.
		golden := common.HexToHash("0x452299ddddf6ab40ffe351e672e786ab155216ed644c7eb9cfbcac88efd027f4")

		require.Equal(t, golden, fdc.ChainBoundRelayPrefixedHash(14, digestMessageHash))
		require.Equal(t, golden, chainBoundPreimageHash(t, 14, digestMessageHash))
	})

	t.Run("binds the chain", func(t *testing.T) {
		t.Parallel()

		for _, chainID := range []uint64{14, 16, 19, 114, 31337} {
			require.Equal(t, chainBoundPreimageHash(t, chainID, digestMessageHash), fdc.ChainBoundRelayPrefixedHash(chainID, digestMessageHash))
		}
		require.NotEqual(t, fdc.ChainBoundRelayPrefixedHash(14, digestMessageHash), fdc.ChainBoundRelayPrefixedHash(16, digestMessageHash))
	})
}
