package processors

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// relayDirectSigningWirePrefix is the Relay Mode-2 message header for protocolId=1
// (direct signing): 1B protocolId || 4B votingRoundId || 1B isSecureRandom.
const relayDirectSigningWirePrefix = "\x01\x00\x00\x00\x00\x00"

// relayPrefixedHash returns the digest an FDC2 data-provider or cosigner signature is
// recovered against: keccak256(chainID || 0x010000000000 || messageHash) when chainBound,
// keccak256(0x010000000000 || messageHash) before the cutover. Each must stay
// byte-identical to Relay.relay() Mode 2 and to Fdc2ProofVerification.toCosignersMessageHash
// of the Relay serving that reward epoch. The signer applies the eth_sign wrap.
//
// chainID is the Relay's sourceChainId; FDC2 only ships alongside a home Relay, where
// that is block.chainid.
//
// Not for the TEE's own signature, which recovers against the bare messageHash.
//
// TODO: drop the local chain-bound branch for tee-node's fdc.ChainBoundRelayPrefixedHash
// once the pin moves past v0.0.24 — that is what verifies these signatures, so the two
// must compute the same bytes.
func relayPrefixedHash(chainID uint64, chainBound bool, messageHash common.Hash) common.Hash {
	if !chainBound {
		return fdc.RelayPrefixedHash(messageHash)
	}

	var chainIDWord [32]byte
	binary.BigEndian.PutUint64(chainIDWord[24:], chainID)

	return crypto.Keccak256Hash(chainIDWord[:], []byte(relayDirectSigningWirePrefix), messageHash[:])
}

// logDigestSchedule reports the FDC2 signature digest in force for chainID.
//
// The TEE machines that verify these signatures compute the chain-bound digest
// unconditionally, so a scheduled epoch is only correct if it is the one the chain's
// contract batch and fleet swap land in.
func logDigestSchedule(chainID uint64, cutover config.RelayCutover) {
	switch e := cutover.StartingRewardEpoch; e {
	case config.CutoverUnscheduled:
		logger.Warnf("FDC2 signatures: relay cutover unscheduled on chain %d, signing every response with the pre-cutover digest", chainID)
	case 0:
		// absent decodes to 0, so this is also what a missing [relay_cutover] reports
		logger.Infof("FDC2 signatures: no relay cutover configured on chain %d, chain-bound digest for every reward epoch", chainID)
	default:
		logger.Infof("FDC2 signatures: chain-bound digest from reward epoch %d on chain %d, pre-cutover digest before it", e, chainID)
	}
}
