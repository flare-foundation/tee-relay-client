package test_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/client/router"
	"github.com/flare-foundation/tee-relay-client/test"
	"github.com/stretchr/testify/require"
)

const TeeInstructionsAddress = "0x1bB2e744E5f7aFFC0dA0d87FA723Ae679f08ca80"

type Logs []database.Log

func TestE2E(t *testing.T) {
	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	cfg := signing.Config{
		Addr:       ":8080",
		APIKeyName: "X-API-KEY",
		APIKeys:    []string{"123"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	signer, cred := test.NewTestSigner(cfg, prv)

	go func() {
		err := signer.Run(ctx)
		require.Error(t, err)
	}()

	nilCred := &config.Credentials{
		APIKeyName: "",
		APIKey:     "",
		URL:        "",
	}

	router := router.New(cred, nilCred, nilCred)

	eventsFile, err := os.ReadFile("./events.json")
	require.NoError(t, err)

	events := []database.Log{}
	err = json.Unmarshal(eventsFile, &events)
	require.NoError(t, err)

	in := make(chan []database.Log)
	out := make(chan *instructions.InstructionBase)

	go instructions.Run(ctx, router, in, out)

	in <- events
	x := <-out

	require.Equal(t, 1, len(x.Signatures))

	cancel()
}
