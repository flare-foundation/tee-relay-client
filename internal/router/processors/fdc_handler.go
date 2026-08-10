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

// Responder provides attestation responses for attestation requests.
type Responder interface {
	Response(context.Context, fdc2.IFdc2HubFdc2AttestationRequest) (VerifierResponse, error)
}

// NewFDCHandler returns an FDCHandler that routes requests to the given verifiers.
func NewFDCHandler(base *Base, verifiers map[string]config.Verifier) (*FDCHandler, error) {
	logDigestSchedule(base.chainID, base.cutover)

	fdcHandler := &FDCHandler{
		Base:      base,
		verifiers: make(map[[64]byte]Responder),
	}

	seen := make(map[[64]byte]string)

	for name, v := range verifiers {
		// a blank field still hashes to a valid identifier that no instruction ever matches
		if v.AttType == "" || v.SourceID == "" {
			return nil, fmt.Errorf("verifier %q: empty type or source (type %q, source %q)", name, v.AttType, v.SourceID)
		}

		identifier, err := v.AttTypeAndSourceID()
		if err != nil {
			return nil, fmt.Errorf("invalid verifier (type %q, source %q, queue %q): %w", v.AttType, v.SourceID, v.QueueName, err)
		}

		// a duplicate would be overwritten here and in NewFDC's queue map independently,
		// so the winning server and queue could come from different entries
		if prev, exists := seen[identifier]; exists {
			return nil, fmt.Errorf("verifiers %q and %q both serve type %q, source %q", prev, name, v.AttType, v.SourceID)
		}
		seen[identifier] = name

		if err := v.Server.Check(); err != nil {
			return nil, fmt.Errorf("invalid verifier server (type %q, source %q, queue %q): %w", v.AttType, v.SourceID, v.QueueName, err)
		}

		fdcHandler.verifiers[identifier] = NewVerifier(&v.Server)
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

	attType, sourceID := atsStrings(ats)

	v, exists := h.verifiers[ats]
	if !exists {
		return fmt.Errorf("no verifier for type: %s, source: %s", attType, sourceID)
	}

	// A re-enqueued instruction that already verified skips the verifier query:
	// the queue re-pushes the same instance, and only a VERIFIED response sets
	// AdditionalFixedMessage — so a retry after a signing/emit failure does not
	// pay another verifier round-trip.
	if len(ib.GeneralData.AdditionalFixedMessage) == 0 {
		start := time.Now()
		res, err := v.Response(ctx, fullRequest)
		if err != nil {
			// type/source in the error identify the verifier in queue logs, where shared queues obscure it
			return fmt.Errorf("getting attestation response (type %s, source %s): %w", attType, sourceID, err)
		}

		switch res.Status {
		case StatusVerified:
		case StatusRetry:
			// error return re-enqueues the instruction: the queue retries after time_off, up to max_attempts
			return fmt.Errorf("verifier (type %s, source %s) status RETRY: %s", attType, sourceID, res.Message)
		case StatusRejected:
			logger.Debugf("verifier rejected request from instruction %s (type %s, source %s): %s", common.Hash(ib.Event.InstructionId).Hex(), attType, sourceID, strconv.Quote(res.Message))

			return nil
		default: // unreachable: Response validates the status
			return fmt.Errorf("%w: %q", ErrUnknownStatus, res.Status)
		}

		logger.Debugf("verifier answered instruction %s in %s", common.Hash(ib.Event.InstructionId).Hex(), time.Since(start))

		ib.GeneralData.AdditionalFixedMessage = res.ResponseBody
	} else {
		logger.Debugf("reusing verified response for instruction %s", common.Hash(ib.Event.InstructionId).Hex())
	}

	messageHash, _, err := fdc.HashMessage(h.chainID, fullRequest, ib.GeneralData.AdditionalFixedMessage, ib.Event.Cosigners, ib.Event.CosignersThreshold, ib.GeneralData.Timestamp)
	if err != nil {
		return fmt.Errorf("hashing fdc message: %w", err)
	}

	// The chain recovers signatures against the Relay Mode-2 digest of the reward
	// epoch's Relay, not the bare messageHash.
	chainBound := h.cutover.ChainBound(ib.GeneralData.RewardEpochID)
	hashToBeSigned := relayPrefixedHash(h.chainID, chainBound, messageHash)
	logger.Debugf("signing instruction %s response for reward epoch %d, chain-bound digest %t",
		common.Hash(ib.Event.InstructionId).Hex(), ib.GeneralData.RewardEpochID, chainBound)
	signature, err := h.signer.Sign(ctx, []common.Hash{hashToBeSigned})
	if err != nil {
		return fmt.Errorf("signing response: %w", err)
	}

	if len(signature) != 1 {
		// Signer is an exported interface — enforce the one-hash/one-signature contract
		return fmt.Errorf("expected 1 signature, got %d", len(signature))
	}
	ib.GeneralData.AdditionalVariableMessage = signature[0]

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
