<div align="center">
  <a href="https://flare.network/" target="blank">
    <img src="https://content.flare.network/Flare-2.svg" width="300" alt="Flare Logo" />
  </a>
  <br />
  <a href="CONTRIBUTING.md">Contributing</a>
  ·
  <a href="SECURITY.md">Security</a>
  ·
  <a href="CHANGELOG.md">Changelog</a>
</div>

# TEE relay client

TEE relay client is a connector between smart contracts on Flare's C-chain and TEE clients.
It listens to `TeeInstructionsSent` events emitted by the `FlareTeeManager` smart contract, processes them, and forwards them to the TEE nodes.

## Running

Build and run the relay client:

```shell
go build -o tee-relay ./cmd/main
./tee-relay
```

The binary expects `config.toml` to be present in the working directory.

A reachable C-chain indexer database is required: the relay connects to it at startup and exits if it cannot.

### Docker

No image is published. Build with the provided `Dockerfile`, which can be used as is:

```shell
docker build -t tee-relay .
docker run -d --name tee-relay \
  -v /etc/tee-relay/config.toml:/app/config.toml:ro \
  --env-file /etc/tee-relay/relay.env \
  tee-relay
```

The runtime stage is `debian:trixie`, to which the build adds the binary and CA certificates. It sets `WORKDIR /app` and
runs as uid 10001.

- **Config** — mount at `/app/config.toml`. Must be readable by uid 10001, or startup panics with `permission denied`.
- **Key** — `PRIVATE_KEY` is mandatory; pass it by env file or secret store, never in the image.
- **Logs** — `/app` is not writable by uid 10001, so `logger.file` needs a mounted writable directory. Otherwise keep
  `console = true` and read `docker logs`.
