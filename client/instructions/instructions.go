package instructions

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
)

// teeFilterer is only used for TeeInstructionSent logs parsing. Set in init().
var teeFilterer *teeinstructions.TeeInstructionsFilterer

// init sets the fdcFilterer
func init() {
	var err error

	teeFilterer, err = teeinstructions.NewTeeInstructionsFilterer(common.Address{}, nil)
	if err != nil {
		logger.Panic("cannot get tee instructions filterer:", err)
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
	Process(Router) error
	// Dispatch instruction to sender
	Dispatch(chan<- *InstructionBase)
}

type Router interface {
	Sign([]common.Hash) ([]hexutil.Bytes, error)
	Augment(common.Hash, common.Hash, hexutil.Bytes) (hexutil.Bytes, hexutil.Bytes, error)
}

func Run(ctx context.Context, router Router, in <-chan []database.Log, out chan<- *InstructionBase) {
	var instructionEvents []database.Log

	for {
		select {
		case <-ctx.Done():
			// TODO
			return
		case instructionEvents = <-in:
			for j := range instructionEvents {
				err := Handle(instructionEvents[j], router, out)
				if err != nil {
					// TODO
					logger.Debugf("parsing :%v", err)
				}
			}
		}
	}
}

// ParseInstruction transforms database log to a designated implementation of Instruction interface.
func ParseInstruction(inLog database.Log) (Instruction, error) {
	event, err := ParseTeeInstructionsSent(inLog)
	if err != nil {
		return nil, err
	}
	var ib InstructionBase
	ib.Event = event
	ib.EventToData(uint32(inLog.Timestamp))

	InClass := OPToInstClass[ib.Event.OpCommand]

	var in Instruction

	switch InClass {
	case Pl:
		in = &Plain{ib}
	case Aug:
		in = &Augment{ib}
	}

	return in, nil
}

// Handle parses, processes instruction log and passes it to out channel.
func Handle(inLog database.Log, r Router, out chan<- *InstructionBase) error {
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
	Signatures  []hexutil.Bytes
}

// EventToData copies relevant fields from Event to GeneralData. Timestamp should be recovered from the block.
//
// TeeID has to be set later when preparing the instruction for specific Tee.
// AdditionalFixedMessage and AdditionalVariableMessage are potentially set during processing.
func (ib *InstructionBase) EventToData(timestamp uint32) {
	ib.GeneralData = instruction.Data{
		InstructionID:   ib.Event.InstructionId,
		Timestamp:       timestamp,
		RewardEpochID:   ib.Event.RewardEpochId,
		OPType:          ib.Event.OpType,
		OPCommand:       ib.Event.OpCommand,
		OriginalMessage: ib.Event.Message,
	}
}

// Dispatch adds ib to the channel.
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

// sign sets signatures of instructions for each Tee.
func (ib *InstructionBase) sign(r Router) error {
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
