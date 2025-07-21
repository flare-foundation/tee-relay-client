package sender_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/client/sender"
	"github.com/flare-foundation/tee-relay-client/test"
	"github.com/stretchr/testify/require"
)

func TestPrepareInstruction(t *testing.T) {
	event := database.Log{
		Address:         "9D7f74d0C41E726EC95884E0e97Fa6129e3b5E99",
		Data:            "00000000000000000000000000000000000000000000000000000000000000a052454700000000000000000000000000000000000000000000000000000000005445455f4154544553544154494f4e00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000280000000000000000000000000000000000000000000000000000000000000006400000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000005b38da6a701c568545dcfcb03fcb875f56beddc40000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000002b68747470733a2f2f746573746e6574732e74686567726170682e636f6d2f7375626772617068732f69642f0000000000000000000000000000000000000000000000000000000000000000005b38da6a701c568545dcfcb03fcb875f56beddc400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000002c68747470733a2f2f746573746e6574732e74686567726170682e636f6d2f7375626772617068732f6964322f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000046e656b6900000000000000000000000000000000000000000000000000000000",
		Topic0:          "0b155d08a134ef49412293aa17be27ead7cc04b35ada16085a36c4355fd6f40d",
		Topic1:          "87c2d362de99f75a4f2755cdaaad2d11bf6cc65dc71356593c445535ff28f43d",
		Topic2:          "000000000000000000000000000000000000000000000000000000000000000a",
		Topic3:          "NULL",
		TransactionHash: "ca7d10dfe73672fd5ba4483bf2a579d04f0729e5aabe236f451a980bb4b9cd80",
		LogIndex:        0,
		Timestamp:       1718113274,
		BlockNumber:     123,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	cfg := signer.Config{
		Addr:       ":8080",
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{"123"},
	}

	signer, cred := test.NewTestSigner(cfg, prv)

	go func() {
		err := signer.Run(ctx)
		require.Error(t, err)
	}()

	ftdcCfg := config.FTDC{
		Queues:    map[string]priority.Params{},
		Verifiers: []config.Verifier{},
	}

	router := instructions.NewRouter(cred, &ftdcCfg)

	out := make(chan *instructions.Base, 2)

	router.Start(ctx, out)

	err = instructions.Handle(ctx, event, router)
	require.NoError(t, err)

	// instr.Dispatch(out)

	base := <-out

	in, url, err := sender.PrepareInstruction(*base, 1)
	require.NoError(t, err)

	teeID1 := common.HexToAddress("5B38Da6a701c568545dCfcB03FcB875f56beddC4")
	expectedURL := "https://testnets.thegraph.com/subgraphs/id2/"

	require.Equal(t, teeID1, in.Data.TeeId)
	require.Equal(t, expectedURL, url)

	_, _, err = sender.PrepareInstruction(*base, 2)
	require.Error(t, err)

	// chack that base is unchanged
	require.Equal(t, base.GeneralData.TeeId, common.Address{})

	err = signer.Shutdown(ctx)
	require.NoError(t, err)

	cancel()
}
