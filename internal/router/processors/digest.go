package processors

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-node/pkg/fdc"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// relayPrefixedHash returns the digest an FDC2 data-provider or cosigner signature is
// recovered against, selecting the form by the chain's cutover. Both forms come from
// tee-node, which verifies these signatures, so the two sides cannot drift; each must
// stay byte-identical to Relay.relay() Mode 2 and to
// Fdc2ProofVerification.toCosignersMessageHash of the Relay serving that reward epoch.
// The signer applies the eth_sign wrap.
//
// chainID is the Relay's sourceChainId; FDC2 only ships alongside a home Relay, where
// that is block.chainid.
//
// Not for the TEE's own signature, which recovers against the bare messageHash.
func relayPrefixedHash(chainID uint64, chainBound bool, messageHash common.Hash) common.Hash {
	if !chainBound {
		return fdc.RelayPrefixedHash(messageHash)
	}

	return fdc.ChainBoundRelayPrefixedHash(chainID, messageHash)
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
