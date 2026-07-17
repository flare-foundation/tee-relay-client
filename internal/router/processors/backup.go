package processors

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/safeurl"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
	"github.com/flare-foundation/go-flare-common/pkg/tee/signer"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"

	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/utils"
	"github.com/flare-foundation/tee-node/pkg/wallets"
	"github.com/flare-foundation/tee-node/pkg/wallets/backup"
)

// Backup processes wallet backup restore instructions.
type Backup struct {
	base            *Base
	allowUnsafeURLs bool
}

const (
	sizeLimitDataProviderRestore = 2 << 20   // 2 MiB
	sizeLimitDirectRestore       = 100 << 10 // 100 KiB
	errorSizeLimit               = 1 << 10   // 1 KiB
)

// NewBackup creates a Backup processor. When allowUnsafeURLs is true SSRF
// protection for backup URLs is disabled — only for local testing.
func NewBackup(base *Base, allowUnsafeURLs bool) *Backup {
	return &Backup{base: base, allowUnsafeURLs: allowUnsafeURLs}
}

// Process dispatches to the per-op restore handler.
func (b *Backup) Process(ctx context.Context, ib *instructions.Base) error {
	switch op.HashToOPCommand(ib.Event.OpCommand) {
	case op.KeyDataProviderRestore:
		return b.processDataProviderRestore(ctx, ib)
	case op.KeyDirectRestore:
		return b.processDirectRestore(ctx, ib)
	default:
		return fmt.Errorf("backup processor: unsupported op command %s", common.Hash(ib.Event.OpCommand).Hex())
	}
}

