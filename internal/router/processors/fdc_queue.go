package processors

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"go.uber.org/zap"
)

// Weight for ordering of the FDC queues.
//
// An item has higher priority if it has arrived earlier. Arrival is Add time,
// not on-chain order: instructions from one collector batch are stamped in
// scheduler order, microseconds apart.
type Weight struct{ time.Time }

// Self returns the Weight itself.
func (w Weight) Self() Weight {
	return w
}

// Less returns true if t is before w.
func (w Weight) Less(t Weight) bool {
	return t.Before(w.Time)
}

// FDCQueue is a priority queue of FDC instructions ordered by Weight.
type FDCQueue struct {
	*priority.PriorityQueue[*instructions.Base, Weight]
}

// NewQueue creates a FDC queue.
func NewQueue(params priority.Params, name string) *FDCQueue {
	// Terminal errors must always be drainable; see ProcessOut. params is a value copy.
	params.ErrorChan = true
	queue := priority.NewWithLogger[*instructions.Base, Weight](params, name, queueLogger())

	return &FDCQueue{queue}
}

// queueLogger corrects the global logger's caller skip for logs emitted inside the priority package.
func queueLogger() *zap.SugaredLogger {
	return logger.Logger().WithOptions(zap.AddCallerSkip(-1))
}

// Handler handles a dequeued instruction.
type Handler interface {
	Handle(context.Context, *instructions.Base) error
}

// wrapHandle logs when an instruction leaves the queue for handling, decorates a handler
// error with the instruction ID, and logs the failed attempt at Debug.
// The decorated error is what the queue retries and reports on the final attempt.
// A panic in the handler is recovered into such an error: the queue's worker
// goroutine has no recover of its own, so it would kill the whole relay.
func (q *FDCQueue) wrapHandle(h Handler) func(context.Context, *instructions.Base) error {
	return func(ctx context.Context, ib *instructions.Base) (err error) {
		// id is computed after the defer so even a panic there is recovered
		var id string

		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("instruction %s: recovered panic: %v", id, rec)
				logger.Errorf("queue %s: panic handling instruction %s: %v\n%s", q.Name(), id, rec, debug.Stack())
			}
		}()

		id = common.Hash(ib.Event.InstructionId).Hex()

		logger.Debugf("queue %s: handling instruction %s", q.Name(), id)
		err = h.Handle(ctx, ib)
		if err != nil {
			err = fmt.Errorf("instruction %s: %w", id, err)
			// quote: verifier errors can embed response bodies with control characters.
			logger.Debugf("queue %s: attempt failed: %s", q.Name(), strconv.Quote(err.Error()))
			return err
		}
		return nil
	}
}

// ProcessOut spawns a go routine that dequeues and handles dequeued items, and a
// go routine that logs instructions dropped after their final attempt. Both are
// registered on wg so a caller can wait for them to exit after cancelling ctx.
//
// ProcessOut must be called at most once per queue (StartQueues already does this).
func (q *FDCQueue) ProcessOut(ctx context.Context, wg *sync.WaitGroup, h Handler) {
	handle := q.wrapHandle(h)

	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case err := <-q.Errors:
				logger.Errorf("queue %s: dropping instruction after final attempt: %s", q.Name(), strconv.Quote(err.Error()))
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			if err := ctx.Err(); err != nil {
				logger.Infof("closing queue %s Run: %v (%d instructions still queued)", q.Name(), err, q.Length())
				return
			}

			q.Dequeue(ctx, handle, nil)
		}
	}()
}
