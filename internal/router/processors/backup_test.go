package processors

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/stretchr/testify/require"
)

// backupID builds a matching (received, requested) BackupID pair for the
// direct-restore consistency check.
func backupID() (wallets.WalletBackupID, wallet.IWalletBackupManagerBackupId) {
	requested := wallet.IWalletBackupManagerBackupId{
		TeeId:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		WalletId:      [32]byte(common.HexToHash("0x22")),
		KeyId:         7,
		KeyType:       [32]byte(common.HexToHash("0x33")),
		SigningAlgo:   [32]byte(common.HexToHash("0x44")),
		PublicKey:     []byte{1, 2, 3, 4},
		RewardEpochId: 9,
		RandomNonce:   [32]byte(common.HexToHash("0x55")),
	}
	received := wallets.WalletBackupID{
		TeeID:         requested.TeeId,
		WalletID:      common.Hash(requested.WalletId),
		KeyID:         requested.KeyId,
		PublicKey:     hexutil.Bytes{1, 2, 3, 4},
		KeyType:       common.Hash(requested.KeyType),
		SigningAlgo:   common.Hash(requested.SigningAlgo),
		RewardEpochID: requested.RewardEpochId,
		RandomNonce:   common.Hash(requested.RandomNonce),
	}
	return received, requested
}

func TestCheckConsistencyDirect(t *testing.T) {
	t.Parallel()

	t.Run("match", func(t *testing.T) {
		received, requested := backupID()
		require.NoError(t, checkConsistencyDirect(received, requested))
	})

	tests := []struct {
		name    string
		mutate  func(*wallets.WalletBackupID)
		wantErr string
	}{
		{"tee", func(id *wallets.WalletBackupID) { id.TeeID = common.HexToAddress("0xdead") }, "tee IDs do not match"},
		{"wallet", func(id *wallets.WalletBackupID) { id.WalletID = common.HexToHash("0xdead") }, "wallet IDs do not match"},
		{"key", func(id *wallets.WalletBackupID) { id.KeyID = 999 }, "key IDs do not match"},
		{"publicKey", func(id *wallets.WalletBackupID) { id.PublicKey = hexutil.Bytes{9} }, "public keys do not match"},
		{"keyType", func(id *wallets.WalletBackupID) { id.KeyType = common.HexToHash("0xdead") }, "key types do not match"},
		{"signingAlgo", func(id *wallets.WalletBackupID) { id.SigningAlgo = common.HexToHash("0xdead") }, "signing algos do not match"},
		{"rewardEpoch", func(id *wallets.WalletBackupID) { id.RewardEpochID = 999 }, "reward epochs do not match"},
		{"randomNonce", func(id *wallets.WalletBackupID) { id.RandomNonce = common.HexToHash("0xdead") }, "random nonces do not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received, requested := backupID()
			tt.mutate(&received)
			require.ErrorContains(t, checkConsistencyDirect(received, requested), tt.wantErr)
		})
	}
}

