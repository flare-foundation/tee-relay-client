package instructions

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/tee-relay-client/client/router"
)

// Plain is a type of instruction that need no additional augmentation, just the signatures of the instruction.
//
// Plain implements Instruction interface.
type Plain struct {
	InstructionBase
}

// Process just adds signatures to p.
func (p *Plain) Process(ctx context.Context, r *router.Router) error {
	return p.sign(ctx, r)
}

// Augment is a type of instruction that needs additional augmentation before signing.
//
// Augment implements Instruction interface.
type Augment struct {
	InstructionBase
}

// Process augments p according to OPType and OPCommand and adds signatures.
func (p *Augment) Process(ctx context.Context, r *router.Router) error {
	fixed, variable, err := r.Augment(ctx, p.Event.OpType, p.Event.OpCommand, p.Event.Message)
	if err != nil {
		return err
	}

	p.GeneralData.AdditionalFixedMessage = fixed
	p.GeneralData.AdditionalVariableMessage = variable

	return p.sign(ctx, r)
}

// AugmentAndSign is a type of instruction that requires additional fixed message and signature of it in the variable message.
//
// AugmentAndSign implements Instruction interface.
type AugmentAndSign struct {
	InstructionBase
}

// Process augments p according to OPType and OPCommand and adds signatures.
func (p *AugmentAndSign) Process(ctx context.Context, r *router.Router) error {
	fixed, _, err := r.Augment(ctx, p.Event.OpType, p.Event.OpCommand, p.Event.Message)
	if err != nil {
		return err
	}

	p.GeneralData.AdditionalFixedMessage = fixed

	hashToBeSigned := crypto.Keccak256Hash(fixed)

	messageSignature, err := r.Sign(ctx, []common.Hash{hashToBeSigned})
	if err != nil {
		return err
	}

	p.GeneralData.AdditionalFixedMessage = messageSignature[0]

	return p.sign(ctx, r)
}
