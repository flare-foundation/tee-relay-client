package processors

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

func TestAtsStrings(t *testing.T) {
	t.Parallel()

	asciiAts, err := config.JoinAttTypeAndSourceID("TeeAvailabilityCheck", "TEE")
	require.NoError(t, err)

	binaryAts := [64]byte{}
	binaryAts[0] = 0x01
	wantBinaryHex := common.BytesToHash(binaryAts[0:32]).Hex()

	interiorNULAts := [64]byte{}
	copy(interiorNULAts[0:32], "A\x00B")
	wantInteriorNULHex := common.BytesToHash(interiorNULAts[0:32]).Hex()

	tests := []struct {
		name         string
		ats          [64]byte
		wantType     string
		wantSourceID string
	}{
		{
			name:         "ASCII NUL-padded halves",
			ats:          asciiAts,
			wantType:     "TeeAvailabilityCheck",
			wantSourceID: "TEE",
		},
		{
			name:     "non-printable non-NUL byte renders as hex",
			ats:      binaryAts,
			wantType: wantBinaryHex,
			// bytes 32:64 are all-zero: NUL-trims to empty, which is
			// vacuously printable, so the source half stays plain text.
			wantSourceID: "",
		},
		{
			name:     "interior NUL renders as hex",
			ats:      interiorNULAts,
			wantType: wantInteriorNULHex,
			// bytes 32:64 are all-zero: NUL-trims to empty, which is
			// vacuously printable, so the source half stays plain text.
			wantSourceID: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			attType, sourceID := atsStrings(test.ats)
			require.Equal(t, test.wantType, attType)
			require.Equal(t, test.wantSourceID, sourceID)
		})
	}
}

// invalidVerifier has an AttType over 32 bytes, making JoinAttTypeAndSourceID fail.
func invalidVerifier() config.Verifier {
	return config.Verifier{
		AttType:   strings.Repeat("a", 33),
		SourceID:  "SrcA",
		QueueName: "queueA",
		Server:    config.Credentials{URL: "http://verifier", KeyName: "k", Key: "supersecret"},
	}
}

func TestNewFDCHandlerInvalidVerifierDoesNotLeakKey(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	base := NewBase(14, signer.NewLocal(key))

	_, err := NewFDCHandler(base, map[string]config.Verifier{"v": invalidVerifier()})
	require.ErrorContains(t, err, "queueA")
	require.NotContains(t, err.Error(), "supersecret")
}

func TestNewFDCInvalidVerifierDoesNotLeakKey(t *testing.T) {
	t.Parallel()
	key, _ := genKey(t)
	base := NewBase(14, signer.NewLocal(key))

	cfg := &config.FDC{
		Queues:    map[string]priority.Params{},
		Verifiers: map[string]config.Verifier{"v": invalidVerifier()},
	}

	_, err := NewFDC(cfg, base)
	require.ErrorContains(t, err, "queueA")
	require.NotContains(t, err.Error(), "supersecret")
}
