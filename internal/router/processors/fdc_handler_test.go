package processors

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// stubResponder is a fake verifier returning canned attestation results.
type stubResponder struct {
	res VerifierResponse
	err error
}

func (s stubResponder) Response(context.Context, fdc2.IFdc2HubFdc2AttestationRequest) (VerifierResponse, error) {
	return s.res, s.err
}

// countingResponder counts Response calls and returns a canned response.
type countingResponder struct {
	calls *atomic.Int32
	res   VerifierResponse
}

func (c countingResponder) Response(context.Context, fdc2.IFdc2HubFdc2AttestationRequest) (VerifierResponse, error) {
	c.calls.Add(1)
	return c.res, nil
}

// emptySigner returns no signatures and no error, breaking the Signer contract.
type emptySigner struct{}

func (emptySigner) Sign(context.Context, []common.Hash) ([]hexutil.Bytes, error) { return nil, nil }
func (emptySigner) Decrypt(context.Context, []byte) (hexutil.Bytes, error)       { return nil, nil }
func (emptySigner) Identify(context.Context) (types.PublicKey, error) {
	return types.PublicKey{}, nil
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
		chainID   = uint64(14)
		threshold = uint64(1)
		timestamp = uint64(1718113274)
	)
	opKey, operator := genKey(t)
	cosigners := []common.Address{common.HexToAddress("0xc1"), common.HexToAddress("0xc2")}

	req, ats := fdcRequest()
	msg, err := fdc.EncodeRequest(req)
	require.NoError(t, err)

	buildIB := func() *instructions.Base {
		ib := signableBase(common.HexToAddress("0x1111111111111111111111111111111111111111"))
		ib.Event = &instructions.InstructionSentEvent{Cosigners: cosigners, CosignersThreshold: threshold}
		ib.GeneralData.OriginalMessage = msg
		ib.GeneralData.Timestamp = timestamp
		return ib
	}
	newHandler := func(out chan *instructions.Base, r Responder) *FDCHandler {
		base := NewBase(chainID, signer.NewLocal(opKey))
		base.SetOut(out)
		return &FDCHandler{Base: base, verifiers: map[[64]byte]Responder{ats: r}}
	}

	t.Run("happy path signs the relay-prefixed hash", func(t *testing.T) {
		respBody := []byte("attestation-response")
		out := make(chan *instructions.Base, 1)
		ib := buildIB()
		require.NoError(t, newHandler(out, stubResponder{res: VerifierResponse{Status: StatusVerified, ResponseBody: respBody}}).Handle(context.Background(), ib))

		got := <-out
		require.Equal(t, hexutil.Bytes(respBody), got.GeneralData.AdditionalFixedMessage)
		require.Len(t, got.Signatures, 1)

		messageHash, _, err := fdc.HashMessage(chainID, req, respBody, cosigners, threshold, timestamp)
		require.NoError(t, err)
		want := chainBoundPreimageHash(t, chainID, messageHash)

		// the cosigner signature must recover the operator over the relay-prefixed hash...
		pub, err := crypto.SigToPub(accounts.TextHash(want[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.Equal(t, operator, crypto.PubkeyToAddress(*pub))

		// ...and NOT over the bare message hash (regression guard).
		bare, err := crypto.SigToPub(accounts.TextHash(messageHash[:]), got.GeneralData.AdditionalVariableMessage)
		require.NoError(t, err)
		require.NotEqual(t, operator, crypto.PubkeyToAddress(*bare))
	})

	t.Run("re-enqueued verified instruction skips the verifier", func(t *testing.T) {
		respBody := []byte("attestation-response")
		out := make(chan *instructions.Base, 1)
		calls := new(atomic.Int32)
		h := newHandler(out, countingResponder{calls: calls, res: VerifierResponse{Status: StatusVerified, ResponseBody: respBody}})

		ib := buildIB()
		ib.GeneralData.AdditionalFixedMessage = respBody // as a prior VERIFIED attempt leaves it
		require.NoError(t, h.Handle(context.Background(), ib))
		require.EqualValues(t, 0, calls.Load())

		got := <-out
		require.Equal(t, hexutil.Bytes(respBody), got.GeneralData.AdditionalFixedMessage)
		require.Len(t, got.Signatures, 1)
	})

	t.Run("verifier rejects -> nothing emitted", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		require.NoError(t, newHandler(out, stubResponder{res: VerifierResponse{Status: StatusRejected, Message: "unsupported"}}).Handle(context.Background(), buildIB()))
		require.Empty(t, out)
	})

	t.Run("verifier retry -> error for the queue, nothing emitted", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		err := newHandler(out, stubResponder{res: VerifierResponse{Status: StatusRetry, Message: "round not finalized"}}).Handle(context.Background(), buildIB())
		require.ErrorContains(t, err, "RETRY")
		require.ErrorContains(t, err, "round not finalized")
		requireErrorNamesVerifier(t, err, ats)
		require.Empty(t, out)
	})

	t.Run("verifier returns unknown status -> error, nothing emitted", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		err := newHandler(out, stubResponder{res: VerifierResponse{Status: "BOGUS"}}).Handle(context.Background(), buildIB())
		require.ErrorIs(t, err, ErrUnknownStatus)
		require.Empty(t, out)
	})

	t.Run("verifier error", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		err := newHandler(out, stubResponder{err: errors.New("boom")}).Handle(context.Background(), buildIB())
		require.ErrorContains(t, err, "boom")
		requireErrorNamesVerifier(t, err, ats)
	})

	t.Run("signer breaking the signature-count contract", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		base := NewBase(chainID, emptySigner{})
		base.SetOut(out)
		h := &FDCHandler{Base: base, verifiers: map[[64]byte]Responder{ats: stubResponder{res: VerifierResponse{Status: StatusVerified, ResponseBody: []byte{0x01}}}}}
		err := h.Handle(context.Background(), buildIB())
		require.ErrorContains(t, err, "expected 1 signature")
		require.Empty(t, out)
	})

	t.Run("ctx cancelled while emitting", func(t *testing.T) {
		out := make(chan *instructions.Base) // unbuffered: the send can never proceed
		h := newHandler(out, stubResponder{res: VerifierResponse{Status: StatusVerified, ResponseBody: []byte{0x01}}})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.ErrorIs(t, h.Handle(ctx, buildIB()), context.Canceled)
	})

	t.Run("no verifier for att type/source", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		base := NewBase(chainID, signer.NewLocal(opKey))
		base.SetOut(out)
		h := &FDCHandler{Base: base, verifiers: map[[64]byte]Responder{}}
		require.ErrorContains(t, h.Handle(context.Background(), buildIB()), "no verifier")
	})
}

// requireErrorNamesVerifier asserts err carries the verifier's type and source strings.
func requireErrorNamesVerifier(t *testing.T, err error, ats [64]byte) {
	t.Helper()
	attType, sourceID := atsStrings(ats)
	require.ErrorContains(t, err, attType)
	require.ErrorContains(t, err, sourceID)
}

func TestAttTypeAndSourceIDBase(t *testing.T) {
	t.Parallel()
	req, ats := fdcRequest()
	msg, err := fdc.EncodeRequest(req)
	require.NoError(t, err)

	ib := &instructions.Base{}
	ib.GeneralData.OriginalMessage = msg

	got, err := AttTypeAndSourceIDBase(ib)
	require.NoError(t, err)
	require.Equal(t, ats, got)
}
