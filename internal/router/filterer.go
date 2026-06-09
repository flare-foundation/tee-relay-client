package router

import (
	"context"
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

// Filterer decides whether an InstructionSent event should be processed.
type Filterer interface {
	Filter(*instructions.InstructionSentEvent) bool // Filter returns true if the event should not be processed.
}

// ProviderFilterer implements Filterer that lets through all the InstructionSent events.
type ProviderFilterer struct{}

var _ Filterer = &ProviderFilterer{}

// Filter always returns false, so every event is processed.
func (pf *ProviderFilterer) Filter(event *instructions.InstructionSentEvent) bool {
	return false
}

// CosignerFilterer implements Filterer that lets through only the InstructionSent events that have the signer's address as a cosigner.
type CosignerFilterer struct{ Address common.Address }

var _ Filterer = &CosignerFilterer{}

// Filter returns true for events whose cosigners do not include the configured address.
func (cf *CosignerFilterer) Filter(event *instructions.InstructionSentEvent) bool {
	return !slices.Contains(event.Cosigners, cf.Address)
}

// NewFilterer returns ProviderFilterer if isCosigner is false or CosignerFilterer else.
// If isCosigner is false, sgnr is never used and can be nil.
func NewFilterer(isCosigner bool, sgnr signer.Signer) (Filterer, error) {
	if !isCosigner {
		return &ProviderFilterer{}, nil
	}

	id, err := sgnr.Identify(context.Background())
	if err != nil {
		return nil, fmt.Errorf("identifying signer: %w", err)
	}

	pubkey, err := types.ParsePubKey(id)
	if err != nil {
		return nil, fmt.Errorf("parsing signer public key: %w", err)
	}

	address := crypto.PubkeyToAddress(*pubkey)

	return &CosignerFilterer{Address: address}, nil
}