func TestCheckConsistency(t *testing.T) {
	t.Parallel()
	key, teeAddr := genKey(t)

	base := func() (wallet.IWalletBackupManagerKeyDataProviderRestore, wallets.WalletBackupID, []teeinstructions.IMachineManagerTeeMachine) {
		_, requested := backupID()
		id, _ := backupID()
		req := wallet.IWalletBackupManagerKeyDataProviderRestore{
			TeePublicKey: walletPubKey(key),
			BackupId:     requested,
		}
		tees := []teeinstructions.IMachineManagerTeeMachine{{TeeId: teeAddr}}
		return req, id, tees
	}

	t.Run("match", func(t *testing.T) {
		req, id, tees := base()
		require.NoError(t, checkConsistency(req, id, tees))
	})
	t.Run("too many tees", func(t *testing.T) {
		req, id, tees := base()
		tees = append(tees, teeinstructions.IMachineManagerTeeMachine{TeeId: teeAddr})
		require.ErrorContains(t, checkConsistency(req, id, tees), "one tee")
	})
	t.Run("pubkey not destination tee", func(t *testing.T) {
		req, id, tees := base()
		tees[0].TeeId = common.HexToAddress("0xdead")
		require.ErrorContains(t, checkConsistency(req, id, tees), "does not match the destination tee")
	})
	t.Run("invalid tee public key", func(t *testing.T) {
		req, id, tees := base()
		req.TeePublicKey = wallet.PublicKey{} // (0,0) is not on the curve
		require.ErrorContains(t, checkConsistency(req, id, tees), "parsing TEE public key")
	})

	fields := []struct {
		name    string
		mutate  func(*wallets.WalletBackupID)
		wantErr string
	}{
		{"teeID", func(id *wallets.WalletBackupID) { id.TeeID = common.HexToAddress("0xbeef") }, "teeID in the request does not match"},
		{"walletID", func(id *wallets.WalletBackupID) { id.WalletID = common.HexToHash("0xbeef") }, "walletID in the request does not match"},
		{"keyID", func(id *wallets.WalletBackupID) { id.KeyID = 999 }, "keyID in the request does not match"},
		{"publicKey", func(id *wallets.WalletBackupID) { id.PublicKey = hexutil.Bytes{9} }, "publicKey in the request does not match"},
		{"keyType", func(id *wallets.WalletBackupID) { id.KeyType = common.HexToHash("0xbeef") }, "keyType in the request does not match"},
		{"signingAlgo", func(id *wallets.WalletBackupID) { id.SigningAlgo = common.HexToHash("0xbeef") }, "signingAlgo in the request does not match"},
		{"rewardEpochID", func(id *wallets.WalletBackupID) { id.RewardEpochID = 999 }, "rewardEpochID in the request does not match"},
		{"randomNonce", func(id *wallets.WalletBackupID) { id.RandomNonce = common.HexToHash("0xbeef") }, "randomNonce in the request does not match"},
	}
	for _, tt := range fields {
		t.Run(tt.name, func(t *testing.T) {
			req, id, tees := base()
			tt.mutate(&id)
			require.ErrorContains(t, checkConsistency(req, id, tees), tt.wantErr)
		})
	}
}

// TestValidateActionResponseDirect guards the source-proxy action-response
// verification — including the three preimage/op checks that must mirror
// tee-node (correct chainID, single accounts.TextHash, KEY_DIRECT_BACKUP op).
func TestValidateActionResponseDirect(t *testing.T) {
	t.Parallel()
	const chainID = uint64(14)
	srcKey, sourceTEE := genKey(t)
	instrID := common.HexToHash("0xabc")

	validResult := func() types.ActionResult {
		return types.ActionResult{
			ID:            instrID,
			SubmissionTag: types.Threshold,
			Status:        1,
			OPType:        op.Wallet.Hash(),
			OPCommand:     op.KeyDirectBackup.Hash(),
			Data:          hexutil.Bytes("direct-backup-envelope"),
		}
	}
	sign := func(t *testing.T, key *ecdsa.PrivateKey, result types.ActionResult, signChainID uint64) types.ActionResponse {
		t.Helper()
		return types.ActionResponse{
			Result:    result,
			Signature: teeSign(t, key, signing.TEEActionResult, signChainID, [32]byte(result.Hash())),
		}
	}

	t.Run("valid", func(t *testing.T) {
		resp := sign(t, srcKey, validResult(), chainID)
		require.NoError(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID))
	})
	t.Run("wrong signer", func(t *testing.T) {
		otherKey, _ := genKey(t)
		resp := sign(t, otherKey, validResult(), chainID)
		require.Error(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID))
	})
	t.Run("wrong chainID in preimage", func(t *testing.T) {
		resp := sign(t, srcKey, validResult(), chainID+1)
		require.Error(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID))
	})
	t.Run("bad status", func(t *testing.T) {
		r := validResult()
		r.Status = 0
		resp := sign(t, srcKey, r, chainID)
		require.ErrorContains(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID), "status 0")
	})
	t.Run("bad submission tag", func(t *testing.T) {
		r := validResult()
		r.SubmissionTag = types.End
		resp := sign(t, srcKey, r, chainID)
		require.ErrorContains(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID), "submission tag")
	})
	t.Run("unexpected ID", func(t *testing.T) {
		resp := sign(t, srcKey, validResult(), chainID)
		require.ErrorContains(t, validateActionResponseDirect(resp, chainID, sourceTEE, common.HexToHash("0xfeed")), "unexpected ID")
	})
	t.Run("bad op type", func(t *testing.T) {
		r := validResult()
		r.OPType = op.FDC2.Hash()
		resp := sign(t, srcKey, r, chainID)
		require.ErrorContains(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID), "op type")
	})
	t.Run("bad op command", func(t *testing.T) {
		r := validResult()
		r.OPCommand = op.KeyDirectRestore.Hash()
		resp := sign(t, srcKey, r, chainID)
		require.ErrorContains(t, validateActionResponseDirect(resp, chainID, sourceTEE, instrID), "op command")
	})
}
