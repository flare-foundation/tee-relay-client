package processors

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
	"github.com/flare-foundation/tee-node/pkg/ftdc"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// FTDCHandler links to verifiers for FTDC instructions.
type FTDCHandler struct {
	*Base

	verifiers map[[64]byte]Responder
}

// Responder provides attestation responses for attestation requests.
// Response returns the attestation response bytes, a success flag, and an error.
type Responder interface {
	Response(context.Context, connector.IFtdcHubFtdcAttestationRequest) ([]byte, bool, error)
}

func NewFTDCHandler(base *Base, verifiers map[string]config.Verifier) (*FTDCHandler, error) {
	ftdcHandler := &FTDCHandler{
		Base:      base,
		verifiers: make(map[[64]byte]Responder),
	}

	for _, v := range verifiers {
		identifier, err := v.AttTypeAndSourceID()
		if err != nil {
			return nil, fmt.Errorf("invalid verifier %v, %v", v, err)
		}

		ftdcHandler.verifiers[identifier] = &Verifier{&v.Server}
	}

	return ftdcHandler, nil
}

// Handle handles instruction base for opType FTDC opCommand PROVE.
func (h *FTDCHandler) Handle(ctx context.Context, ib *instructions.Base) error {
	fullRequest, err := structs.Decode[connector.IFtdcHubFtdcAttestationRequest](connector.MessageArguments[op.Prove], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding request: %v", err) // should never happen
	}

	ats, err := AttTypeAndSourceID(&fullRequest.Header)
	if err != nil {
		return fmt.Errorf("reading reading att type and source ID: %v", err) // should never happen
	}

	v, exists := h.verifiers[ats]
	if !exists {
		return fmt.Errorf("no verifier for %v", ats)
	}

	attResponse, retry, err := v.Response(ctx, fullRequest)
	if err != nil {
		switch retry {
		case true:
			return fmt.Errorf("getting response: %v", err)
		case false:
			logger.Debugf("request %s failed %v, not retrying", ib.GeneralData.InstructionID.String(), err)
			return nil
		}
	}

	ib.GeneralData.AdditionalFixedMessage = attResponse
	hashToBeSigned, _, _, err := ftdc.HashMessage(fullRequest, attResponse, ib.Event.Cosigners, ib.Event.CosignersThreshold, ib.GeneralData.Timestamp)
	if err != nil {
		return fmt.Errorf("hashing ftdc message: %w", err)
	}

	signature, err := h.signer.Sign(ctx, []common.Hash{hashToBeSigned})
	if err != nil {
		return fmt.Errorf("signing response: %v", err)
	}

	ib.GeneralData.AdditionalVariableMessage = signature[0] // if err != nil, len(signature)=1

	err = ib.Sign(ctx, h.signer)
	if err != nil {
		return fmt.Errorf("signing instruction: %v", err)
	}

	select {
	case h.out <- ib:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AttTypeAndSourceIDBase can be used for FTDC instructions and
// returns concatenated attestation type and source ID each 32 bytes
// for and encoded attestationRequest.
func AttTypeAndSourceIDBase(b *instructions.Base) ([64]byte, error) {
	fullRequest, err := structs.Decode[connector.IFtdcHubFtdcAttestationRequest](connector.MessageArguments[op.Prove], b.GeneralData.OriginalMessage)
	if err != nil {
		return [64]byte{}, fmt.Errorf("decoding ftdc request: %v", err)
	}

	return AttTypeAndSourceID(&fullRequest.Header)
}

// AttTypeAndSourceID returns concatenated attestation type and source ID each 32 bytes
// for and encoded attestationRequest.
func AttTypeAndSourceID(header *connector.IFtdcHubFtdcRequestHeader) ([64]byte, error) {
	res := [64]byte{}

	copy(res[:32], header.AttestationType[:])
	copy(res[32:], header.SourceId[:])

	return res, nil
}
