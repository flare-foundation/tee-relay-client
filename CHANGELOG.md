# Changelog

## [Unreleased]

### Changed

- The FDC2 attestation-response signature binds the chain id from a per-chain breaking reward epoch on: the digest becomes `keccak256(chainID ‖ 0x010000000000 ‖ messageHash)` instead of `keccak256(0x010000000000 ‖ messageHash)`, matching the new Relay's Mode-2 digest (`Relay.relay()`) and `Fdc2ProofVerification.toCosignersMessageHash`, which the chain recovers both the data-provider and the cosigner signature against. The form is chosen per instruction from the reward epoch the event carries, so a response for an epoch the old Relay still serves keeps the pre-cutover digest and no round is signed twice. The boundary is a compiled-in table (`internal/router/processors/digest.go`), not configuration: Flare is chain-bound from its first epoch, because no relay client runs there until the new Relay is deployed; Coston, Coston2 and Songbird carry placeholders until their epochs are announced, and until then sign the pre-cutover form and warn at startup; a chain absent from the table is chain-bound throughout. Nothing else the relay signs changes — instruction and backup signatures already carry the chain id inside the `SignedPayload` envelope, and the relay reads no Relay contract, so it needs none of the signing-policy, randomness or address-cutover handling the FSP client does. TEE machines verify these signatures inside the enclave and gate on the same boundary, so their table must carry the same epochs; a disagreement rejects every response on one side of it.

### Security

- Go toolchain patched to 1.25.13, clearing four standard-library advisories `govulncheck` reports as reachable from this code on 1.25.12: GO-2026-6218 (`net/url`) and GO-2026-5026 (`net/http`, the vendored `x/net/idna`) through the backup fetch, GO-2026-6090 (`crypto/tls`) through the backup fetch, the indexer connection and the external signer client, and GO-2026-5972 (`encoding/asn1`) through the ECIES decryption the package pulls in. The version is pinned in `go.mod`, `.gitlab-ci.yml` and the `Dockerfile`, whose builder image digest moves with it.
