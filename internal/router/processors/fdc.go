package processors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/convert"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// FDC processes FDC2 instructions by adding them to a queue for the designated verifier.
type FDC struct {
	idToQueueName map[[64]byte]string
	queues        map[string]*FDCQueue
	handler       *FDCHandler
}

// NewFDC returns an FDC built from cfg and base; a nil cfg yields an FDC with no queues or verifiers.
func NewFDC(cfg *config.FDC, base *Base) (*FDC, error) {
	if cfg == nil {
		handler, err := NewFDCHandler(base, nil)
		if err != nil {
			return nil, fmt.Errorf("creating FDC handler: %w", err)
		}
		return &FDC{
			idToQueueName: make(map[[64]byte]string),
			queues:        map[string]*FDCQueue{},
			handler:       handler,
		}, nil
	}

	queues := make(map[string]*FDCQueue)

	handler, err := NewFDCHandler(base, cfg.Verifiers)
	if err != nil {
		return nil, fmt.Errorf("creating FDC handler: %w", err)
	}

	for name := range cfg.Queues {
		queues[name] = NewQueue(cfg.Queues[name], name)
	}

	idToQueueName := make(map[[64]byte]string)

	for _, v := range cfg.Verifiers {
		identifier, err := v.AttTypeAndSourceID()
		if err != nil {
			return nil, fmt.Errorf("invalid verifier (type %q, source %q, queue %q): %w", v.AttType, v.SourceID, v.QueueName, err)
		}
		idToQueueName[identifier] = v.QueueName

		_, exists := queues[v.QueueName]
		if !exists {
			return nil, fmt.Errorf("undefined queue %s for %s, %s", v.QueueName, v.AttType, v.SourceID)
		}
	}

	return &FDC{
		idToQueueName: idToQueueName,
		queues:        queues,
		handler:       handler,
	}, nil
}

// StartQueues initiates FDC queues to process the inputs with the set verifiers and pass the result
// to the base processor.
func (f *FDC) StartQueues(ctx context.Context, wg *sync.WaitGroup) {
	if len(f.idToQueueName) == 0 {
		logger.Warnf("no FDC verifiers configured — FDC2 PROVE instructions will fail")
	}

	for name, q := range f.queues {
		logger.Infof("started FDC queue %s", name)
		q.InitiateAndRun(ctx)
		q.ProcessOut(ctx, wg, f.handler)
	}

	for id, queueName := range f.idToQueueName {
		logger.Infof("FDC queue %s serves type %s, source %s", queueName, convert.CommonHashToString(common.BytesToHash(id[:32])), convert.CommonHashToString(common.BytesToHash(id[32:64])))
	}
}

// Process adds the instruction to the FDC queue with the current timestamp as weight.
func (f *FDC) Process(ctx context.Context, ib *instructions.Base) error {
	id, err := AttTypeAndSourceIDBase(ib)
	if err != nil {
		return fmt.Errorf("extracting attestation type and source ID: %w", err)
	}

	queueName, exits := f.idToQueueName[id]
	if !exits {
		attType, sourceID := atsStrings(id)
		return fmt.Errorf("no queue for: %s, %s", attType, sourceID)
	}

	q, exits := f.queues[queueName]
	if !exits { // should be impossible
		attType, sourceID := atsStrings(id)
		return fmt.Errorf("no queue %q for: %s, %s", queueName, attType, sourceID)
	}
	_, err = q.Add(ctx, ib, Weight{time.Now()})
	if err != nil {
		return fmt.Errorf("adding to queue %v: %w", queueName, err)
	}

	// depth is approximate: Add hands off to a channel and a goroutine pushes
	// to the heap asynchronously, so it may not yet count this item.
	logger.Debugf("queued instruction %s on %s (depth %d)", common.Hash(ib.Event.InstructionId).Hex(), queueName, q.Length())

	return nil
}
