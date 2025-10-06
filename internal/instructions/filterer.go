package instructions

import (
	"slices"

	"github.com/ethereum/go-ethereum/common"
)

type Filterer interface {
	Filter(*InstructionSentEvent) bool
}

type ProviderFilterer struct{}

func (pf *ProviderFilterer) Filter(event *InstructionSentEvent) bool {
	return true
}

type CosignerFilterer struct{ Address common.Address }

func (cf *CosignerFilterer) Filter(event *InstructionSentEvent) bool {
	return slices.Contains(event.Cosigners, cf.Address)
}
