package test_test

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/stretchr/testify/require"
)

const TeeInstructionsAddress = "0x1bB2e744E5f7aFFC0dA0d87FA723Ae679f08ca80"

type TestRouter struct {
	prv *ecdsa.PrivateKey
}

func (tr TestRouter) Sign(hashes []common.Hash) ([]hexutil.Bytes, error) {
	out := make([]hexutil.Bytes, len(hashes))

	var err error

	for j := range hashes {
		out[j], err = instruction.SignInstructionHash(hashes[j], tr.prv)
		if err != nil {
			return nil, err
		}
	}

	return out, err
}

func (tr TestRouter) Augment(opType common.Hash, opCommand common.Hash, message hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error) {
	return hexutil.Bytes{}, hexutil.Bytes{}, nil
}

type Logs []database.Log

func TestE2E(t *testing.T) {
	prv, err := crypto.GenerateKey()
	require.NoError(t, err)

	router := TestRouter{
		prv: prv,
	}

	eventsFile, err := os.ReadFile("./events.json")
	require.NoError(t, err)

	events := []database.Log{}
	err = json.Unmarshal(eventsFile, &events)
	require.NoError(t, err)

	in := make(chan []database.Log)
	out := make(chan *instructions.InstructionBase)

	ctx, cancel := context.WithCancel(context.Background())

	go instructions.Run(ctx, router, in, out)

	in <- events
	x := <-out

	require.Equal(t, 1, len(x.Signatures))

	cancel()
}
