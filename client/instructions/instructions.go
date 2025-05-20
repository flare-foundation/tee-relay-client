package instructions

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/client/router"
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

// Instruction allows instruction handling.
type Instruction interface {
	// Process prepares the instruction to be sent to Tees
	Process(context.Context, *router.Router) error
	// Dispatch instruction to sender
	Dispatch(chan<- *InstructionBase)
}

// Run starts a go routine in which events from in chanel are Handled and the results are passed to out channel.
func Run(ctx context.Context, router *router.Router, in <-chan []database.Log, out chan<- *InstructionBase) {
	go func() {
		var instructionEvents []database.Log
		var ok bool

		for {
			select {
			case <-ctx.Done():
				logger.Infof("closing instructions Run: %v", ctx.Err())
				return
			case instructionEvents, ok = <-in:
				if !ok {
					logger.Infof("closing instructions Run: in channel closed")
					return
				}

				for j := range instructionEvents {
					err := Handle(ctx, instructionEvents[j], router, out)
					if err != nil {
						logger.Errorf("error handling instruction %s: %v", instructionEvents[j].Topic1, err)
					}
				}
			}
		}
	}()
}

// parseTeeInstructionsSent tries to parse parseTeeInstructionsSent log as stored in the c-chain indexer database.
func parseTeeInstructionsSent(i database.Log) (*teeinstructions.TeeInstructionsTeeInstructionsSent, error) {
	cl, err := events.ConvertDatabaseLogToChainLog(i)
	if err != nil {
		return nil, fmt.Errorf("converting instruction db log %v: %v", i, err)
	}

	is, err := teeFilterer.ParseTeeInstructionsSent(*cl)
	if err != nil {
		return nil, fmt.Errorf("parsing instruction %v: %v", i, err)
	}

	return is, nil
}

// ParseInstruction transforms database log to a designated implementation of Instruction interface.
func ParseInstruction(il database.Log) (Instruction, error) {
	event, err := parseTeeInstructionsSent(il)
	if err != nil {
		return nil, err
	}
	var ib InstructionBase
	ib.Event = event
	ib.EventToData(uint32(il.Timestamp))

	logger.Debugf("received instruction: %s, with ts %d at %d", ib.GeneralData.InstructionID, ib.GeneralData.Timestamp, time.Now().Unix())

	InClass, exists := OPToInstClass[ib.Event.OpCommand]
	if !exists {
		return nil, fmt.Errorf("unsorted opCommand: %s for instruction %v", string(ib.Event.OpCommand[:]), il)
	}
	var in Instruction

	switch InClass {
	case Pl:
		in = &Plain{ib}
	case Aug:
		in = &Augment{ib}
	case AugNSign:
		in = &AugmentAndSign{ib}
	}

	return in, nil
}

// Handle parses, processes instruction log and passes it to out channel.
func Handle(ctx context.Context, inLog database.Log, r *router.Router, out chan<- *InstructionBase) error {
	instr, err := ParseInstruction(inLog)
	if err != nil {
		return err
	}

	go func() {
		err := instr.Process(ctx, r)
		if err != nil {
			logger.Errorf("processing instruction %s, %v", inLog.Topic1, err)
			return
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
		DataFixed: instruction.DataFixed{
			InstructionID:   ib.Event.InstructionId,
			Timestamp:       timestamp,
			RewardEpochID:   ib.Event.RewardEpochId,
			OPType:          ib.Event.OpType,
			OPCommand:       ib.Event.OpCommand,
			OriginalMessage: ib.Event.Message,
		},
		AdditionalVariableMessage: hexutil.Bytes{},
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
			return nil, fmt.Errorf("hash of %v; %v", data, err)
		}
	}
	return hashes, nil
}

// sign sets signatures of instructions for each Tee.
func (ib *InstructionBase) sign(ctx context.Context, r *router.Router) error {
	logger.Debugf("sending %v to sign", ib.GeneralData.InstructionID)

	toSign, err := ib.hashesForSigning()
	if err != nil {
		return fmt.Errorf("preparing: %v", err)
	}

	signatures, err := r.Sign(ctx, toSign)
	if err != nil {
		return fmt.Errorf("getting signatures for %v: %v", ib.GeneralData, err)
	}
	ib.Signatures = signatures

	return nil
}
