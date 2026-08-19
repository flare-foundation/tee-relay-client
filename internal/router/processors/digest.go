package processors

import (
	"encoding/binary"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-node/pkg/fdc"
)

// relayDirectSigningWirePrefix is the Relay Mode-2 message header for protocolId=1
// (direct signing): 1B protocolId || 4B votingRoundId || 1B isSecureRandom.
const relayDirectSigningWirePrefix = "\x01\x00\x00\x00\x00\x00"

// epochUnscheduled marks a chain whose breaking reward epoch has not been announced.
const epochUnscheduled = uint32(math.MaxUint32)

// breakingRewardEpochs is, per chain, the first reward epoch whose FDC2 responses the
// new Relay verifies — from it on they are signed with the chain-bound digest, before it
// with the pre-cutover one.
//
// A chain absent from the map is chain-bound at every epoch: the zero value makes every
// reward epoch compare at or past the boundary. Only a chain that already runs relay
// clients against the old Relay needs an entry. Flare is listed as 0 for the same reason
// it needs no transition — no relay client runs there until the new Relay is deployed.
//
// The values must match the ones the TEE machines gate on; they verify these signatures
// inside the enclave and a disagreement rejects every response across the boundary.
//
// An epoch must be the one whose start coincides with the contract batch. The chain has no
// per-epoch fallback here — Relay._verifyCustomSignature self-calls the new Relay and
// Fdc2ProofVerification.toCosignersMessageHash is inlined in the upgraded facets — so both
// forms are only ever accepted on their own side of the batch, never both at once.
//
// TODO: set the announced epochs for Coston, Coston2 and Songbird.
var breakingRewardEpochs = map[uint64]uint32{
	14:  0,                // Flare
	16:  epochUnscheduled, // Coston
	19:  epochUnscheduled, // Songbird
	114: epochUnscheduled, // Coston2
}

// chainBoundDigest reports whether chainID's FDC2 responses for rewardEpochID are signed
// with the chain-bound digest.
func chainBoundDigest(chainID uint64, rewardEpochID uint32) bool {
	return chainBoundFromEpoch(breakingRewardEpochs[chainID], rewardEpochID)
}

// chainBoundFromEpoch reports whether rewardEpochID is at or past breaking.
func chainBoundFromEpoch(breaking, rewardEpochID uint32) bool {
	return breaking != epochUnscheduled && rewardEpochID >= breaking
}

// relayPrefixedHash returns the digest an FDC2 data-provider or cosigner signature for
// rewardEpochID is recovered against: keccak256(chainID || 0x010000000000 || messageHash)
// from the chain's breaking reward epoch on, keccak256(0x010000000000 || messageHash)
// before it. Each must stay byte-identical to Relay.relay() Mode 2 and to
// Fdc2ProofVerification.toCosignersMessageHash of the Relay serving that epoch. The
// signer applies the eth_sign wrap.
//
// chainID is the Relay's sourceChainId; FDC2 only ships alongside a home Relay, where
// that is block.chainid.
//
// Not for the TEE's own signature, which recovers against the bare messageHash.
//
// TODO: fold both forms into tee-node's fdc package once it gates the digest too — that
// is what verifies these signatures, so the two must compute the same bytes.
func relayPrefixedHash(chainID uint64, rewardEpochID uint32, messageHash common.Hash) common.Hash {
	if !chainBoundDigest(chainID, rewardEpochID) {
		return fdc.RelayPrefixedHash(messageHash)
	}

	var chainIDWord [32]byte
	binary.BigEndian.PutUint64(chainIDWord[24:], chainID)

	return crypto.Keccak256Hash(chainIDWord[:], []byte(relayDirectSigningWirePrefix), messageHash[:])
}

// logDigestSchedule reports the FDC2 signature digest in force for chainID.
func logDigestSchedule(chainID uint64) {
	breaking, listed := breakingRewardEpochs[chainID]

	switch {
	case !listed || breaking == 0:
		logger.Infof("FDC2 signatures: chain-bound digest for every reward epoch on chain %d", chainID)
	case breaking == epochUnscheduled:
		logger.Warnf("FDC2 signatures: no breaking reward epoch set for chain %d, signing every response with the pre-cutover digest", chainID)
	default:
		logger.Infof("FDC2 signatures: chain-bound digest from reward epoch %d on chain %d, pre-cutover digest before it", breaking, chainID)
	}
}
