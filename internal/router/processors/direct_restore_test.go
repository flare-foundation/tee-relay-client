package processors

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/wallets/backup"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// TestProcessDirectRestore drives processDirectRestore end-to-end against an
// httptest source proxy. The happy path proves the envelope and action-result
// signatures are reconstructed exactly as tee-node signs them (guarding the
// preimage fixes); the negative cases prove the new gates reject bad input.
func TestProcessDirectRestore(t *testing.T) {
	t.Parallel()
	const chainID = uint64(14)

	operatorKey, _ := genKey(t)
	srcKey, sourceTEE := genKey(t)
	destTEE := common.HexToAddress("0x2222222222222222222222222222222222222222")
	backupInstrID := common.HexToHash("0xdeadbeef")

	wbID, reqBackupID := backupID()
	payloadBytes, err := json.Marshal(backup.KeyDirectBackupPayload{BackupID: wbID})
	require.NoError(t, err)

	// envelope returns a SignedKeyDirectBackup signed by envelopeSigner.
	envelope := func(envelopeSigner *ecdsa.PrivateKey) []byte {
		b, mErr := json.Marshal(types.SignedKeyDirectBackup{
			Payload:      payloadBytes,
			TEESignature: teeSign(t, envelopeSigner, signing.TEEKeyDirectBackup, chainID, crypto.Keccak256Hash(payloadBytes)),
		})
		require.NoError(t, mErr)
		return b
	}

	// actionResponse wraps env in a source-TEE-signed ActionResponse.
	actionResponse := func(env []byte) []byte {
		result := types.ActionResult{
			ID:            backupInstrID,
			SubmissionTag: types.Threshold,
			Status:        1,
			OPType:        op.Wallet.Hash(),
			OPCommand:     op.KeyDirectBackup.Hash(),
			Data:          env,
		}
		b, mErr := json.Marshal(types.ActionResponse{
			Result:    result,
			Signature: teeSign(t, srcKey, signing.TEEActionResult, chainID, [32]byte(result.Hash())),
		})
		require.NoError(t, mErr)
		return b
	}

	proxy := func(body []byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}))
	}

	build := func(url string, tees ...common.Address) *instructions.Base {
		req := wallet.IWalletBackupManagerKeyDirectRestore{
			SourceTeeId:          sourceTEE,
			SourceProxyUrl:       url,
			BackupId:             reqBackupID,
			BackupInstructionId:  [32]byte(backupInstrID),
			DestinationNonce:     big.NewInt(0),
			MachinePathListNonce: big.NewInt(0),
		}
		msg, eErr := structs.Encode(wallet.MessageArguments[op.KeyDirectRestore], req)
		require.NoError(t, eErr)
		ib := signableBase(tees...)
		ib.Event = &instructions.InstructionSentEvent{OpCommand: op.KeyDirectRestore.Hash()}
		ib.GeneralData.OriginalMessage = msg
		return ib
	}

	processor := func(out chan *instructions.Base) *Backup {
		base := NewBase(chainID, config.RelayCutover{}, signer.NewLocal(operatorKey))
		base.SetOut(out)
		return NewBackup(base, true) // allowUnsafeURLs: reach the httptest loopback
	}

	t.Run("happy path", func(t *testing.T) {
		env := envelope(srcKey)
		srv := proxy(actionResponse(env))
		defer srv.Close()

		out := make(chan *instructions.Base, 1)
		ib := build(srv.URL, destTEE)
		require.NoError(t, processor(out).Process(context.Background(), ib))

		got := <-out
		require.Equal(t, hexutil.Bytes(env), got.GeneralData.AdditionalFixedMessage)
		require.Len(t, got.Signatures, 1)
	})

	t.Run("more than one tee rejected before fetch", func(t *testing.T) {
		out := make(chan *instructions.Base, 1)
		ib := build("http://127.0.0.1:1", destTEE, common.HexToAddress("0x3333333333333333333333333333333333333333"))
		require.ErrorContains(t, processor(out).Process(context.Background(), ib), "one destination")
	})

	t.Run("envelope signed by wrong TEE", func(t *testing.T) {
		srv := proxy(actionResponse(envelope(operatorKey))) // not the source TEE
		defer srv.Close()

		out := make(chan *instructions.Base, 1)
		ib := build(srv.URL, destTEE)
		require.ErrorContains(t, processor(out).Process(context.Background(), ib), "source TEE")
	})
}
