package sender

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/random"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/stretchr/testify/require"
)

func TestInstructionOPLogging(t *testing.T) {
	iId, err := random.Hash()
	require.NoError(t, err)

	teeID := common.BytesToAddress([]byte("myAddress"))

	data := instruction.Data{
		DataFixed: instruction.DataFixed{
			InstructionID:          iId,
			TeeID:                  teeID,
			Timestamp:              0,
			RewardEpochID:          0,
			OPType:                 op.XRP.Hash(),
			OPCommand:              op.Pay.Hash(),
			Cosigners:              []common.Address{},
			CosignersThreshold:     0,
			OriginalMessage:        hexutil.Bytes{},
			AdditionalFixedMessage: hexutil.Bytes{},
		},
		AdditionalVariableMessage: hexutil.Bytes{},
	}

	log := instructionOPLogging(data)

	expectedLog := "id: " + iId.String() + " opType: " + string(op.XRP) + ", opCommand: " + string(op.Pay)

	require.Equal(t, expectedLog, log)
}
