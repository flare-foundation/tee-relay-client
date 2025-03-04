package instructions

import "github.com/flare-foundation/tee-relay-client/client/router"

// Plain is a type of instruction that need no additional augmentation, just the signatures of the instruction.
//
// Plain implements Instruction interface.
type Plain struct {
	InstructionBase
}

// Process just add signatures to p
func (p *Plain) Process(r router.Router) error {
	return p.sign(r)
}

// Plain is a type of instruction that needs additional augmentation before signing.
//
// Plain implements Instruction interface.
type Augment struct {
	InstructionBase
}

// Process
func (p *Augment) Process(r router.Router) error {
	fixed, variable, err := r.Augment(p.Event.OpType, p.Event.OpCommand, p.Event.Message)
	if err != nil {
		return err
	}

	p.GeneralData.AdditionalFixedMessage = fixed
	p.GeneralData.AdditionalVariableMessage = variable

	err = p.sign(r)
	if err != nil {
		return err
	}

	return nil
}