- **Ports** — none, and no health endpoint; liveness comes from the logs.
- **Stopping** — `SIGTERM` is handled, but in-flight instructions are not drained, and there is no durable cursor. They
  are re-collected after restart only if `[collector] start_interval` is greater than zero and the instruction's block
  is still inside that window (see [Collector](#collector)); queued FDC work is lost. To recover reliably, restart
  before the window moves past the affected blocks.

## Configurations

The configuration is read from the `config.toml` file.

Copy the example to get started:

```shell
cp config.toml.example config.toml
```

### Modes of operation

TEE relay client can be run in _provider_ or _cosigner_ mode, controlled by the `is_cosigner` field.

#### Provider mode

For Flare entities included in the current signing policy. The relay client processes all instructions.

```toml
is_cosigner = false # default
```

#### Cosigner mode

For cosigners defined by TEE protocols that are not in the signing policy. The relay client processes only instructions that include the address corresponding to the configured private key among the cosigners.

```toml
is_cosigner = true
```

### FlareTeeManager address

Address of the `FlareTeeManager` smart contract to listen to:

```toml
flare_tee_manager = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"
```

### Chain ID

Required. The chain the relay signs for — it is part of every signature the relay produces, so it must match the network
the `flare_tee_manager` address is deployed on. Startup fails if it is unset or zero.

It must also equal the `Relay` contract's `sourceChainId`, which is what the chain hashes into the FDC2 signature digest.
The relay has no RPC connection and cannot read it, so the value is trusted as configured; the two always agree because
FDC2 is only deployed alongside a home `Relay`, where `sourceChainId` is forced to the network's own chain id.

```toml
chain_id = 14 # Flare mainnet
```

### Relay cutover

Optional. The first reward epoch whose FDC2 attestation responses are signed with the chain-bound digest — see
[FDC2](#fdc2) for what the two digest forms are.

```toml
[relay_cutover]
starting_reward_epoch = 5451
```

| value | effect |
| --- | --- |
| omitted, or `0` | every reward epoch is chain-bound |
| `N > 0` | pre-cutover digest below `N`, chain-bound from `N` on |
| `-1` | pre-cutover digest at every reward epoch |

Omitting the block means the cutover has already happened, so a chain still awaiting it must say so with `-1` until its
epoch is announced. Only the epoch is configured: the new `Relay`'s address is not, because the relay reads no `Relay`
contract. The form in force is logged at startup, and `-1` is logged as a warning.

### Collector

```toml
[collector]
start_interval = 100 # defaults to 100 when omitted
```

`start_interval` is how many blocks below the indexer's last block the initial log scan starts.
Because the relay keeps no durable cursor, this window is rescanned on every restart: a larger
value recovers instructions missed while the relay was down, at the cost of reprocessing (re-signing
and re-sending) everything else in the window. Set it to `0` to start immediately after the last
block and never look back — the scan bound is exclusive, so the last block itself is not rescanned.

Must not be negative. A value larger than the indexer's current block height starts the scan at the
earliest block the indexer still retains, which reprocesses every instruction in it.

### C-chain indexer database

Credentials for the C-chain indexer database:

```toml
[db]
host = "localhost"
port = 3306
database = "flare_ftso_indexer"
username = "root"
password = "root"
log_queries = false
```

The database should be operated by C-chain indexer connected to desired chain.
The indexer should index `TeeInstructionsSent` events emitted by the FlareTeeManager diamond contract.

Connection-pool limits are optional and default to whatever `database/sql` uses. They live in a nested `[db.pool]` table —
`max_open_conns`, `max_idle_conns`, `conn_max_lifetime`, `conn_max_idle_time`. The relay queries the indexer from a single
goroutine, so tuning them is rarely useful. Note that placing these keys directly under `[db]` instead of `[db.pool]` is
rejected at startup as an unknown field.

### Signer

The relay client requires access to a private key — the signing policy key for providers, or the designated cosigning key for cosigners. The key is used to:

- sign instructions
- identify and decrypt packages for key recovery
- (cosigner mode only) identify relevant instructions

Two modes are defined, but **the external signer is not implemented yet** — use the local signer.

#### Local signer

The private key is held by the relay client itself, read from an environment variable at startup.

```toml
[signer]
local = true
private_key_variable = "PRIVATE_KEY" # name of the env var; defaults to "PRIVATE_KEY"
```

Set the environment variable to a `0x`-prefixed 32-byte hex string:

```shell
export PRIVATE_KEY=0x<64 hex chars>
```

#### External signer

> **Not implemented yet.** External signing is not available in this release: no signer service
> is deployed or operated for it, and the path is untested end to end. Run with
> `signer.local = true`. The endpoints below specify the interface a future service must satisfy.

The private key is held by an external signer service (typically the FSP client).

```toml
[signer]
local = false
url = "https://signer-host:port"
key_name = "X-API-KEY"
key = "<api-key>"
```

The external signer URL is operator-controlled and may point to a local address.
Use `https` for any non-loopback host: the API key and the `/decrypt` plaintext
(key-split secret material) would otherwise transit in cleartext. `http` is
acceptable only for a loopback address.

The signer service must expose three endpoints:

**`POST /sign`**

Request:

```json
{ "hashes": ["<0x-prefixed 32-byte hex>", ...] }
```

Response:

```json
{ "signatures": ["<0x-prefixed 65-byte hex>", ...] }
```

The `j`-th signature is the ECDSA personal signature ([ERC-191](https://eips.ethereum.org/EIPS/eip-191) version `0x45`) of the `j`-th hash.

**`POST /decrypt`**

Request:

```json
{ "cipher": "<0x-prefixed hex ciphertext>" }
```

Response:

```json
{ "plain": "<0x-prefixed hex plaintext>" }
```

`plain` is the ECIES decryption of `cipher`.

**`GET /id`**

Response:

```json
{ "x": "<0x-prefixed 32-byte hex>", "y": "<0x-prefixed 32-byte hex>" }
```

Returns the secp256k1 public key coordinates of the key used for signing and decryption.

A prototype of such a service exists in [`go-flare-common/pkg/tee/signer`](https://github.com/flare-foundation/go-flare-common/tree/main/pkg/tee/signer). It is not part of a supported deployment.

### FDC2

One of the protocols operated on Flare TEEs is FDC2 (Flare Data Connector). FDC2 instructions require additional processing — the relay client must query designated verifier servers to obtain attestation responses.

A verifier must be configured for each supported (attestation type, source) pair. To avoid overloading servers, each verifier is backed by a queue. Multiple verifiers can share a queue when they point to the same server.

The relay signs each attestation response with the `Relay` Mode-2 digest of the reward epoch the instruction carries. From the configured starting reward epoch on, that digest binds the chain id — `keccak256(chain_id ‖ 0x010000000000 ‖ messageHash)` — because the new `Relay` recovers signatures against it; before that epoch the pre-cutover form, without the chain id, is used.

The boundary comes from `[relay_cutover]` (see [Relay cutover](#relay-cutover)), not from the binary.

TEE machines verify this signature inside the enclave before adding their own, and they compute the chain-bound digest
unconditionally — they carry no boundary of their own. The configured epoch is therefore only correct if it is the one
the chain's contract batch and TEE fleet swap land in: below it the pre-cutover fleet serves the chain, at and above it
the new one. A configured epoch that does not match the swap rejects every response on one side of it.

#### Queues

```toml
[fdc.queues.exampleQueue]
max_dequeues_per_second = 100 # zero for unlimited
max_workers = 50              # zero for unlimited
max_attempts = 3
time_off = "2s"
```

#### Verifiers

```toml
[fdc.verifiers.example]
type = "AttestationTypeExampleName"
source = "ExampleSource"
queue = "exampleQueue"
server.url = "https://verifier-host/path/to/endpoint"
server.key_name = "X-API-KEY"
server.key = "exampleKey"
```

Verifier server URLs are operator-controlled and may point to local addresses.
Use `https` for any non-loopback host so the API key and request/response bodies
are not sent in cleartext; `http` is acceptable only for a loopback address.

### Logging

```toml
[logger]
level = "INFO"       # DEBUG, INFO, WARN, ERROR (DPANIC, PANIC and FATAL are also accepted)
console = true       # write logs to stdout
file = ""            # path to log file; empty disables file logging
max_file_size = 10   # max log file size in MB before rotation
max_backups = 10     # number of rotated files to keep
max_age_days = 30    # days to keep rotated files
```

## Environment variables

| Variable            | Required                   | Description                                                                                                                                                      |
| ------------------- | -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PRIVATE_KEY`       | When `signer.local = true` | Private key for local signing. Name is configurable via `signer.private_key_variable`. Must be a `0x`-prefixed 32-byte hex string.                               |
| `ALLOW_UNSAFE_URLS` | No                         | Set to `true` to disable SSRF protection on backup and TEE sender URLs. Intended for local end-to-end testing only. A warning is logged at startup when enabled. |
