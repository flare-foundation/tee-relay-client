package router

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/internal/router/processors"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	r, err := NewRouter(signer.NewLocal(key), 14, &config.FDC{}, &ProviderFilterer{}, false)
	require.NoError(t, err)
	return r
}

func TestInstClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ot   op.Type
		oc   op.Command
		want InstructionClass
	}{
		{"fdc prove", op.FDC2, op.Prove, FDC},
		{"backup data provider", op.Wallet, op.KeyDataProviderRestore, Backup},
		{"backup direct", op.Wallet, op.KeyDirectRestore, Backup},
		{"plain xrp pay", op.XRP, op.Pay, Plain},
		{"plain key generate", op.Wallet, op.KeyGenerate, Plain},
		{"invalid pair", op.FDC2, op.Pay, Invalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, instClass(tt.ot.Hash(), tt.oc.Hash()))
		})
	}
}

func TestRoute(t *testing.T) {
	t.Parallel()
	r := newTestRouter(t)

	tests := []struct {
		name string
		ot   op.Type
		oc   op.Command
		want processors.Processor
	}{
		{"plain", op.XRP, op.Pay, &processors.Base{}},
		{"fdc", op.FDC2, op.Prove, &processors.FDC{}},
		{"backup", op.Wallet, op.KeyDirectRestore, &processors.Backup{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &instructions.Base{Event: &instructions.InstructionSentEvent{
				OpType:    tt.ot.Hash(),
				OpCommand: tt.oc.Hash(),
			}}
			p, err := r.Route(b)
			require.NoError(t, err)
			require.IsType(t, tt.want, p)
		})
	}

	t.Run("invalid", func(t *testing.T) {
		b := &instructions.Base{Event: &instructions.InstructionSentEvent{
			OpType:    op.FDC2.Hash(),
			OpCommand: op.Pay.Hash(),
		}}
		_, err := r.Route(b)
		require.Error(t, err)
	})
}

func TestHandleParseError(t *testing.T) {
	t.Parallel()
	r := newTestRouter(t)
	// An empty log cannot be parsed as a TeeInstructionsSent event.
	err := r.Handle(context.Background(), database.Log{})
	require.ErrorContains(t, err, "parsing instruction")
}
