package testutils_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/tee-relay-client/internal/router"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

const TeeInstructionsAddress = "0x1bB2e744E5f7aFFC0dA0d87FA723Ae679f08ca80"

type Logs []database.Log

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := uint64(14)

	sgnr := signer.NewLocal(prv)

	f, err := router.NewFilterer(false, sgnr)
	require.NoError(t, err)

	rtr, err := router.NewRouter(sgnr, chainID, config.RelayCutover{}, nil, f, false)
	require.NoError(t, err)

	eventsFile, err := os.ReadFile("./events.json")
	require.NoError(t, err)

	events := []database.Log{}
	err = json.Unmarshal(eventsFile, &events)
	require.NoError(t, err)

	in := make(chan []database.Log)
	out := make(chan *instructions.Base, 3)

	var wg sync.WaitGroup
	rtr.Run(ctx, &wg, in, out)

	in <- events

	id0 := events[0].Topic2
	id1 := events[1].Topic2

	time.Sleep(10 * time.Millisecond)

	require.Len(t, out, 2)

	for range 2 {
		select {
		case x := <-out:
			switch hex.EncodeToString(x.GeneralData.InstructionID[:]) {
			case id0:
				require.Equal(t, 2, len(x.Signatures))
			case id1:
				require.Equal(t, 2, len(x.Signatures))
			default:
				t.Error("no matching id")
			}
		case <-ctx.Done():
			t.Error("timed out")
		}
	}

	cancel()
}

func TestIntegrationCosigner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)

	chainID := uint64(14)

	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	sgnr := signer.NewLocal(prv)

	f, err := router.NewFilterer(true, sgnr)
	require.NoError(t, err)

	rtr, err := router.NewRouter(sgnr, chainID, config.RelayCutover{}, nil, f, false)
	require.NoError(t, err)

	eventsFile, err := os.ReadFile("./events.json")
	require.NoError(t, err)

	events := []database.Log{}
	err = json.Unmarshal(eventsFile, &events)
	require.NoError(t, err)

	in := make(chan []database.Log, 10)
	out := make(chan *instructions.Base, 10)

	var wg sync.WaitGroup
	rtr.Run(ctx, &wg, in, out)

	in <- events

	time.Sleep(10 * time.Millisecond)

	require.Len(t, out, 0)

	cancel()
}
