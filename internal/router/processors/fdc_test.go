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
