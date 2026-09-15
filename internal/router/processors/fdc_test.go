package processors

import (
	"context"
	"testing"
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestNewFDC(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	base := NewBase(14, config.RelayCutover{}, signer.NewLocal(key))

	t.Run("nil cfg yields an empty FDC", func(t *testing.T) {
		f, err := NewFDC(nil, base)
		require.NoError(t, err)
		require.NotNil(t, f)
	})

	t.Run("verifier referencing an undefined queue", func(t *testing.T) {
		cfg := &config.FDC{
			Queues: map[string]priority.Params{},
			Verifiers: map[string]config.Verifier{
				"v": {
					AttType:   "TypeA",
					SourceID:  "SrcA",
					QueueName: "missing",
					Server:    config.Credentials{URL: "http://verifier"},
				},
			},
		}
		_, err := NewFDC(cfg, base)
		require.ErrorContains(t, err, "undefined queue")
	})

	t.Run("duplicate type and source", func(t *testing.T) {
		cfg := &config.FDC{
			Queues: map[string]priority.Params{"q": {MaxAttempts: 1}},
			Verifiers: map[string]config.Verifier{
				"a": {AttType: "TypeA", SourceID: "SrcA", QueueName: "q", Server: config.Credentials{URL: "http://a"}},
				"b": {AttType: "TypeA", SourceID: "SrcA", QueueName: "q", Server: config.Credentials{URL: "http://b"}},
			},
		}
		_, err := NewFDC(cfg, base)
		require.ErrorContains(t, err, "both serve")
		require.ErrorContains(t, err, "TypeA")
	})

	t.Run("verifier with empty type or source", func(t *testing.T) {
		cfg := &config.FDC{
			Queues: map[string]priority.Params{"q": {MaxAttempts: 1}},
			Verifiers: map[string]config.Verifier{
				"v": {AttType: "TypeA", QueueName: "q", Server: config.Credentials{URL: "http://v"}},
			},
		}
		_, err := NewFDC(cfg, base)
		require.ErrorContains(t, err, "empty type or source")
	})

	t.Run("verifier with empty server URL", func(t *testing.T) {
		cfg := &config.FDC{
			Queues: map[string]priority.Params{"q": {MaxAttempts: 1}},
			Verifiers: map[string]config.Verifier{
				"v": {AttType: "TypeA", SourceID: "SrcA", QueueName: "q"},
			},
		}
		_, err := NewFDC(cfg, base)
		require.ErrorContains(t, err, "URL not set")
	})

	t.Run("verifier with unnamed api key", func(t *testing.T) {
		cfg := &config.FDC{
			Queues: map[string]priority.Params{"q": {MaxAttempts: 1}},
			Verifiers: map[string]config.Verifier{
				"v": {AttType: "TypeA", SourceID: "SrcA", QueueName: "q", Server: config.Credentials{URL: "http://v", Key: "secret"}},
			},
		}
		_, err := NewFDC(cfg, base)
		require.ErrorContains(t, err, "unnamed api key")
	})
}

// proveInstruction returns an instruction whose request carries attType and sourceID.
func proveInstruction(t *testing.T, attType, sourceID string) *instructions.Base {
	t.Helper()
	ats, err := config.JoinAttTypeAndSourceID(attType, sourceID)
	require.NoError(t, err)

	req := fdc2.IFdc2HubFdc2AttestationRequest{
		Header: fdc2.IFdc2HubFdc2RequestHeader{
			AttestationType: [32]byte(ats[:32]),
			SourceId:        [32]byte(ats[32:]),
		},
		RequestBody: []byte("request-body"),
	}
	msg, err := structs.Encode(fdc2.MessageArguments[op.Prove], req)
	require.NoError(t, err)

	ib := &instructions.Base{Event: &instructions.InstructionSentEvent{InstructionId: [32]byte{0x01}}}
	ib.GeneralData.OriginalMessage = msg

	return ib
}

func TestFDCProcess(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	base := NewBase(14, config.RelayCutover{}, signer.NewLocal(key))

	newFDC := func(t *testing.T) *FDC {
		t.Helper()
		cfg := &config.FDC{
			Queues: map[string]priority.Params{"q": {MaxAttempts: 1}},
			Verifiers: map[string]config.Verifier{
				"v": {AttType: "TypeA", SourceID: "SrcA", QueueName: "q", Server: config.Credentials{URL: "http://verifier"}},
			},
		}
		f, err := NewFDC(cfg, base)
		require.NoError(t, err)
		return f
	}

	t.Run("queues a matching instruction", func(t *testing.T) {
		t.Parallel()
		f := newFDC(t)
		f.queues["q"].InitiateAndRun(t.Context())

		require.NoError(t, f.Process(t.Context(), proveInstruction(t, "TypeA", "SrcA")))
		require.Eventually(t, func() bool { return f.queues["q"].Length() == 1 }, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("no queue for unknown type and source", func(t *testing.T) {
		t.Parallel()
		f := newFDC(t)

		err := f.Process(t.Context(), proveInstruction(t, "TypeB", "SrcB"))
		require.ErrorContains(t, err, "no queue for")
		require.ErrorContains(t, err, "TypeB")
	})

	t.Run("cancelled ctx wraps the queue add error", func(t *testing.T) {
		t.Parallel()
		f := newFDC(t) // queue not running: Add blocks, so cancellation wins deterministically
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := f.Process(ctx, proveInstruction(t, "TypeA", "SrcA"))
		require.ErrorContains(t, err, "adding to queue")
	})
}
