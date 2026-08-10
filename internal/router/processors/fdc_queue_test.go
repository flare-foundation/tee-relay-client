package processors

import (
	"context"
	"errors"
	"sync"
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

// signalingHandler fails every call and signals that it ran.
type signalingHandler struct {
	err error
	ch  chan struct{}
}

func (h signalingHandler) Handle(context.Context, *instructions.Base) error {
	select {
	case h.ch <- struct{}{}:
	default:
	}
	return h.err
}

func TestProcessOut(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewQueue(priority.Params{MaxAttempts: 1}, "queueA")
	q.InitiateAndRun(ctx)

	// pre-filled terminal error: the drain goroutine must consume it
	q.Errors <- errors.New("pre-drop")

	handled := make(chan struct{}, 1)
	var wg sync.WaitGroup
	q.ProcessOut(ctx, &wg, signalingHandler{err: errors.New("boom"), ch: handled})

	require.Eventually(t, func() bool { return len(q.Errors) == 0 }, 2*time.Second, 10*time.Millisecond)

	ib := &instructions.Base{Event: &instructions.InstructionSentEvent{InstructionId: common.HexToHash("0xabc")}}
	_, err := q.Add(ctx, ib, Weight{time.Now()})
	require.NoError(t, err)

	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessOut never dispatched the instruction to the handler")
	}

	// the failed only attempt is dropped through the Errors channel and drained
	require.Eventually(t, func() bool { return len(q.Errors) == 0 }, 2*time.Second, 10*time.Millisecond)

	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessOut goroutines did not exit after ctx cancellation")
	}
}

// recordingHandler reports each handled instruction ID.
type recordingHandler struct{ ch chan common.Hash }

func (h recordingHandler) Handle(_ context.Context, ib *instructions.Base) error {
	h.ch <- common.Hash(ib.Event.InstructionId)
	return nil
}

// TestQueueDequeuesEarliestFirst pins the Weight.Less direction: it inverts the
// priority package's comparison, which inverts once more — earliest arrival pops first.
func TestQueueDequeuesEarliestFirst(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	q := NewQueue(priority.Params{MaxAttempts: 1}, "queueA")
	q.InitiateAndRun(ctx)

	base := time.Unix(1718113274, 0)
	arrival := []int{3, 0, 4, 1, 2} // shuffled enqueue order
	for _, off := range arrival {
		ib := &instructions.Base{Event: &instructions.InstructionSentEvent{InstructionId: common.BytesToHash([]byte{byte(off)})}}
		_, err := q.Add(ctx, ib, Weight{base.Add(time.Duration(off) * time.Minute)})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return q.Length() == len(arrival) }, 2*time.Second, 10*time.Millisecond)

	got := make(chan common.Hash, len(arrival))
	handle := q.wrapHandle(recordingHandler{ch: got})
	for i := range arrival {
		q.Dequeue(ctx, handle, nil)
		select {
		case id := <-got:
			require.Equal(t, common.BytesToHash([]byte{byte(i)}), id, "dequeue %d must be the earliest remaining arrival", i)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for dequeue")
		}
	}
}
