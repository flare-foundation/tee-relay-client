package processors

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/signing"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-node/pkg/types"
	"github.com/flare-foundation/tee-node/pkg/utils"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/stretchr/testify/require"
)

// genKey returns a fresh secp256k1 key and its Ethereum address.
func genKey(t *testing.T) (*ecdsa.PrivateKey, common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	return key, crypto.PubkeyToAddress(key.PublicKey)
}

// walletPubKey returns key's public key in the on-chain wallet.PublicKey form.
func walletPubKey(key *ecdsa.PrivateKey) wallet.PublicKey {
	pk := types.PubKeyToStruct(&key.PublicKey)
	return wallet.PublicKey{X: pk.X, Y: pk.Y}
}

// teeSign reproduces how a TEE signs an artifact: over
// signing.Payload{prefix, chainID, dataHash}.Hash(), wrapped once by
// accounts.TextHash via utils.Sign. Use it to forge source-TEE signatures.
func teeSign(t *testing.T, key *ecdsa.PrivateKey, prefix signing.Prefix, chainID uint64, dataHash [32]byte) hexutil.Bytes {
	t.Helper()
	h, err := signing.NewPayload(prefix, chainID, dataHash).Hash()
	require.NoError(t, err)
	sig, err := utils.Sign(h[:], key)
	require.NoError(t, err)
	return sig
}

// signableBase builds a minimal instruction destined for the given tee
// addresses that Base.Sign can hash and sign.
func signableBase(tees ...common.Address) *instructions.Base {
	machines := make([]teeinstructions.IMachineManagerTeeMachine, len(tees))
	for i, a := range tees {
		machines[i] = teeinstructions.IMachineManagerTeeMachine{TeeId: a}
	}
	return &instructions.Base{
		Tees: machines,
		GeneralData: instruction.Data{
			DataFixed: instruction.DataFixed{
				InstructionID: common.HexToHash("0xabc"),
				Timestamp:     1718113274,
			},
		},
	}
}
