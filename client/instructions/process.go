package instructions

// Plain is a type of instruction that need no additional augmentation, just the signatures of the instruction.
//
// Plain implements Instruction interface.
type Plain struct {
	InstructionBase
}

// Process just adds signatures to p.
func (p *Plain) Process(r Router) error {
	return p.sign(r)
}

// Augment is a type of instruction that needs additional augmentation before signing.
//
// Augment implements Instruction interface.
type Augment struct {
	InstructionBase
}

// Process augments p according to OPType and OPCommand and adds signatures.
func (p *Augment) Process(r Router) error {
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
