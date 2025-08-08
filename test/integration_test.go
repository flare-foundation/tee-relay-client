package test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/stretchr/testify/require"
)

const TeeInstructionsAddress = "0x1bB2e744E5f7aFFC0dA0d87FA723Ae679f08ca80"

type Logs []database.Log

func TestIntegration(t *testing.T) {
	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	cfg := signer.Config{
		Addr:       ":8080",
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{"123"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	signer, cred := NewTestSigner(cfg, prv)

	go func() {
		err := signer.Run(ctx)
		require.Error(t, err)
	}()

	router := instructions.NewRouter(cred, nil)

	eventsFile, err := os.ReadFile("./events.json")
	require.NoError(t, err)

	events := []database.Log{}
	err = json.Unmarshal(eventsFile, &events)
	require.NoError(t, err)

	in := make(chan []database.Log)
	out := make(chan *instructions.Base)

	instructions.Run(ctx, router, in, out)

	in <- events

	id0 := events[0].Topic1
	id1 := events[1].Topic1

	for range 2 {
		x := <-out

		switch hex.EncodeToString(x.GeneralData.InstructionID[:]) {
		case id0:
			require.Equal(t, 1, len(x.Signatures))
		case id1:
			require.Equal(t, 2, len(x.Signatures))
		default:
			t.Error("no matching id")
		}
	}

	cancel()
}