// processDataProviderRestore handles the legacy KEY_DATA_PROVIDER_RESTORE flow:
// it fetches the backup, decrypts this data provider's key split, encrypts it
// for the TEE, signs, and forwards.
func (b *Backup) processDataProviderRestore(ctx context.Context, ib *instructions.Base) error {
	fullRequest, err := structs.Decode[wallet.IWalletBackupManagerKeyDataProviderRestore](wallet.MessageArguments[op.KeyDataProviderRestore], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding restore request: %w", err)
	}

	if !b.allowUnsafeURLs {
		if err = safeurl.Validate(ctx, fullRequest.BackupUrl); err != nil {
			return fmt.Errorf("validating backup URL: %w", err)
		}
	}

	var client *http.Client
	if b.allowUnsafeURLs {
		client = &http.Client{Timeout: 10 * time.Second}
	} else {
		client = safeurl.NewClient(10 * time.Second)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fullRequest.BackupUrl, nil)
	if err != nil {
		return fmt.Errorf("creating backup request: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetching backup: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // closing response body, error is not actionable

	logger.Debugf("restore %s: fetched backup from %s in %s", common.Hash(ib.Event.InstructionId).Hex(), strconv.Quote(fullRequest.BackupUrl), time.Since(start))

	if resp.StatusCode != http.StatusOK {
		if resp.Header.Get("Content-Type") == "text/plain; charset=utf-8" {
			respLimited := &io.LimitedReader{R: resp.Body, N: errorSizeLimit}
			buf := new(strings.Builder)
			_, err := io.Copy(buf, respLimited)
			if err == nil {
				return fmt.Errorf("request responded with code %d, reason: %s", resp.StatusCode, strconv.Quote(buf.String()))
			}
		}

		return fmt.Errorf("request responded with code %d", resp.StatusCode)
	}

	respLimited := &io.LimitedReader{
		R: resp.Body,
		N: sizeLimitDataProviderRestore,
	}

	decoder := json.NewDecoder(respLimited)
	response := new(wallets.TEEBackupResponse)
	err = decoder.Decode(response)
	if err != nil {
		return fmt.Errorf("decoding backup response: %w", err)
	}

	err = checkConsistency(fullRequest, response.BackupID, ib.Tees)
	if err != nil {
		return fmt.Errorf("backup package inconsistent with the request: %w", err)
	}

	var wBackup backup.WalletBackup
	err = json.Unmarshal(response.WalletBackup, &wBackup)
	if err != nil {
		return fmt.Errorf("unmarshaling wallet backup: %w", err)
	}

	err = wBackup.Check(b.base.chainID)
	if err != nil {
		return fmt.Errorf("checking wallet backup: %w", err)
	}

	// the blob's id must match the request-validated response id
	if response.BackupID.Equal(&wBackup.WalletBackupID) != nil {
		return errors.New("backup package wallet id does not match the response backup id")
	}

	ib.GeneralData.AdditionalFixedMessage, err = json.Marshal(wBackup.WalletBackupMetaData)
	if err != nil {
		return fmt.Errorf("marshaling wallet backup metadata: %w", err)
	}

	pk, err := b.base.signer.Identify(ctx)
	if err != nil {
		return fmt.Errorf("identifying signer: %w", err)
	}

	ptForTEE, err := b.plaintextForTEE(ctx, wBackup, &pk)
	if ptForTEE == nil { // if err is not nil, ptForTEE is nil. If err is nil and ptForTEE is nil, the entity legitimately has nothing to send.
		if err == nil {
			logger.Debugf("restore %s: signer not among backup owners, nothing to send", common.Hash(ib.Event.InstructionId).Hex())
		}
		return err
	}

	teePK, err := types.ParsePubKey(types.PublicKey{
		X: fullRequest.TeePublicKey.X,
		Y: fullRequest.TeePublicKey.Y,
	})
	if err != nil {
		return fmt.Errorf("parsing TEE public key: %w", err)
	}

	pke, err := signer.ECDSAPubKeyToECIES(teePK)
	if err != nil {
		return fmt.Errorf("converting TEE public key to ECIES: %w", err)
	}
	cipher, err := ecies.Encrypt(rand.Reader, pke, ptForTEE, nil, nil)
	if err != nil {
		return fmt.Errorf("encrypting for TEE: %w", err)
	}

	ib.GeneralData.AdditionalVariableMessage = cipher

	err = ib.Sign(ctx, b.base.signer, b.base.chainID)
	if err != nil {
		return fmt.Errorf("signing: %w", err)
	}

	select {
	case b.base.out <- ib:
		logger.Debugf("restore %s: forwarded to sender", common.Hash(ib.Event.InstructionId).Hex())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// decryptKeySplit decrypts a KeySplit from the provided cipher.
func (b *Backup) decryptKeySplit(ctx context.Context, cipher []byte) (backup.KeySplit, error) {
	var keySplit backup.KeySplit

	plaintext, err := b.base.signer.Decrypt(ctx, cipher)
	if err != nil {
		return keySplit, fmt.Errorf("decrypting: %w", err)
	}

	err = json.Unmarshal(plaintext, &keySplit)
	if err != nil {
		return keySplit, fmt.Errorf("unmarshaling key split: %w", err)
	}

	err = keySplit.VerifySignature(b.base.chainID)
	if err != nil {
		return keySplit, fmt.Errorf("verifying key split signature: %w", err)
	}

	return keySplit, nil
}

// checkConsistency checks that the fields in the restore request match those in the wallet backup ID.
func checkConsistency(request wallet.IWalletBackupManagerKeyDataProviderRestore, id wallets.WalletBackupID, tees []teeinstructions.IMachineManagerTeeMachine) error {
	pk, err := types.ParsePubKey(types.PublicKey{
		X: request.TeePublicKey.X,
		Y: request.TeePublicKey.Y,
	})
	if err != nil {
		return fmt.Errorf("parsing TEE public key: %w", err)
	}

	recoveredTeeID := crypto.PubkeyToAddress(*pk)

	switch {
	case len(tees) != 1:
		return errors.New("restore can only be requested on one tee per instruction")
	case recoveredTeeID != tees[0].TeeId:
		return errors.New("provided public key does not match the destination tee")
	case request.BackupId.TeeId != id.TeeID:
		return errors.New("teeID in the request does not match the teeID in the wallet backup id")
	case common.Hash(request.BackupId.WalletId) != id.WalletID:
		return errors.New("walletID in the request does not match the walletID in the wallet backup id")
	case request.BackupId.KeyId != id.KeyID:
		return errors.New("keyID in the request does not match the keyID in the wallet backup id")
	case !slices.Equal(request.BackupId.PublicKey, id.PublicKey):
		return errors.New("publicKey in the request does not match the publicKey in the wallet backup id")
	case common.Hash(request.BackupId.KeyType) != id.KeyType:
		return errors.New("keyType in the request does not match the keyType in the wallet backup id")
	case common.Hash(request.BackupId.SigningAlgo) != id.SigningAlgo:
		return errors.New("signingAlgo in the request does not match the signingAlgo in the wallet backup id")
	case request.BackupId.RewardEpochId != id.RewardEpochID:
		return errors.New("rewardEpochID in the request does not match the rewardEpochID in the wallet backup id")
	case common.Hash(request.BackupId.RandomNonce) != id.RandomNonce:
		return errors.New("randomNonce in the request does not match the randomNonce in the wallet backup id")
	default:
		return nil
	}
}

// plaintextForTEE returns the decrypted key splits for the TEE node, based on the wallet backup and public key.
// It finds the relevant encrypted parts, decrypts them, and marshals the result.
// If the public key is both among the provider and admin owners, it returns marshaled array of both split.
// If the public key is only among one of them, it returns the decrypted split directly.
// If the public key is not found in either, it returns nil which indicates there is nothing to send.
func (b *Backup) plaintextForTEE(ctx context.Context, wb backup.WalletBackup, pk *types.PublicKey) ([]byte, error) {
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

		if wb.WalletBackupID.Equal(&keySplits[0].WalletBackupID) != nil { //nolint:staticcheck // embedded field used to avoid ambiguity
			return nil, errors.New("invalid wallet id in provider's key split")
		}

		keySplits[1], err = b.decryptKeySplit(ctx, wb.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return nil, err
		}

		if wb.WalletBackupID.Equal(&keySplits[1].WalletBackupID) != nil { //nolint:staticcheck // embedded field used to avoid ambiguity
			return nil, errors.New("invalid wallet id in admin's key split")
		}

		res, err := json.Marshal(keySplits)
		if err != nil {
			return nil, err
		}

		return res, nil
	case index >= 0 && indexAdmin == -1:
		keySplit, err := b.decryptKeySplit(ctx, wb.ProviderEncryptedParts.Splits[index])
		if err != nil {
			return nil, err
		}

		if wb.WalletBackupID.Equal(&keySplit.WalletBackupID) != nil { //nolint:staticcheck // embedded field used to avoid ambiguity
			return nil, errors.New("invalid wallet id in provider's key split")
		}

		res, err := json.Marshal(keySplit)
		if err != nil {
			return nil, err
		}

		return res, err
	case index == -1 && indexAdmin >= 0:
		keySplit, err := b.decryptKeySplit(ctx, wb.AdminEncryptedParts.Splits[indexAdmin])
		if err != nil {
			return nil, err
		}

		if wb.WalletBackupID.Equal(&keySplit.WalletBackupID) != nil { //nolint:staticcheck // embedded field used to avoid ambiguity
			return nil, errors.New("invalid wallet id in admin's key split")
		}

		res, err := json.Marshal(keySplit)
		if err != nil {
			return nil, err
		}

		return res, err

	default:
		return nil, nil
	}
}

// processDirectRestore handles KEY_DIRECT_RESTORE: it fetches the source TEE's
// direct-backup envelope from the source proxy's GET /action/result/{id} and
// splices it onto AdditionalFixedMessage for the destination TEE.
func (b *Backup) processDirectRestore(ctx context.Context, ib *instructions.Base) error {
	if len(ib.Tees) != 1 {
		return errors.New("direct restore is only possible to one destination")
	}

	req, err := structs.Decode[wallet.IWalletBackupManagerKeyDirectRestore](wallet.MessageArguments[op.KeyDirectRestore], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding direct restore request: %w", err)
	}

	backupActionID := common.BytesToHash(req.BackupInstructionId[:])
	url := strings.TrimRight(req.SourceProxyUrl, "/") + "/action/result/" + backupActionID.Hex()

	// SourceProxyUrl is on-chain / attacker-influenced, so guard against SSRF.
	if !b.allowUnsafeURLs {
		if err = safeurl.Validate(ctx, url); err != nil {
			return fmt.Errorf("validating source proxy URL: %w", err)
		}
	}

	var client *http.Client
	if b.allowUnsafeURLs {
		client = &http.Client{Timeout: 10 * time.Second}
	} else {
		client = safeurl.NewClient(10 * time.Second)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating direct backup fetch request: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("fetching direct backup envelope from %s: %w", strconv.Quote(url), err)
	}
	defer resp.Body.Close() //nolint:errcheck // closing response body, error is not actionable

	logger.Debugf("direct restore %s: fetched envelope from %s in %s", common.Hash(ib.Event.InstructionId).Hex(), strconv.Quote(url), time.Since(start))

	if resp.StatusCode != http.StatusOK {
		if resp.Header.Get("Content-Type") == "text/plain; charset=utf-8" {
			respLimited := &io.LimitedReader{
				R: resp.Body,
				N: errorSizeLimit,
			}
			buf := new(strings.Builder)
			if _, copyErr := io.Copy(buf, respLimited); copyErr == nil {
				return fmt.Errorf("direct backup fetch responded with code %d, reason: %s", resp.StatusCode, strconv.Quote(buf.String()))
			}
		}
		return fmt.Errorf("direct backup fetch responded with code %d", resp.StatusCode)
	}

	respLimited := &io.LimitedReader{
		R: resp.Body,
		N: sizeLimitDirectRestore,
	}
	var actionResp types.ActionResponse
	if err = json.NewDecoder(respLimited).Decode(&actionResp); err != nil {
		return fmt.Errorf("decoding action response from source proxy: %w", err)
	}

	if err := validateActionResponseDirect(actionResp, b.base.chainID, req.SourceTeeId, req.BackupInstructionId); err != nil {
		return fmt.Errorf("validating backup action response: %w", err)
	}

	// Result.Data is the SignedKeyDirectBackup envelope the destination TEE expects.
	ib.GeneralData.AdditionalFixedMessage = actionResp.Result.Data

	var skdb types.SignedKeyDirectBackup
	if err := json.Unmarshal(actionResp.Result.Data, &skdb); err != nil {
		return fmt.Errorf("unmarshaling SignedKeyDirectBackup: %w", err)
	}

	prefixedHash, err := signing.NewPayload(signing.TEEKeyDirectBackup, b.base.chainID, crypto.Keccak256Hash(skdb.Payload)).Hash()
	if err != nil {
		return fmt.Errorf("retrieving SignedKeyDirectBackup Payload hash: %w", err)
	}
	if err := utils.VerifySignature(prefixedHash[:], skdb.TEESignature, req.SourceTeeId); err != nil {
		return fmt.Errorf("verifying SignedKeyDirectBackup signature against source TEE: %w", err)
	}

	var payload backup.KeyDirectBackupPayload
	if err := json.Unmarshal(skdb.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshaling KeyDirectBackupPayload, %w", err)
	}

	if err := checkConsistencyDirect(payload.BackupID, req.BackupId); err != nil {
		return fmt.Errorf("checking consistency: %w", err)
	}

	if err = ib.Sign(ctx, b.base.signer, b.base.chainID); err != nil {
		return fmt.Errorf("signing: %w", err)
	}

	select {
	case b.base.out <- ib:
		logger.Debugf("direct restore %s: forwarded to sender", common.Hash(ib.Event.InstructionId).Hex())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// validateActionResponseDirect verifies the source proxy's action response is
// signed by the source TEE and is the expected successful KEY_DIRECT_BACKUP action.
func validateActionResponseDirect(resp types.ActionResponse, chainID uint64, sourceTEE common.Address, expectedInstructionID common.Hash) error {
	respHash, err := signing.NewPayload(signing.TEEActionResult, chainID, [32]byte(resp.Result.Hash())).Hash()
	if err != nil {
		return fmt.Errorf("retrieving action result signing hash: %w", err)
	}

	if err = utils.VerifySignature(respHash[:], resp.Signature, sourceTEE); err != nil {
		return err
	}
	if resp.Result.Status != 1 {
		return fmt.Errorf("response has status %d (should be 1)", resp.Result.Status)
	}
	if resp.Result.SubmissionTag != types.Threshold {
		return fmt.Errorf("response has submission tag %v (should be %v)", resp.Result.SubmissionTag, types.Threshold)
	}
	if resp.Result.ID != expectedInstructionID {
		return errors.New("response has unexpected ID")
	}
	if resp.Result.OPType != op.Wallet.Hash() {
		return fmt.Errorf("response has op type %s", op.HashToOPType(resp.Result.OPType))
	}
	if resp.Result.OPCommand != op.KeyDirectBackup.Hash() {
		return fmt.Errorf("response has op command %s", op.HashToOPCommand(resp.Result.OPCommand))
	}

	return nil
}

// checkConsistencyDirect checks that the backup payload's BackupID matches the
// requested BackupId field by field.
func checkConsistencyDirect(receivedID wallets.WalletBackupID, requestedID wallet.IWalletBackupManagerBackupId) error {
	switch {
	case receivedID.TeeID != requestedID.TeeId:
		return errors.New("tee IDs do not match")
	case receivedID.WalletID != common.Hash(requestedID.WalletId):
		return errors.New("wallet IDs do not match")
	case receivedID.KeyID != requestedID.KeyId:
		return errors.New("key IDs do not match")
	case !bytes.Equal(receivedID.PublicKey, requestedID.PublicKey):
		return errors.New("public keys do not match")
	case receivedID.KeyType != common.Hash(requestedID.KeyType):
		return errors.New("key types do not match")
	case receivedID.SigningAlgo != common.Hash(requestedID.SigningAlgo):
		return errors.New("signing algos do not match")
	case receivedID.RewardEpochID != requestedID.RewardEpochId:
		return errors.New("reward epochs do not match")
	case receivedID.RandomNonce != common.Hash(requestedID.RandomNonce):
		return errors.New("random nonces do not match")
	}

	return nil
}
