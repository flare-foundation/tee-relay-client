package sender_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/internal/router"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/sender"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	rsigner "github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/flare-foundation/tee-relay-client/pkg/testutils"

	"github.com/stretchr/testify/require"
)

func TestPrepareInstruction(t *testing.T) {
	event := database.Log{
		Address:         "5e17b14ADd6c386305A32928F985b29bbA34Eff5",
		Data:            "00000000000000000000000000000000000000000000000000000000000000e0465f58525000000000000000000000000000000000000000000000000000000050415900000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000280000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000de0b6b3a76400000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000e0000000000000000000000000e4e29e5bc4b1b96ae1111b7b3492cf12ec20417b00000000000000000000000022334455667788990011223344556677889900110000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001368747470733a2f2f6578616d706c652e636f6d000000000000000000000000000000000000000000000000003344556677889900112233445566778899001122000000000000000000000000080610de2a1b93b3cea64e7251660fd843d8a0250000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000001468747470733a2f2f6578616d706c65322e636f6d000000000000000000000000000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000004746f646f000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		Topic0:          "f770e69a9fc05b7180797556ec4cedb6108ce2c56ffa76c84aa087efeb5e6963",
		Topic1:          "0000000000000000000000000000000000000000000000000000000000000000",
		Topic2:          "1122334455667788990011223344556677889900112233445566778899001122",
		Topic3:          "0000000000000000000000000000000000000000000000000000000000000001",
		TransactionHash: "f353de2db83e253b54034caaba46c6adcd0bb09aeb3e7bb2b545813def208ab6",
		LogIndex:        1,
		Timestamp:       1718113274,
		BlockNumber:     123,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	signerServer, cred, err := testutils.NewTestSigner(prv)
	require.NoError(t, err)

	go func() {
		err := signerServer.Run(ctx)
		require.Error(t, err)
	}()

	fdcCfg := config.FDC{
		Queues:    map[string]priority.Params{},
		Verifiers: map[string]config.Verifier{},
	}

	s := rsigner.NewRemote(cred)

	chainID := uint64(14)

	r, err := router.NewRouter(s, chainID, &fdcCfg, nil, false)
	require.NoError(t, err)

	out := make(chan *instructions.Base, 2)

	r.SetOut(out)
	r.StartQueues(ctx)

	err = r.Handle(ctx, event)
	require.NoError(t, err)

	// instr.Dispatch(out)

	select {
	case err := <-ctx.Done():
		t.Errorf("contex canceled: %v", err)
		cancel()
	case base := <-out:

		in, url, err := sender.PrepareInstruction(*base, 0)
		require.NoError(t, err)

		teeID1 := common.HexToAddress("e4e29E5BC4B1b96ae1111B7b3492cf12EC20417b")
		expectedURL := "https://example.com"

		require.Equal(t, teeID1, in.Data.TeeID)
		require.Equal(t, expectedURL, url)

		_, _, err = sender.PrepareInstruction(*base, 2)
		require.Error(t, err)

		// chack that base is unchanged
		require.Equal(t, base.GeneralData.TeeID, common.Address{})

		err = signerServer.Shutdown(ctx)
		require.NoError(t, err)

		cancel()
	}
}
