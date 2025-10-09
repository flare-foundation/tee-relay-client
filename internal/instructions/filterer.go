package instructions

import (
	"context"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

type Filterer interface {
	//
	Filter(*InstructionSentEvent) bool // Filter returns true if the event should not be processed.
}

type ProviderFilterer struct{}

func (pf *ProviderFilterer) Filter(event *InstructionSentEvent) bool {
	return false
}

type CosignerFilterer struct{ Address common.Address }

func (cf *CosignerFilterer) Filter(event *InstructionSentEvent) bool {
	return !slices.Contains(event.Cosigners, cf.Address)
}

func NewFilterer(isCosigner bool, sgnr signer.Signer) (Filterer, error) {
	if !isCosigner {
		return &ProviderFilterer{}, nil
	}

	id, err := sgnr.Identify(context.Background())
	if err != nil {
		return nil, err
	}

	pubkey, err := types.ParsePubKey(id)
	if err != nil {
		return nil, err
	}

	address := crypto.PubkeyToAddress(*pubkey)

	return &CosignerFilterer{Address: address}, err
}
