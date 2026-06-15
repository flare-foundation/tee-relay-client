package processors

import (
	"testing"

	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestNewFDC(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	base := NewBase(14, signer.NewLocal(key))

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
}
