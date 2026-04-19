package router

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestFilterer(t *testing.T) {
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(priv.PublicKey)

	addresses := []common.Address{common.HexToAddress("0x7777"), common.HexToAddress("0x8888"), common.HexToAddress("0x9999")}

	addressesWith := append([]common.Address{}, addresses...)
	addressesWith = append(addressesWith, address)

	ls := signer.NewLocal(priv)

	eventPass := instructions.InstructionSentEvent{
		ExtensionId:        &big.Int{},
		InstructionId:      [32]byte{},
		RewardEpochId:      0,
		TeeMachines:        []teeinstructions.IMachineManagerFacetTeeMachine{},
		OpType:             [32]byte{},
		OpCommand:          [32]byte{},
		Message:            []byte{},
		Cosigners:          addressesWith,
		CosignersThreshold: 0,
		Fee:                &big.Int{},
		Raw:                types.Log{},
	}

	event := instructions.InstructionSentEvent{
		ExtensionId:        &big.Int{},
		InstructionId:      [32]byte{},
		RewardEpochId:      0,
		TeeMachines:        []teeinstructions.IMachineManagerFacetTeeMachine{},
		OpType:             [32]byte{},
		OpCommand:          [32]byte{},
		Message:            []byte{},
		Cosigners:          addresses,
		CosignersThreshold: 0,
		Fee:                &big.Int{},
		Raw:                types.Log{},
	}

	t.Run("cosigner", func(t *testing.T) {
		f, err := NewFilterer(true, ls)
		require.NoError(t, err)

		pass := f.Filter(&eventPass)
		require.False(t, pass)

		nopass := f.Filter(&event)
		require.True(t, nopass)
	})

	t.Run("provider", func(t *testing.T) {
		f, err := NewFilterer(false, ls)
		require.NoError(t, err)

		pass := f.Filter(&eventPass)
		require.False(t, pass)

		nopass := f.Filter(&event)
		require.False(t, nopass)
	})
}
