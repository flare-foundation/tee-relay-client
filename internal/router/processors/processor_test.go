package processors

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestBaseProcess(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	tee := common.HexToAddress("0x1111111111111111111111111111111111111111")

	t.Run("out channel not set", func(t *testing.T) {
		b := NewBase(14, config.RelayCutover{}, signer.NewLocal(key))
		require.ErrorContains(t, b.Process(context.Background(), signableBase(tee)), "out channel not set")
	})

	t.Run("signs and emits", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		b := NewBase(14, config.RelayCutover{}, signer.NewLocal(key))
		b.SetOut(out)

		ib := signableBase(tee)
		require.NoError(t, b.Process(context.Background(), ib))

		got := <-out
		require.Same(t, ib, got)
		require.Len(t, got.Signatures, len(got.Tees))
	})

	t.Run("context canceled while emitting", func(t *testing.T) {
		out := make(chan *instructions.Base) // unbuffered, never read
		b := NewBase(14, config.RelayCutover{}, signer.NewLocal(key))
		b.SetOut(out)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, b.Process(ctx, signableBase(tee)), context.Canceled)
	})
}
