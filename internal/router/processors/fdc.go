package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/convert"
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
			return nil, fmt.Errorf("invalid verifier %v: %w", v, err)
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
func (f *FDC) StartQueues(ctx context.Context) {
	for _, q := range f.queues {
		q.InitiateAndRun(ctx)
		q.ProcessOut(ctx, f.handler)
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
		return fmt.Errorf("no queue for: %s, %s", convert.CommonHashToString(common.BytesToHash(id[:32])), convert.CommonHashToString(common.BytesToHash(id[32:64])))
	}

	q, exits := f.queues[queueName]
	if !exits { // should be impossible
		return fmt.Errorf("no queue %q for: %s, %s", queueName, convert.CommonHashToString(common.BytesToHash(id[:32])), convert.CommonHashToString(common.BytesToHash(id[32:64])))
	}
	_, err = q.Add(ctx, ib, Weight{time.Now()})
	if err != nil {
		return fmt.Errorf("adding to queue %v: %w", queueName, err)
	}

	return nil
}
