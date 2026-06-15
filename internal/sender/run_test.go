package sender

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/stretchr/testify/require"
)

func TestNewTransport(t *testing.T) {
	t.Parallel()
	// Compare by pointer identity, not value: deep-comparing the global
	// http.DefaultTransport races with concurrent HTTP use of it.
	require.Same(t, http.DefaultTransport, newTransport(true))
	require.NotSame(t, http.DefaultTransport, newTransport(false))
}

// TestRun feeds one instruction addressed to two TEEs and asserts both TEE
// endpoints receive a POST to /instruction.
func TestRun(t *testing.T) {
	t.Parallel()

	type hit struct{ path string }
	hits := make(chan hit, 2)
	teeServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits <- hit{path: r.URL.Path}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(SignedReceipt{})
		}))
	}
	srv1, srv2 := teeServer(), teeServer()
	defer srv1.Close()
	defer srv2.Close()

	ib := &instructions.Base{
		Tees: []teeinstructions.IMachineManagerTeeMachine{
			{TeeId: common.HexToAddress("0x1"), Url: srv1.URL},
			{TeeId: common.HexToAddress("0x2"), Url: srv2.URL},
		},
		Signatures: []hexutil.Bytes{{0x01}, {0x02}},
		GeneralData: instruction.Data{
			DataFixed: instruction.DataFixed{InstructionID: common.HexToHash("0xabc")},
		},
	}

	in := make(chan *instructions.Base, 1)
	Run(t.Context(), in, true) // allowUnsafeURLs: reach the httptest loopback
	in <- ib

	for range 2 {
		select {
		case h := <-hits:
			require.Equal(t, "/instruction", h.path)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for TEE delivery")
		}
	}
}
