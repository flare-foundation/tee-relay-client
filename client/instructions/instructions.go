package instructions

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/instruction"
	"github.com/flare-foundation/tee-relay-client/client/router"
)

// teeFilterer is only used for TeeInstructionSent logs parsing. Set in init().
var teeFilterer *teeinstructions.TeeInstructionsFilterer

// init sets the fdcFilterer
func init() {
	var err error

	teeFilterer, err = teeinstructions.NewTeeInstructionsFilterer(common.Address{}, nil)
	if err != nil {
		logger.Panic("cannot get fdc contract:", err)
	}
}

// ParseTeeInstructionsSent tries to parse ParseTeeInstructionsSent log as stored in the c-chain indexer database
func ParseTeeInstructionsSent(instruction database.Log) (*teeinstructions.TeeInstructionsTeeInstructionsSent, error) {
	chainLog, err := events.ConvertDatabaseLogToChainLog(instruction)
	if err != nil {
		return nil, err
	}

	return teeFilterer.ParseTeeInstructionsSent(*chainLog)
}

// Instruction allows instruction handling.
type Instruction interface {
	// Process prepares the instruction to be sent to Tees
	Process(router.Router) error
	// Dispatch instruction to sender
	Dispatch(chan<- *InstructionBase)
}

func ParseInstruction(inLog database.Log) (Instruction, error) {
	event, err := ParseTeeInstructionsSent(inLog)
	if err != nil {
		return nil, err
	}
	var ib InstructionBase
	ib.Event = event
	ib.EventToData()

	InClass := OPToClass[ib.Event.OpCommand]

	var in Instruction

	switch InClass {
	case Pl:
		in = &Plain{ib}
	case Aug:
		in = &Augment{ib}
	}

	return in, nil
}

func Handle(inLog database.Log, r router.Router, out chan<- *InstructionBase) error {
	instr, err := ParseInstruction(inLog)

	if err != nil {
		return err
	}

	go func() {
		err := instr.Process(r)

		if err != nil {
			return //TODO error handling
		}

		instr.Dispatch(out)
	}()

	return nil
}

type InstructionBase struct {
	Event       *teeinstructions.TeeInstructionsTeeInstructionsSent
	GeneralData instruction.Data // Data without TeeID
	Signatures  [][]byte
}

func (ib *InstructionBase) EventToData() {
	ib.GeneralData = instruction.Data{
		InstructionID:   ib.Event.InstructionId,
		RewardEpochID:   ib.Event.RewardEpochId,
		OPType:          ib.Event.OpType,
		OPCommand:       ib.Event.OpCommand,
		OriginalMessage: ib.Event.Message,
	}
}

func (ib *InstructionBase) Dispatch(iChan chan<- *InstructionBase) {
	iChan <- ib
}

// HashesForSigning prepares hashes of instruction data that are to be signed.
//
// Place of the hash corresponds to the place of TeeMachine in event.
func (ib *InstructionBase) hashesForSigning() ([]common.Hash, error) {
	data := ib.GeneralData

	hashes := make([]common.Hash, len(ib.Event.TeeMachines))
	var err error

	for j := range ib.Event.TeeMachines {
		data.TeeID = ib.Event.TeeMachines[j].TeeId
		hashes[j], err = data.HashForSigning()
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

func (ib *InstructionBase) sign(r router.Router) error {
	toSign, err := ib.hashesForSigning()
	if err != nil {
		return err
	}

	signatures, err := r.Sign(toSign)
	if err != nil {
		return err
	}
	ib.Signatures = signatures

	return nil
}

// PLACEHOLDER
func FetchSignatures([]common.Hash) ([][]byte, error) {
	return nil, nil
}
