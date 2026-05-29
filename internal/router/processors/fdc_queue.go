package processors

import (
	"context"
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
)

// Weight for ordering of the FDC queues.
//
// An item has higher priority if it has arrived earlier.
type Weight struct{ time.Time }

func (w Weight) Self() Weight {
	return w
}

// Less returns true if t is before w.
func (w Weight) Less(t Weight) bool {
	return t.Before(w.Time)
}

type FDCQueue struct {
	*priority.PriorityQueue[*instructions.Base, Weight]
}

// NewQueue creates a FDC queue.
func NewQueue(params priority.Params, name string) *FDCQueue {
	queue := priority.New[*instructions.Base, Weight](params, name)

	return &FDCQueue{queue}
}

type Handler interface {
	Handle(context.Context, *instructions.Base) error
}

// ProcessOut spawns a go routine that dequeues and handles dequeues items.
func (q *FDCQueue) ProcessOut(ctx context.Context, h Handler) {
	go func() {
		for {
			if err := ctx.Err(); err != nil {
				logger.Infof("processing out of queue %s stopped: %v", q.Name(), err)
				return
			}

			q.Dequeue(ctx, h.Handle, nil)
		}
	}()
}
