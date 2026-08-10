package processors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/stretchr/testify/require"
)

// failingHandler always fails with a fixed error.
type failingHandler struct{ err error }

func (h failingHandler) Handle(context.Context, *instructions.Base) error {
	return h.err
}

// panickingHandler always panics.
type panickingHandler struct{}

func (panickingHandler) Handle(context.Context, *instructions.Base) error {
	panic("kaboom")
}

func TestQueuePanicRecovered(t *testing.T) {
	t.Parallel()

	q := NewQueue(priority.Params{MaxAttempts: 1}, "queueA")
	instrID := common.HexToHash("0x0badc0de")
	ib := &instructions.Base{Event: &instructions.InstructionSentEvent{InstructionId: instrID}}

	var err error
	require.NotPanics(t, func() { err = q.wrapHandle(panickingHandler{})(t.Context(), ib) })
	require.ErrorContains(t, err, "recovered panic")
	require.ErrorContains(t, err, "kaboom")
	require.ErrorContains(t, err, instrID.Hex())
}

func TestNewQueueErrorChanForced(t *testing.T) {
	t.Parallel()
	q := NewQueue(priority.Params{ErrorChan: false}, "queueA")
	require.NotNil(t, q.Errors)
}

func TestQueueTerminalErrorCarriesInstructionID(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	q := NewQueue(priority.Params{MaxAttempts: 1}, "queueA")
	q.InitiateAndRun(ctx)

	instrID := common.HexToHash("0xdeadbeef")
	ib := &instructions.Base{Event: &instructions.InstructionSentEvent{InstructionId: instrID}}

	_, err := q.Add(ctx, ib, Weight{time.Now()})
	require.NoError(t, err)

	sentinel := errors.New("boom")
	q.Dequeue(ctx, q.wrapHandle(failingHandler{err: sentinel}), nil)

	select {
	case drained := <-q.Errors:
		require.ErrorContains(t, drained, "boom")
		require.Contains(t, drained.Error(), instrID.Hex())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal error")
	}
}
