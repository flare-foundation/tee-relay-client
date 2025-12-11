package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// FTDC processes FTDC instructions by adding them to a queue for the designated verifier.
type FTDC struct {
	idToQueueName map[[64]byte]string
	queues        map[string]*FTDCQueue
	handler       *FTDCHandler
}

func NewFTDC(cfg *config.FTDC, base *Base) (*FTDC, error) {
	if cfg == nil {
		handler, err := NewFTDCHandler(base, nil)
		if err != nil {
			return nil, err
		}
		return &FTDC{
			idToQueueName: make(map[[64]byte]string),
			queues:        map[string]*FTDCQueue{},
			handler:       handler,
		}, nil
	}

	queues := make(map[string]*FTDCQueue)

	handler, err := NewFTDCHandler(base, cfg.Verifiers)
	if err != nil {
		return nil, err
	}

	for name := range cfg.Queues {
		queues[name] = NewQueue(cfg.Queues[name], name)
	}

	idToQueueName := make(map[[64]byte]string)

	for _, v := range cfg.Verifiers {
		identifier, err := v.AttTypeAndSourceID()
		if err != nil {
			return nil, fmt.Errorf("invalid verifier %v, %v", v, err)
		}
		idToQueueName[identifier] = v.QueueName

		_, exists := queues[v.QueueName]
		if !exists {
			return nil, fmt.Errorf("undefined queue %s for %s, %s", v.QueueName, v.AttType, v.SourceID)
		}
	}

	return &FTDC{
		idToQueueName: idToQueueName,
		queues:        queues,
		handler:       handler,
	}, nil
}

// StartQueues initiates FTDC queues to process the inputs with the set verifiers and pass the result
// to the base processor.
func (f *FTDC) StartQueues(ctx context.Context) {
	for _, q := range f.queues {
		q.InitiateAndRun(ctx)
		q.ProcessOut(ctx, f.handler)
	}
}

// Process adds the instruction to the FTDC queue with the current timestamp as weight.
func (f *FTDC) Process(ctx context.Context, ib *instructions.Base) error {
	id, err := AttTypeAndSourceIDBase(ib)
	if err != nil {
		return err
	}

	queueName, exits := f.idToQueueName[id]
	if !exits {
		return fmt.Errorf("no queue for: %v", id)
	}

	q, exits := f.queues[queueName]
	if !exits { // should be impossible
		return fmt.Errorf("no queue for: %v", id)
	}
	q.Add(ib, Weight{time.Now()})

	return nil
}
