package instructions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestParseInstruction(t *testing.T) {
	// instruction event of F_XRP, PAY with "todo" message, no cosigners, and two machines to send to.
	eventDBjson := `{
        "Address": "5e17b14ADd6c386305A32928F985b29bbA34Eff5",
        "Data": "00000000000000000000000000000000000000000000000000000000000000e0465f58525000000000000000000000000000000000000000000000000000000050415900000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000280000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000de0b6b3a76400000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000e0000000000000000000000000e4e29e5bc4b1b96ae1111b7b3492cf12ec20417b00000000000000000000000022334455667788990011223344556677889900110000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001368747470733a2f2f6578616d706c652e636f6d000000000000000000000000000000000000000000000000003344556677889900112233445566778899001122000000000000000000000000080610de2a1b93b3cea64e7251660fd843d8a0250000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001468747470733a2f2f6578616d706c65322e636f6d000000000000000000000000000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000004746f646f000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		"Topic0": "f770e69a9fc05b7180797556ec4cedb6108ce2c56ffa76c84aa087efeb5e6963",
        "Topic1": "0000000000000000000000000000000000000000000000000000000000000000",
        "Topic2": "1122334455667788990011223344556677889900112233445566778899001122",
        "Topic3": "0000000000000000000000000000000000000000000000000000000000000001",
        "TransactionHash": "f353de2db83e253b54034caaba46c6adcd0bb09aeb3e7bb2b545813def208ab6",
        "LogIndex": 1,
        "Timestamp": 1718113274
      }`

	var event database.Log

	chainID := uint64(14)

	err := json.Unmarshal([]byte(eventDBjson), &event)
	require.NoError(t, err)

	ib, err := ParseInstruction(event)
	require.NoError(t, err)

	require.Len(t, ib.Signatures, 0)
	require.NotNil(t, ib.Event)

	require.Equal(t, uint64(1718113274), ib.GeneralData.Timestamp)
	require.Equal(t, common.Address{}, ib.GeneralData.TeeID)

	hashes, err := ib.hashesForSigning(chainID)
	require.NoError(t, err)
	require.Len(t, hashes, 2)

	// Golden: pin the prefix+chainID signing preimage for this fixed event. A
	// change here means the signed hash changed and must be matched on-chain.
	require.Equal(t, "0xf56e1e6e1b2402a990b7fcd389882bae8e23a8011db9d69b99580259c6d38277", hashes[0].Hex())
	require.Equal(t, "0x393b89cfa8eab701688465549fa1057ac4ef73c846b71e8603a4a4316e593757", hashes[1].Hex())

	require.Equal(t, common.Address{}, ib.GeneralData.TeeID)
}

// TestSignAndRecover signs an instruction through the real Base.Sign path and
// recovers the operator address from each per-tee signature, proving the
// prefix+chainID preimage round-trips. Recovering with the wrong chainID must
// NOT yield the operator, proving chainID is actually bound into the signed hash.
func TestSignAndRecover(t *testing.T) {
	const chainID = uint64(14)

	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	operator := crypto.PubkeyToAddress(priv.PublicKey)

	ib := &Base{
		Tees: []teeinstructions.IMachineManagerTeeMachine{
			{TeeId: common.HexToAddress("0x1111111111111111111111111111111111111111")},
			{TeeId: common.HexToAddress("0x2222222222222222222222222222222222222222")},
		},
		GeneralData: instruction.Data{
			DataFixed: instruction.DataFixed{
				InstructionID: common.HexToHash("0xabc"),
				Timestamp:     1718113274,
			},
		},
	}

	require.NoError(t, ib.Sign(context.Background(), signer.NewLocal(priv), chainID))
	require.Len(t, ib.Signatures, len(ib.Tees))

	for j := range ib.Tees {
		data := ib.GeneralData
		data.TeeID = ib.Tees[j].TeeId
		instr := instruction.Instruction{Data: data, Signature: ib.Signatures[j]}

		pub, err := instr.RecoverSignersPubKey(chainID)
		require.NoError(t, err)
		require.Equal(t, operator, crypto.PubkeyToAddress(*pub), "tee %d: operator must recover with the correct chainID", j)

		wrongPub, err := instr.RecoverSignersPubKey(chainID + 1)
		require.NoError(t, err)
		require.NotEqual(t, operator, crypto.PubkeyToAddress(*wrongPub), "tee %d: wrong chainID must not recover the operator", j)
	}
}
