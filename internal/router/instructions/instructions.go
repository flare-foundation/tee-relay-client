// Package instructions parses and signs TEE instruction events from the indexer database.
package instructions

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

// teeFilterer is only used for TeeInstructionSent logs parsing. Set in init().
var teeFilterer *teeinstructions.InstructionsFilterer

// InstructionSentEvent is the parsed TeeInstructionsSent log.
type InstructionSentEvent = teeinstructions.InstructionsTeeInstructionsSent

// Base holds a TEE instruction event together with its parsed data and signatures.
type Base struct {
	Event       *InstructionSentEvent
	Tees        []teeinstructions.IMachineManagerTeeMachine
	GeneralData instruction.Data // Data without TeeID
	Signatures  []hexutil.Bytes
}

// init sets the teeFilterer.
func init() {
	var err error

	teeFilterer, err = teeinstructions.NewInstructionsFilterer(common.Address{}, nil)
	if err != nil {
		panic("cannot get tee instructions filterer: " + err.Error())
	}
}

// parseTeeInstructionsSent tries to parse parseTeeInstructionsSent log as stored in the c-chain indexer database.
func parseTeeInstructionsSent(i database.Log) (*InstructionSentEvent, error) {
	cl, err := events.ConvertDatabaseLogToChainLog(i)
	if err != nil {
		return nil, fmt.Errorf("converting instruction db log 0x%s: %w", i.Topic2, err)
	}

	is, err := teeFilterer.ParseTeeInstructionsSent(*cl)
	if err != nil {
		return nil, fmt.Errorf("parsing instruction 0x%s: %w", i.Topic2, err)
	}

	return is, nil
}

// ParseInstruction transforms database log to instruction Base.
func ParseInstruction(il database.Log) (*Base, error) {
	event, err := parseTeeInstructionsSent(il)
	if err != nil {
		return nil, err
	}
	var ib Base
	ib.Event = event
	ib.EventToData(il.Timestamp)

	logger.Debugf("received instruction %s, block ts %d, at %d", ib.GeneralData.InstructionID, ib.GeneralData.Timestamp, time.Now().Unix())
	return &ib, nil
}

// EventToData copies relevant fields from Event to GeneralData. Timestamp should be recovered from the block.
//
// TeeID has to be set later when preparing the instruction for a specific Tee.
// AdditionalFixedMessage and AdditionalVariableMessage are potentially set during processing.
// A slice of tees without duplicates is made.
func (ib *Base) EventToData(timestamp uint64) {
	ib.GeneralData = instruction.Data{
		DataFixed: instruction.DataFixed{
			InstructionID:      ib.Event.InstructionId,
			Timestamp:          timestamp,
			RewardEpochID:      ib.Event.RewardEpochId,
			OPType:             ib.Event.OpType,
			OPCommand:          ib.Event.OpCommand,
			Cosigners:          ib.Event.Cosigners,
			CosignersThreshold: ib.Event.CosignersThreshold,
			OriginalMessage:    ib.Event.Message,
		},
		AdditionalVariableMessage: hexutil.Bytes{},
	}

	ib.Tees = removeDuplicates(ib.Event.TeeMachines)
}

// HashesForSigning prepares hashes of instruction data that are to be signed.
//
// Place of the hash corresponds to the place of TeeMachine in event where duplicates are removed.
func (ib *Base) hashesForSigning(chainID uint64) ([]common.Hash, error) {
	data := ib.GeneralData

	hashes := make([]common.Hash, len(ib.Tees))
	var err error

	for j := range ib.Tees {
		data.TeeID = ib.Tees[j].TeeId
		hashes[j], err = data.HashForSigning(chainID)
		if err != nil {
			return nil, fmt.Errorf("hashing instruction %s for tee %s: %w", data.InstructionID, data.TeeID, err)
		}
	}
	return hashes, nil
}

// Sign sets signatures of instructions for each Tee.
func (ib *Base) Sign(ctx context.Context, s signer.Signer, chainID uint64) error {
	logger.Debugf("sending instruction %s to sign", ib.GeneralData.InstructionID)

	toSign, err := ib.hashesForSigning(chainID)
	if err != nil {
		return fmt.Errorf("preparing: %w", err)
	}

	signatures, err := s.Sign(ctx, toSign)
	if err != nil {
		return fmt.Errorf("getting signatures for instruction %s: %w", ib.GeneralData.InstructionID, err)
	}
	ib.Signatures = signatures

	return nil
}

// removeDuplicates creates a new array from s without duplicated entries.
func removeDuplicates(s []teeinstructions.IMachineManagerTeeMachine) []teeinstructions.IMachineManagerTeeMachine {
	set := make(map[teeinstructions.IMachineManagerTeeMachine]bool)
	unique := make([]teeinstructions.IMachineManagerTeeMachine, 0, len(s))
	for j := range s {
		if !set[s[j]] {
			set[s[j]] = true
			unique = append(unique, s[j])
		}
	}

	return unique
}
