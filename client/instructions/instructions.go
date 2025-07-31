package instructions

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeextensionregistry"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
)

// teeFilterer is only used for TeeInstructionSent logs parsing. Set in init().
var teeFilterer *teeextensionregistry.TeeExtensionRegistryFilterer

// init sets the fdcFilterer.
func init() {
	var err error

	teeFilterer, err = teeextensionregistry.NewTeeExtensionRegistryFilterer(common.Address{}, nil)
	if err != nil {
		logger.Panic("cannot get tee instructions filterer:", err)
	}
}

// Run starts a go routine in which events from in chanel are Handled and the results are passed to out channel.
func Run(ctx context.Context, router *Router, in <-chan []database.Log, out chan<- *Base) {
	router.Start(ctx, out)

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
					err := Handle(ctx, instructionEvents[j], router)
					if err != nil {
						logger.Errorf("error handling instruction %s: %v", instructionEvents[j].Topic1, err)
					}
				}
			}
		}
	}()
}

// parseTeeInstructionsSent tries to parse parseTeeInstructionsSent log as stored in the c-chain indexer database.
func parseTeeInstructionsSent(i database.Log) (*teeextensionregistry.TeeExtensionRegistryTeeInstructionsSent, error) {
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
func ParseInstruction(il database.Log) (*Base, error) {
	event, err := parseTeeInstructionsSent(il)
	if err != nil {
		return nil, err
	}
	var ib Base
	ib.Event = event
	ib.EventToData(il.Timestamp)

	logger.Debugf("received instruction: %s, with ts %d at %d", common.Hash(ib.GeneralData.InstructionId), ib.GeneralData.Timestamp, time.Now().Unix())
	return &ib, nil
}

// Handle parses, processes instruction log.
func Handle(ctx context.Context, inLog database.Log, r *Router) error {
	instr, err := ParseInstruction(inLog)
	if err != nil {
		return err
	}

	processor, err := r.Route(instr)
	if err != nil {
		return fmt.Errorf("no processor for %v: %v", instr.Event.InstructionId, err)
	}

	go func() {
		err := processor.Process(ctx, instr)
		if err != nil {
			logger.Errorf("processing instruction %s, %v", inLog.Topic1, err)
		}
	}()

	return nil
}

type Base struct {
	Event       *teeextensionregistry.TeeExtensionRegistryTeeInstructionsSent
	GeneralData instruction.Data // Data without TeeID
	Signatures  []hexutil.Bytes
}

// EventToData copies relevant fields from Event to GeneralData. Timestamp should be recovered from the block.
//
// TeeID has to be set later when preparing the instruction for specific Tee.
// AdditionalFixedMessage and AdditionalVariableMessage are potentially set during processing.
func (ib *Base) EventToData(timestamp uint64) {
	ib.GeneralData = instruction.Data{
		DataFixed: instruction.DataFixed{
			InstructionId:   ib.Event.InstructionId,
			Timestamp:       timestamp,
			RewardEpochId:   ib.Event.RewardEpochId,
			OpType:          ib.Event.OpType,
			OpCommand:       ib.Event.OpCommand,
			OriginalMessage: ib.Event.Message,
		},
		AdditionalVariableMessage: hexutil.Bytes{},
	}
}

// HashesForSigning prepares hashes of instruction data that are to be signed.
//
// Place of the hash corresponds to the place of TeeMachine in event.
func (ib *Base) hashesForSigning() ([]common.Hash, error) {
	data := ib.GeneralData

	hashes := make([]common.Hash, len(ib.Event.TeeMachines))
	var err error

	for j := range ib.Event.TeeMachines {
		data.TeeId = ib.Event.TeeMachines[j].TeeId
		hashes[j], err = data.HashForSigning()
		if err != nil {
			return nil, fmt.Errorf("hash of %v; %v", data, err)
		}
	}
	return hashes, nil
}

// sign sets signatures of instructions for each Tee.
func (ib *Base) Sign(ctx context.Context, s *Signer) error {
	logger.Debugf("sending %v to sign", ib.GeneralData.InstructionId)

	toSign, err := ib.hashesForSigning()
	if err != nil {
		return fmt.Errorf("preparing: %v", err)
	}

	signatures, err := s.FetchSignatures(ctx, toSign)
	if err != nil {
		return fmt.Errorf("getting signatures for %v: %v", ib.GeneralData, err)
	}
	ib.Signatures = signatures

	return nil
}
