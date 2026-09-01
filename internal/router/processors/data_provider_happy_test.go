package processors

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	csigner "github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/utils"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/flare-foundation/tee-node/pkg/wallets/backup"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// evmSign reproduces wallets EVMSignAlgo (keccak256-secp256k1-ecdsa) signing:
// crypto.Sign over keccak256(hash). VerifySignature recovers the same way.
func evmSign(t *testing.T, key *ecdsa.PrivateKey, hash [32]byte) []byte {
	t.Helper()
	sig, err := crypto.Sign(crypto.Keccak256(hash[:]), key)
	require.NoError(t, err)
	return sig
}

// pubKey64 returns the uncompressed [X||Y] public key (no 0x04 prefix).
func pubKey64(key *ecdsa.PrivateKey) []byte {
	return crypto.FromECDSAPub(&key.PublicKey)[1:]
}

func eciesEncrypt(t *testing.T, key *ecdsa.PrivateKey, msg []byte) []byte {
	t.Helper()
	pub, err := csigner.ECDSAPubKeyToECIES(&key.PublicKey)
	require.NoError(t, err)
	cipher, err := ecies.Encrypt(rand.Reader, pub, msg, nil, nil)
	require.NoError(t, err)
	return cipher
}

// TestProcessDataProviderRestoreHappyPath builds a fully-signed WalletBackup in
// which the relay (operator key) is a provider owner, runs the full restore, and
// decrypts the TEE-bound output to confirm the relay decrypted the provider key
// split and re-encrypted it for the destination TEE.
func TestProcessDataProviderRestoreHappyPath(t *testing.T) {
	t.Parallel()
	const chainID = uint64(14)

	relayKey, _ := genKey(t)       // operator / local signer; provider owner
	walletKey, _ := genKey(t)      // wallet identity; signs the backup and the split
	adminKey, _ := genKey(t)       // unrelated admin owner (relay is not in admin set)
	teeKey, teeAddr := genKey(t)   // backup TEE; its address is the backup id TeeID
	destKey, destAddr := genKey(t) // destination TEE the split is re-encrypted for

	id := wallets.WalletBackupID{
		TeeID:         teeAddr,
		WalletID:      common.HexToHash("0x22"),
		KeyID:         7,
		PublicKey:     pubKey64(walletKey),
		KeyType:       common.HexToHash("0x33"),
		SigningAlgo:   wallets.EVMSignAlgo,
		RewardEpochID: 9,
		RandomNonce:   common.HexToHash("0x55"),
	}

	// Provider split owned by the relay, signed by the wallet key, ECIES-encrypted
	// to the relay.
	ksd := backup.KeySplitData{
		Shares: []backup.ShamirShare{{X: 1, Y: []byte{2}}},
		PartialWalletBackupID: backup.PartialWalletBackupID{
			WalletBackupID: id,
			IsAdmin:        false,
		},
		OwnerPublicKey: types.PubKeyToStruct(&relayKey.PublicKey),
	}
	splitSignHash, err := ksd.SignHash(chainID)
	require.NoError(t, err)
	keySplit := backup.KeySplit{KeySplitData: ksd, Signature: evmSign(t, walletKey, splitSignHash)}
	keySplitBytes, err := json.Marshal(keySplit)
	require.NoError(t, err)

	provider := &backup.EncryptedShares{
		Splits:           []hexutil.Bytes{eciesEncrypt(t, relayKey, keySplitBytes)},
		OwnersPublicKeys: []types.PublicKey{types.PubKeyToStruct(&relayKey.PublicKey)},
		Threshold:        1,
		Weights:          []uint16{1},
	}
	admin := &backup.EncryptedShares{
		Splits:           []hexutil.Bytes{[]byte("admin-share-not-decrypted")},
		OwnersPublicKeys: []types.PublicKey{types.PubKeyToStruct(&adminKey.PublicKey)},
		Threshold:        1,
		Weights:          []uint16{1},
	}

	wb := backup.WalletBackup{
		WalletBackupMetaData: backup.WalletBackupMetaData{
			WalletBackupID:     id,
			AdminsPublicKeys:   []types.PublicKey{types.PubKeyToStruct(&adminKey.PublicKey)},
			AdminsThreshold:    1,
			ProvidersThreshold: 1,
		},
		AdminEncryptedParts:    admin,
		ProviderEncryptedParts: provider,
	}
	ownerHash, err := wb.OwnerSignHash(chainID)
	require.NoError(t, err)
	wb.Signature = evmSign(t, walletKey, ownerHash)
	teeHash, err := wb.TEESignHash(chainID)
	require.NoError(t, err)
	wb.TEESignature, err = utils.Sign(teeHash[:], teeKey)
	require.NoError(t, err)

	wbBytes, err := json.Marshal(wb)
	require.NoError(t, err)
	respBytes, err := json.Marshal(wallets.TEEBackupResponse{BackupID: id, WalletBackup: wbBytes})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(respBytes)
	}))
	defer srv.Close()

	req := wallet.IWalletBackupManagerKeyDataProviderRestore{
		TeePublicKey: walletPubKey(destKey),
		BackupId: wallet.IWalletBackupManagerBackupId{
			TeeId:         id.TeeID,
			WalletId:      [32]byte(id.WalletID),
			KeyId:         id.KeyID,
			KeyType:       [32]byte(id.KeyType),
			SigningAlgo:   [32]byte(id.SigningAlgo),
			PublicKey:     []byte(id.PublicKey),
			RewardEpochId: id.RewardEpochID,
			RandomNonce:   [32]byte(id.RandomNonce),
		},
		BackupUrl: srv.URL,
		Nonce:     big.NewInt(0),
	}
	msg, err := structs.Encode(wallet.MessageArguments[op.KeyDataProviderRestore], req)
	require.NoError(t, err)

	ib := signableBase(destAddr)
	ib.Event = &instructions.InstructionSentEvent{OpCommand: op.KeyDataProviderRestore.Hash()}
	ib.GeneralData.OriginalMessage = msg

	out := make(chan *instructions.Base, 1)
	base := NewBase(chainID, config.RelayCutover{}, signer.NewLocal(relayKey))
	base.SetOut(out)
	require.NoError(t, NewBackup(base, true).Process(context.Background(), ib))

	got := <-out
	require.Len(t, got.Signatures, 1)

	wantMeta, err := json.Marshal(wb.WalletBackupMetaData)
	require.NoError(t, err)
	require.Equal(t, hexutil.Bytes(wantMeta), got.GeneralData.AdditionalFixedMessage)

	// Decrypt the TEE-bound output with the destination key: it must be the
	// relay's provider key split, re-encrypted by the relay.
	destECIES, err := csigner.ECDSAPrivKeyToECIES(destKey)
	require.NoError(t, err)
	forTEE, err := destECIES.Decrypt(got.GeneralData.AdditionalVariableMessage, nil, nil)
	require.NoError(t, err)
	var roundTripped backup.KeySplit
	require.NoError(t, json.Unmarshal(forTEE, &roundTripped))
	require.Equal(t, types.PubKeyToStruct(&relayKey.PublicKey), roundTripped.OwnerPublicKey)
	require.NoError(t, roundTripped.VerifySignature(chainID))
}
