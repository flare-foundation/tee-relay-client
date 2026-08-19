package processors

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/convert"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// FDCHandler links to verifiers for F_FDC2 instructions.
type FDCHandler struct {
	*Base

	verifiers map[[64]byte]Responder
}

var _ Responder = &Verifier{}

// Responder provides attestation responses for attestation requests.
// Response returns the attestation response bytes, a success flag, and an error.
type Responder interface {
	Response(context.Context, fdc2.IFdc2HubFdc2AttestationRequest) ([]byte, bool, error)
}

// NewFDCHandler returns an FDCHandler that routes requests to the given verifiers.
func NewFDCHandler(base *Base, verifiers map[string]config.Verifier) (*FDCHandler, error) {
	logDigestSchedule(base.chainID)

	fdcHandler := &FDCHandler{
		Base:      base,
		verifiers: make(map[[64]byte]Responder),
	}

	for _, v := range verifiers {
		identifier, err := v.AttTypeAndSourceID()
		if err != nil {
			return nil, fmt.Errorf("invalid verifier (type %q, source %q, queue %q): %w", v.AttType, v.SourceID, v.QueueName, err)
		}

		fdcHandler.verifiers[identifier] = &Verifier{&v.Server}
	}

	return fdcHandler, nil
}

// Handle handles instruction base for opType F_FDC2 opCommand PROVE.
func (h *FDCHandler) Handle(ctx context.Context, ib *instructions.Base) error {
	fullRequest, err := structs.Decode[fdc2.IFdc2HubFdc2AttestationRequest](fdc2.MessageArguments[op.Prove], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding request: %w", err) // should never happen
	}

	ats, err := AttTypeAndSourceID(&fullRequest.Header)
	if err != nil {
		return fmt.Errorf("reading att type and source ID: %w", err) // should never happen
	}

	v, exists := h.verifiers[ats]
	if !exists {
		attType, sourceID := atsStrings(ats)
		return fmt.Errorf("no verifier for type: %s, source: %s", attType, sourceID)
	}

	start := time.Now()
	attResponse, success, err := v.Response(ctx, fullRequest)
	if !success {
		attType, sourceID := atsStrings(ats)

		if err != nil {
			// err can carry the verifier's response body; quote it so control
			// characters cannot forge log lines.
			logger.Debugf("verifier error for instruction %s (type %s, source %s): %s", common.Hash(ib.Event.InstructionId).Hex(), attType, sourceID, strconv.Quote(err.Error()))
			return fmt.Errorf("getting attestation response: %w", err)
		}

		logger.Debugf("verifier rejected request from instruction %s (type %s, source %s)", common.Hash(ib.Event.InstructionId).Hex(), attType, sourceID)

		return nil
	}

	logger.Debugf("verifier answered instruction %s in %s", common.Hash(ib.Event.InstructionId).Hex(), time.Since(start))

	ib.GeneralData.AdditionalFixedMessage = attResponse
	messageHash, _, err := fdc.HashMessage(h.chainID, fullRequest, attResponse, ib.Event.Cosigners, ib.Event.CosignersThreshold, ib.GeneralData.Timestamp)
	if err != nil {
		return fmt.Errorf("hashing fdc message: %w", err)
	}

	// The chain recovers signatures against the Relay Mode-2 digest of the reward
	// epoch's Relay, not the bare messageHash.
	hashToBeSigned := relayPrefixedHash(h.chainID, ib.GeneralData.RewardEpochID, messageHash)
	logger.Debugf("signing instruction %s response for reward epoch %d, chain-bound digest %t",
		common.Hash(ib.Event.InstructionId).Hex(), ib.GeneralData.RewardEpochID, chainBoundDigest(h.chainID, ib.GeneralData.RewardEpochID))
	signature, err := h.signer.Sign(ctx, []common.Hash{hashToBeSigned})
	if err != nil {
		return fmt.Errorf("signing response: %w", err)
	}

	ib.GeneralData.AdditionalVariableMessage = signature[0] // if err == nil, len(signature)=1

	err = ib.Sign(ctx, h.signer, h.chainID)
	if err != nil {
		return fmt.Errorf("signing instruction: %w", err)
	}

	select {
	case h.out <- ib:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AttTypeAndSourceIDBase can be used for FDC2 instructions and
// returns concatenated attestation type and source ID each 32 bytes
// for and encoded attestationRequest.
func AttTypeAndSourceIDBase(b *instructions.Base) ([64]byte, error) {
	fullRequest, err := structs.Decode[fdc2.IFdc2HubFdc2AttestationRequest](fdc2.MessageArguments[op.Prove], b.GeneralData.OriginalMessage)
	if err != nil {
		return [64]byte{}, fmt.Errorf("decoding fdc request: %w", err)
	}

	return AttTypeAndSourceID(&fullRequest.Header)
}

// AttTypeAndSourceID returns concatenated attestation type and source ID each 32 bytes
// for and encoded attestationRequest.
func AttTypeAndSourceID(header *fdc2.IFdc2HubFdc2RequestHeader) ([64]byte, error) {
	res := [64]byte{}

	copy(res[:32], header.AttestationType[:])
	copy(res[32:], header.SourceId[:])

	return res, nil
}

// atsStrings renders the attestation type and source ID halves of ats as text
// when the NUL-trimmed bytes are printable ASCII, as 0x-hex otherwise.
func atsStrings(ats [64]byte) (string, string) {
	return atsHalf(ats[0:32]), atsHalf(ats[32:64])
}

// atsHalf renders half as text when its NUL-trimmed bytes are printable
// ASCII, as 0x-hex otherwise.
func atsHalf(half []byte) string {
	hash := common.BytesToHash(half)

	s := convert.CommonHashToString(hash)
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return hash.Hex()
		}
	}

	return s
}
