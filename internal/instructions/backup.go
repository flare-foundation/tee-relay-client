package instructions

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"

	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/flare-foundation/tee-node/pkg/wallets/backup"
)

type BackupProcessor struct {
	base *BaseProcessor
}

const sizeLimit = 100 << 10 // 100 Kib

// Process handles the backup restore flow for a TEE wallet.
// It fetches backup data, decodes and decrypts it, prepares the message for TEE,
// encrypts it for the TEE node, signs the message, and sends it to the output channel.
func (b *BackupProcessor) Process(ctx context.Context, ib *Base) error {
	fullRequest, err := structs.Decode[wallet.ITeeWalletBackupManagerKeyDataProviderRestore](wallet.MessageArguments[op.KeyDataProviderRestore], ib.GeneralData.OriginalMessage)

	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fullRequest.BackupUrl, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		if resp.Header.Get("Content-Type") == "text/plain; charset=utf-8" {
			respLimited := &io.LimitedReader{R: resp.Body, N: sizeLimit}
			buf := new(strings.Builder)
			_, err := io.Copy(buf, respLimited)
			// check errors
			if err == nil {
				return fmt.Errorf("request responded with code %d, reason: %s", resp.StatusCode, buf.String())
			}
		}

		return fmt.Errorf("request responded with code %d", resp.StatusCode)
	}

	respLimited := &io.LimitedReader{R: resp.Body, N: sizeLimit}
	defer resp.Body.Close() //nolint:errcheck

	decoder := json.NewDecoder(respLimited)
	response := new(wallets.TEEBackupResponse)

	err = decoder.Decode(response)
	if err != nil {
		return err
	}

	var wBackup backup.WalletBackup
	err = json.Unmarshal(response.WalletBackup, &wBackup)
	if err != nil {
		return err
	}

	ib.GeneralData.AdditionalFixedMessage, err = json.Marshal(wBackup.WalletBackupMetaData)
	if err != nil {
		return err
	}

	pk, err := b.base.signer.Identify(ctx)
	if err != nil {
		return err
	}

	ptForTEE, err := b.plaintextForTEE(ctx, wBackup, pk)
	if ptForTEE == nil { // if err is not nil, ptForTEE is nil. If err is nil and ptForTEE is nil, there is the entity legitimately has nothing to send.
		return err
	}

	teePK, err := types.ParsePubKey(types.PublicKey{
		X: fullRequest.TeePublicKey.X,
		Y: fullRequest.TeePublicKey.Y,
	})
	if err != nil {
		return err
	}

	pke := ecies.ImportECDSAPublic(teePK)
	cipher, err := ecies.Encrypt(rand.Reader, pke, ptForTEE, nil, nil)
	if err != nil {
		return err
	}

	ib.GeneralData.AdditionalVariableMessage = cipher

	err = ib.Sign(ctx, b.base.signer)
	if err != nil {
		return fmt.Errorf("signing: %v", err)
	}

	select {
	case b.base.out <- ib:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// decryptKeySplit decrypts a KeySplit from the provided cipher.
func (b *BackupProcessor) decryptKeySplit(ctx context.Context, cipher []byte) (backup.KeySplit, error) {
	var keySplit backup.KeySplit

	plaintext, err := b.base.signer.Decrypt(ctx, cipher)
	if err != nil {
		return keySplit, err
	}

	err = json.Unmarshal(plaintext, &keySplit)
	if err != nil {
		return keySplit, err
	}

	return keySplit, nil
}

// plaintextForTEE returns the decrypted key splits for the TEE node, based on the wallet backup and public key.
// It finds the relevant encrypted parts, decrypts them, and marshals the result.
// If the public key is both among the provider and admin owners, it returns marshaled array of both split.
// If the public key is only among one of them, it returns the decrypted split directly.
// If the public key is not found in either, it returns nil which indicates there is nothing to send.
func (b *BackupProcessor) plaintextForTEE(ctx context.Context, wb backup.WalletBackup, pk *types.PublicKey) ([]byte, error) {
	index := slices.Index(wb.ProviderEncryptedParts.OwnersPublicKeys, *pk)
	indexAdmin := slices.Index(wb.AdminEncryptedParts.OwnersPublicKeys, *pk)

	var err error

	switch {
	case index >= 0 && indexAdmin >= 0:
		keySplits := [2]backup.KeySplit{}

		keySplits[0], err = b.decryptKeySplit(ctx, wb.ProviderEncryptedParts.Splits[index])
		if err != nil {
			return nil, err
		}
		keySplits[1], err = b.decryptKeySplit(ctx, wb.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return nil, err
		}

		res, err := json.Marshal(keySplits)
		if err != nil {
			return nil, err
		}

		return res, nil
	case index >= 0 && indexAdmin == -1:
		res, err := b.base.signer.Decrypt(ctx, wb.ProviderEncryptedParts.Splits[index])
		if err != nil {
			return nil, err
		}

		return res, err
	case index == -1 && indexAdmin >= 0:
		res, err := b.base.signer.Decrypt(ctx, wb.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return nil, err
		}

		return res, err

	default:
		return nil, nil
	}
}
