package processors

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/flare-foundation/tee-node/pkg/wallets/backup"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
	"github.com/stretchr/testify/require"
)

// relaySplit builds a KeySplit owned by ownerKey, signed by walletKey, and
// ECIES-encrypted to ownerKey, with its WalletBackupID bound to id.
func relaySplit(t *testing.T, walletKey, ownerKey *ecdsa.PrivateKey, id wallets.WalletBackupID, chainID uint64, isAdmin bool) hexutil.Bytes {
	t.Helper()
	ksd := backup.KeySplitData{
		Shares:                []backup.ShamirShare{{X: big.NewInt(1), Y: big.NewInt(2)}},
		PartialWalletBackupID: backup.PartialWalletBackupID{WalletBackupID: id, PartialPubKey: pubKey64(walletKey), IsAdmin: isAdmin},
		OwnerPublicKey:        types.PubKeyToStruct(&ownerKey.PublicKey),
	}
	signHash, err := ksd.SignHash(chainID)
	require.NoError(t, err)
	ks := backup.KeySplit{KeySplitData: ksd, Signature: evmSign(t, walletKey, signHash)}
	b, err := json.Marshal(ks)
	require.NoError(t, err)
	return eciesEncrypt(t, ownerKey, b)
}

func shares(owner types.PublicKey, split hexutil.Bytes) *backup.EncryptedShares {
	return &backup.EncryptedShares{
		Splits:           []hexutil.Bytes{split},
		OwnersPublicKeys: []types.PublicKey{owner},
		Threshold:        1,
		Weights:          []uint16{1},
	}
}

func TestPlaintextForTEE(t *testing.T) {
	t.Parallel()
	const chainID = uint64(14)

	relayKey, _ := genKey(t)
	walletKey, _ := genKey(t)
	otherKey, _ := genKey(t)

	id := wallets.WalletBackupID{
		TeeID:       common.HexToAddress("0xabc"),
		WalletID:    common.HexToHash("0x22"),
		KeyID:       7,
		PublicKey:   pubKey64(walletKey),
		KeyType:     common.HexToHash("0x33"),
		SigningAlgo: wallets.EVMSignAlgo,
		RandomNonce: common.HexToHash("0x55"),
	}
	relayPub := types.PubKeyToStruct(&relayKey.PublicKey)
	otherPub := types.PubKeyToStruct(&otherKey.PublicKey)
	junk := hexutil.Bytes("not-decrypted")

	relayProvider := relaySplit(t, walletKey, relayKey, id, chainID, false)
	relayAdmin := relaySplit(t, walletKey, relayKey, id, chainID, true)

	b := NewBackup(NewBase(chainID, signer.NewLocal(relayKey)), true)

	t.Run("provider only", func(t *testing.T) {
		wb := backup.WalletBackup{
			WalletBackupMetaData:   backup.WalletBackupMetaData{WalletBackupID: id},
			ProviderEncryptedParts: shares(relayPub, relayProvider),
			AdminEncryptedParts:    shares(otherPub, junk),
		}
		res, err := b.plaintextForTEE(context.Background(), wb, &relayPub)
		require.NoError(t, err)
		var ks backup.KeySplit
		require.NoError(t, json.Unmarshal(res, &ks))
		require.NoError(t, ks.VerifySignature(chainID))
	})

	t.Run("admin only", func(t *testing.T) {
		wb := backup.WalletBackup{
			WalletBackupMetaData:   backup.WalletBackupMetaData{WalletBackupID: id},
			ProviderEncryptedParts: shares(otherPub, junk),
			AdminEncryptedParts:    shares(relayPub, relayAdmin),
		}
		res, err := b.plaintextForTEE(context.Background(), wb, &relayPub)
		require.NoError(t, err)
		var ks backup.KeySplit
		require.NoError(t, json.Unmarshal(res, &ks))
		require.NoError(t, ks.VerifySignature(chainID))
	})

	t.Run("provider and admin", func(t *testing.T) {
		wb := backup.WalletBackup{
			WalletBackupMetaData:   backup.WalletBackupMetaData{WalletBackupID: id},
			ProviderEncryptedParts: shares(relayPub, relayProvider),
			AdminEncryptedParts:    shares(relayPub, relayAdmin),
		}
		res, err := b.plaintextForTEE(context.Background(), wb, &relayPub)
		require.NoError(t, err)
		var both [2]backup.KeySplit
		require.NoError(t, json.Unmarshal(res, &both))
		require.NoError(t, both[0].VerifySignature(chainID))
		require.NoError(t, both[1].VerifySignature(chainID))
	})

	t.Run("owner in neither set returns nil", func(t *testing.T) {
		wb := backup.WalletBackup{
			WalletBackupMetaData:   backup.WalletBackupMetaData{WalletBackupID: id},
			ProviderEncryptedParts: shares(otherPub, junk),
			AdminEncryptedParts:    shares(otherPub, junk),
		}
		res, err := b.plaintextForTEE(context.Background(), wb, &relayPub)
		require.NoError(t, err)
		require.Nil(t, res)
	})
}
