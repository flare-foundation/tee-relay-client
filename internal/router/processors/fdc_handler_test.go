package processors

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// stubResponder is a fake verifier returning canned attestation results.
type stubResponder struct {
	body    []byte
	success bool
	err     error
}

func (s stubResponder) Response(context.Context, fdc2.IFdc2HubFdc2AttestationRequest) ([]byte, bool, error) {
	return s.body, s.success, s.err
}

func fdcRequest() (fdc2.IFdc2HubFdc2AttestationRequest, [64]byte) {
	attType := [32]byte(common.HexToHash("0x0aa1"))
	sourceID := [32]byte(common.HexToHash("0x0b22"))
	var ats [64]byte
	copy(ats[:32], attType[:])
	copy(ats[32:], sourceID[:])
	req := fdc2.IFdc2HubFdc2AttestationRequest{
		Header: fdc2.IFdc2HubFdc2RequestHeader{
			AttestationType: attType,
			SourceId:        sourceID,
			ThresholdBIPS:   5000,
			ProofOwner:      common.HexToAddress("0x99"),
		},
		RequestBody: []byte("request-body"),
	}
	return req, ats
}

func TestFDCHandlerHandle(t *testing.T) {
	t.Parallel()
	const (
		chainID     = uint64(14) // Flare: chain-bound digest from the first reward epoch
		unscheduled = uint64(16) // Coston: no breaking reward epoch set yet
		rewardEpoch = uint32(417)
		threshold   = uint64(1)
		timestamp   = uint64(1718113274)
	)
	opKey, operator := genKey(t)
	cosigners := []common.Address{common.HexToAddress("0xc1"), common.HexToAddress("0xc2")}

	req, ats := fdcRequest()
	msg, err := structs.Encode(fdc2.MessageArguments[op.Prove], req)
	require.NoError(t, err)

	buildIB := func() *instructions.Base {
		ib := signableBase(common.HexToAddress("0x1111111111111111111111111111111111111111"))
		ib.Event = &instructions.InstructionSentEvent{Cosigners: cosigners, CosignersThreshold: threshold}
		ib.GeneralData.OriginalMessage = msg
		ib.GeneralData.Timestamp = timestamp
		ib.GeneralData.RewardEpochID = rewardEpoch
		return ib
	}
	newHandlerOn := func(chain uint64, out chan *instructions.Base, r Responder) *FDCHandler {
		base := NewBase(chain, signer.NewLocal(opKey))
		base.SetOut(out)
		return &FDCHandler{Base: base, verifiers: map[[64]byte]Responder{ats: r}}
	}
	newHandler := func(out chan *instructions.Base, r Responder) *FDCHandler {
		return newHandlerOn(chainID, out, r)
	}

	t.Run("happy path signs the relay-prefixed hash", func(t *testing.T) {
		respBody := []byte("attestation-response")
		out := make(chan *instructions.Base, 1)
		ib := buildIB()
		require.NoError(t, newHandler(out, stubResponder{body: respBody, success: true}).Handle(context.Background(), ib))

		got := <-out
		require.Equal(t, hexutil.Bytes(respBody), got.GeneralData.AdditionalFixedMessage)
		require.Len(t, got.Signatures, 1)

		messageHash, _, err := fdc.HashMessage(chainID, req, respBody, cosigners, threshold, timestamp)
		require.NoError(t, err)
		want := relayPrefixedHash(chainID, rewardEpoch, messageHash)

		// the cosigner signature must recover the operator over the relay-prefixed hash...
		pub, err := crypto.SigToPub(accounts.TextHash(want[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.Equal(t, operator, crypto.PubkeyToAddress(*pub))

		// ...and NOT over the bare message hash (regression guard).
		bare, err := crypto.SigToPub(accounts.TextHash(messageHash[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.NotEqual(t, operator, crypto.PubkeyToAddress(*bare))

		// ...nor over the pre-cutover digest that omitted the chain id.
		unbound := fdc.RelayPrefixedHash(messageHash)
		old, err := crypto.SigToPub(accounts.TextHash(unbound[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.NotEqual(t, operator, crypto.PubkeyToAddress(*old))
	})

	t.Run("epoch before the boundary signs the pre-cutover digest", func(t *testing.T) {
		respBody := []byte("attestation-response")
		out := make(chan *instructions.Base, 1)
		require.NoError(t, newHandlerOn(unscheduled, out, stubResponder{body: respBody, success: true}).
			Handle(context.Background(), buildIB()))

		got := <-out
		messageHash, _, err := fdc.HashMessage(unscheduled, req, respBody, cosigners, threshold, timestamp)
		require.NoError(t, err)
		require.False(t, chainBoundDigest(unscheduled, rewardEpoch))

		want := fdc.RelayPrefixedHash(messageHash)
		pub, err := crypto.SigToPub(accounts.TextHash(want[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.Equal(t, operator, crypto.PubkeyToAddress(*pub))
	})

	t.Run("verifier rejects -> nothing emitted", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		require.NoError(t, newHandler(out, stubResponder{success: false}).Handle(context.Background(), buildIB()))
		require.Empty(t, out)
	})

	t.Run("verifier error", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		err := newHandler(out, stubResponder{err: errors.New("boom")}).Handle(context.Background(), buildIB())
		require.ErrorContains(t, err, "boom")
	})

	t.Run("no verifier for att type/source", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		base := NewBase(chainID, signer.NewLocal(opKey))
		base.SetOut(out)
		h := &FDCHandler{Base: base, verifiers: map[[64]byte]Responder{}}
		require.ErrorContains(t, h.Handle(context.Background(), buildIB()), "no verifier")
	})
}

func TestAttTypeAndSourceIDBase(t *testing.T) {
	t.Parallel()
	req, ats := fdcRequest()
	msg, err := structs.Encode(fdc2.MessageArguments[op.Prove], req)
	require.NoError(t, err)

	ib := &instructions.Base{}
	ib.GeneralData.OriginalMessage = msg

	got, err := AttTypeAndSourceIDBase(ib)
	require.NoError(t, err)
	require.Equal(t, ats, got)
}
