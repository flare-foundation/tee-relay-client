package router

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/router/processors"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

// Instruction Class refers to an implementation of Instruction interface.
type InstructionClass int

const (
	Invalid InstructionClass = iota
	Plain
	FTDC
	Backup
)

// Router holds processors for instructions.
type Router struct {
	Filterer

	baseProcessor   *processors.Base
	ftdcProcessor   *processors.FTDC
	backupProcessor *processors.Backup
}

// NewRouter assembles Router from configs.
func NewRouter(signer signer.Signer, ftdcCfg *config.FTDC, filterer Filterer) (*Router, error) {
	r := new(Router)

	r.Filterer = filterer
	r.baseProcessor = processors.NewBase(signer)
	r.backupProcessor = processors.NewBackup(r.baseProcessor)

	var err error
	r.ftdcProcessor, err = processors.NewFTDC(ftdcCfg, r.baseProcessor)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// SetOut sets the output chan of the base processor.
func (r *Router) SetOut(out chan<- *instructions.Base) {
	r.baseProcessor.SetOut(out)
}

// StartQueues initiates FTDC queues to process the inputs with the set verifiers and pass the result
// to the base processor.
func (r *Router) StartQueues(ctx context.Context) {
	r.ftdcProcessor.StartQueues(ctx)
}

// Run starts a go routine in which events from in chanel are Handled and the results are passed to out channel.
func (router *Router) Run(ctx context.Context, in <-chan []database.Log, out chan<- *instructions.Base) {
	router.SetOut(out)
	router.StartQueues(ctx)

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
					err := router.Handle(ctx, instructionEvents[j])
					if err != nil {
						logger.Errorf("error handling instruction %s: %v", instructionEvents[j].Topic2, err)
					}
				}
			}
		}
	}()
}

// Handle parses, processes instruction log.
func (r *Router) Handle(ctx context.Context, inLog database.Log) error {
	instr, err := instructions.ParseInstruction(inLog)
	if err != nil {
		return err
	}

	if r.Filterer != nil && r.Filter(instr.Event) {
		return nil
	}

	processor, err := r.Route(instr)
	if err != nil {
		return fmt.Errorf("no processor for %v: %v", instr.Event.InstructionId, err)
	}

	go func() {
		err := processor.Process(ctx, instr)
		if err != nil {
			logger.Errorf("processing instruction %s, %v", inLog.Topic2, err)
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
	case FTDC: // currently only opCommand
		return r.ftdcProcessor, nil
	case Backup:
		return r.backupProcessor, nil
	case Invalid: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	default: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	}
}

func instClass(opType, opCommand common.Hash) InstructionClass {
	t := op.HashToOPType(opType)
	c := op.HashToOPCommand(opCommand)

	switch {
	case t == op.FTDC && c == op.Prove:
		return FTDC
	case t == op.Wallet && c == op.KeyDataProviderRestore:
		return Backup
	case op.IsValid(t, c):
		return Plain
	default:
		return Invalid
	}
}
