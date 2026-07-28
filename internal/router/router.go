// Package router routes parsed instruction logs to their processors.
package router

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/router/processors"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

// InstructionClass selects which processor implementation handles an instruction.
type InstructionClass int

// InstructionClass values enumerate the supported instruction classes.
const (
	Invalid InstructionClass = iota
	Plain
	FDC
	Backup
)

// Router holds processors for instructions.
type Router struct {
	Filterer

	baseProcessor   *processors.Base
	fdcProcessor    *processors.FDC
	backupProcessor *processors.Backup
}

// NewRouter assembles Router from configs.
func NewRouter(signer signer.Signer, chainID uint64, fdcCfg *config.FDC, filterer Filterer, allowUnsafeURLs bool) (*Router, error) {
	r := new(Router)

	r.Filterer = filterer
	r.baseProcessor = processors.NewBase(chainID, signer)
	r.backupProcessor = processors.NewBackup(r.baseProcessor, allowUnsafeURLs)

	var err error
	r.fdcProcessor, err = processors.NewFDC(fdcCfg, r.baseProcessor)
	if err != nil {
		return nil, fmt.Errorf("creating FDC processor: %w", err)
	}

	return r, nil
}

// SetOut sets the output chan of the base processor.
func (r *Router) SetOut(out chan<- *instructions.Base) {
	r.baseProcessor.SetOut(out)
}

// StartQueues initiates FDC queues to process the inputs with the set verifiers and pass the result
// to the base processor.
func (r *Router) StartQueues(ctx context.Context, wg *sync.WaitGroup) {
	r.fdcProcessor.StartQueues(ctx, wg)
}

// Run starts a go routine in which events from in chanel are Handled and the results are passed to out channel.
func (router *Router) Run(ctx context.Context, wg *sync.WaitGroup, in <-chan []database.Log, out chan<- *instructions.Base) {
	router.SetOut(out)
	router.StartQueues(ctx, wg)

	wg.Go(func() {
		var instructionEvents []database.Log
		var ok bool

		for {
			select {
			case <-ctx.Done():
				logger.Infof("closing router Run: %v", ctx.Err())
				return
			case instructionEvents, ok = <-in:
				if !ok {
					logger.Infof("closing router Run: in channel closed")
					return
				}

				for j := range instructionEvents {
					err := router.Handle(ctx, instructionEvents[j])
					if err != nil {
						logger.Errorf("handling instruction %s: %v", topicHex(instructionEvents[j].Topic2), err)
					}
				}
			}
		}
	})
}

// Handle parses, processes instruction log.
func (r *Router) Handle(ctx context.Context, inLog database.Log) error {
	instr, err := instructions.ParseInstruction(inLog)
	if err != nil {
		return fmt.Errorf("parsing instruction: %w", err)
	}

	if r.Filterer != nil && r.Filter(instr.Event) {
		logger.Debugf("instruction %s filtered out", common.Hash(instr.Event.InstructionId).Hex())
		return nil
	}

	processor, err := r.Route(instr)
	if err != nil {
		return fmt.Errorf("no processor for %v: %w", common.Hash(instr.Event.InstructionId), err)
	}

	go func() {
		// recover so a panic on one instruction cannot crash the relay
		defer func() {
			if rec := recover(); rec != nil {
				logger.Errorf("recovered panic processing instruction %s: %v", topicHex(inLog.Topic2), rec)
			}
		}()

		err := processor.Process(ctx, instr)
		if err != nil {
			logger.Errorf("processing instruction %s: %v", topicHex(inLog.Topic2), err)
		}
	}()

	return nil
}

// Route returns the processor for the instruction base.
func (r *Router) Route(b *instructions.Base) (processors.Processor, error) {
	ic := instClass(b.Event.OpType, b.Event.OpCommand)
	switch ic {
	case Plain:
		return r.baseProcessor, nil
	case FDC: // currently only opCommand
		return r.fdcProcessor, nil
	case Backup:
		return r.backupProcessor, nil
	case Invalid: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	default: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	}
}

// topicHex renders a bare Topic2 hex string in the canonical 0x-prefixed form.
func topicHex(topic string) string {
	return "0x" + topic
}

func instClass(opType, opCommand common.Hash) InstructionClass {
	t := op.HashToOPType(opType)
	c := op.HashToOPCommand(opCommand)

	switch {
	case t == op.FDC2 && c == op.Prove:
		return FDC
	case t == op.Wallet && c == op.KeyDataProviderRestore:
		return Backup
	case t == op.Wallet && c == op.KeyDirectRestore:
		// Spliced with the source-side envelope by the Backup processor.
		return Backup
	case op.IsValid(t, c):
		return Plain
	default:
		return Invalid
	}
}
