package processors

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// TestProcessDataProviderRestore covers the error paths of the legacy
// data-provider restore that need no full wallet-backup fixture: the
// consistency gate, the non-200 path, and the malformed-blob guard that must
// return an error rather than panic the relay. (The ECIES happy path needs a
// fully-signed WalletBackup blob and is deferred to a dedicated fixture.)
func TestProcessDataProviderRestore(t *testing.T) {
	t.Parallel()
	const chainID = uint64(14)

	operatorKey, _ := genKey(t)
	destKey, destTEE := genKey(t)
	id, reqID := backupID()

	proxy := func(status int, body []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}))
	}

	build := func(url string) *instructions.Base {
		req := wallet.IWalletBackupManagerKeyDataProviderRestore{
			TeePublicKey: walletPubKey(destKey),
			BackupId:     reqID,
			BackupUrl:    url,
			Nonce:        big.NewInt(0),
		}
		msg, err := structs.Encode(wallet.MessageArguments[op.KeyDataProviderRestore], req)
		require.NoError(t, err)
		ib := signableBase(destTEE)
		ib.Event = &instructions.InstructionSentEvent{OpCommand: op.KeyDataProviderRestore.Hash()}
		ib.GeneralData.OriginalMessage = msg
		return ib
	}

	process := func(ib *instructions.Base) error {
		out := make(chan *instructions.Base, 1)
		base := NewBase(chainID, config.RelayCutover{}, signer.NewLocal(operatorKey))
		base.SetOut(out)
		return NewBackup(base, true).Process(context.Background(), ib)
	}

	response := func(backupID wallets.WalletBackupID, blob []byte) []byte {
		b, err := json.Marshal(wallets.TEEBackupResponse{BackupID: backupID, WalletBackup: blob})
		require.NoError(t, err)
		return b
	}

	t.Run("malformed backup blob is rejected without panicking", func(t *testing.T) {
		srv := proxy(http.StatusOK, response(id, []byte("{}")))
		defer srv.Close()
		require.ErrorContains(t, process(build(srv.URL)), "checking wallet backup")
	})

	t.Run("backup id inconsistent with request", func(t *testing.T) {
		mismatched := id
		mismatched.WalletID = common.HexToHash("0xdead")
		srv := proxy(http.StatusOK, response(mismatched, []byte("{}")))
		defer srv.Close()
		require.ErrorContains(t, process(build(srv.URL)), "inconsistent")
	})

	t.Run("non-200 backup response", func(t *testing.T) {
		srv := proxy(http.StatusInternalServerError, []byte("nope"))
		defer srv.Close()
		require.ErrorContains(t, process(build(srv.URL)), "code 500")
	})
}
