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

	index := slices.Index(wBackup.ProviderEncryptedParts.OwnersPublicKeys, *pk)
	indexAdmin := slices.Index(wBackup.AdminEncryptedParts.OwnersPublicKeys, *pk)

	var plaintextForTEE []byte

	switch {
	case index >= 0 && indexAdmin >= 0:
		keySplits := [2]backup.KeySplit{}

		keySplits[0], err = b.decryptKeySplit(ctx, wBackup.ProviderEncryptedParts.Splits[index])
		if err != nil {
			return err
		}
		keySplits[1], err = b.decryptKeySplit(ctx, wBackup.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return err
		}

		plaintextForTEE, err = json.Marshal(keySplits)
		if err != nil {
			return err
		}
	case index >= 0 && indexAdmin == -1:
		plaintextForTEE, err = b.base.signer.Decrypt(ctx, wBackup.ProviderEncryptedParts.Splits[index])
		if err != nil {
			return err
		}
	case index == -1 && indexAdmin >= 0:
		plaintextForTEE, err = b.base.signer.Decrypt(ctx, wBackup.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return err
		}
	default:
		return nil
	}

	teePK, err := types.ParsePubKey(types.PublicKey{
		X: fullRequest.TeePublicKey.X,
		Y: fullRequest.TeePublicKey.Y,
	})
	if err != nil {
		return err
	}

	pke := ecies.ImportECDSAPublic(teePK)

	cipher, err := ecies.Encrypt(rand.Reader, pke, plaintextForTEE, nil, nil)
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
