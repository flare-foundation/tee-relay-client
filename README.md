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
  -p 127.0.0.1:8080:8080 \
  -v /etc/tee-relay/config.toml:/app/config.toml:ro \
  --env-file /etc/tee-relay/relay.env \
  tee-relay
```

The runtime stage is `debian:trixie`, to which the build adds the binary and CA certificates. It sets `WORKDIR /app` and
runs as uid 10001.

- **Config** — mount at `/app/config.toml`. Must be readable by uid 10001, or startup panics with `permission denied`.
- **Key** — `PRIVATE_KEY` is mandatory; pass it by env file or secret store, never in the image.
- **Contract address** — `FLARE_TEE_MANAGER_CONTRACT_ADDRESS` supplies `flare_tee_manager`, so the address can come
  from the deployment environment instead of the mounted config. Set in both places, the two must agree (see
  [FlareTeeManager address](#flareteemanager-address)).
- **Logs** — `/app` is not writable by uid 10001, so `logger.file` needs a mounted writable directory. Otherwise keep
  `console = true` and read `docker logs`.
- **Ports** — `8080`, which the image `EXPOSE`s, serves the [health endpoints](#health-endpoints). `EXPOSE` publishes
  nothing: map it with `-p` to reach it from the host. The image has no `curl`, so a Docker `HEALTHCHECK` cannot call
  the endpoints; use the orchestrator's HTTP probes.
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

Required. Address of the `FlareTeeManager` smart contract to listen to:

```toml
flare_tee_manager = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"
```

It can be set by the `FLARE_TEE_MANAGER_CONTRACT_ADDRESS` environment variable instead, in the same
`0x`-prefixed form. If both sources are used they must agree — a conflict fails startup rather than
picking a winner, since the address decides which contract's instructions the relay signs.
[`chain_id`](#chain-id) has no environment counterpart: it stays in the config file and must match the
network this address is deployed on.

### Chain ID

Required. The chain the relay signs for — it is part of every signature the relay produces, so it must match the network
the `flare_tee_manager` address is deployed on. Startup fails if it is unset or zero.

It must also equal the `Relay` contract's `sourceChainId`, which is what the chain hashes into the FDC2 signature digest.
The relay has no RPC connection and cannot read it, so the value is trusted as configured; the two always agree because
FDC2 is only deployed alongside a home `Relay`, where `sourceChainId` is forced to the network's own chain id.

```toml
chain_id = 14 # Flare mainnet
```

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

The relay signs each attestation response with the `Relay` Mode-2 digest, which binds the chain id —
`keccak256(chain_id ‖ 0x010000000000 ‖ messageHash)` — because that is what the `Relay` recovers signatures against.

#### Queues

```toml
[fdc.queues.exampleQueue]
max_dequeues_per_second = 100 # zero for unlimited
max_workers = 50              # zero for unlimited
max_attempts = 3
time_off = "2s"
```

Unlike `max_dequeues_per_second` and `max_workers` above, `max_attempts` has no
zero-means-unlimited reading: it must be at least 1, and startup fails on 0 rather
than treating it as "no retries"; 1 means a single attempt with no retry. `time_off`
must be positive when `max_attempts` is greater than 1; it is unconstrained at
`max_attempts = 1`.

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

Startup fails if `server.url` is empty, fails to parse, has a scheme other than
`http` or `https` (this is what rejects a bare `host:port` like `localhost:8080`,
which parses with scheme `localhost`), or has an empty host. `server.key_name` may
be empty only when `server.key` is empty; once `server.key` is set, `key_name` is
required and must be a valid HTTP header name (letters, digits, and
`` !#$%&'*+-.^_`|~ ``), and `key` must not contain CR or LF. Startup also fails if
a verifier's `type` or `source` is empty or longer than 32 bytes, or if two
verifiers share the same `type` and `source`.

#### Verifier API

The relay client queries a verifier with a single endpoint:

**`POST <server.url>`**

Request:

```json
{
  "attestationType": "<0x-prefixed 32-byte hex>",
  "sourceId": "<0x-prefixed 32-byte hex>",
  "requestBody": "<0x-prefixed hex>"
}
```

Response (HTTP 200):

```json
{
  "status": "<VERIFIED | RETRY | REJECTED>",
  "responseBody": "<0x-prefixed hex; only with status VERIFIED>",
  "message": "<reason; only with status RETRY or REJECTED>"
}
```

`status` is the verifier's verdict on the request:

- `VERIFIED` — the request is confirmed. `responseBody` carries the ABI-encoded attestation response and must be nonempty and at most 100 KiB (the instruction size limit enforced by the TEEs); `message` must be empty.
- `RETRY` — the request cannot be decided yet (e.g. the queried data is not yet final); `message` says why and `responseBody` must be empty. The client re-enqueues the instruction: the queue retries it after `time_off`, up to `max_attempts` in total, then drops it.
- `REJECTED` — the request is invalid or unconfirmable; `message` says why and `responseBody` must be empty. The client drops the instruction without retrying.

An empty `responseBody` may also be encoded as JSON `null` or omitted entirely. Any other `status`, a `VERIFIED` response with an empty or oversized `responseBody`, or an undecodable body is a protocol error; the client treats it like `RETRY`.

A non-200 status means the request was not processed. Within a single query the client retries `408`, `429`, `5xx`, and transport failures (3 attempts, 5 s apart); any other status fails the query at once, and the failed query is again retried through the queue. A non-200 response can carry a diagnostic reason in its body, but only with `Content-Type: text/plain` — bodies of any other content type are discarded. Responses are read up to 1 MiB; the client truncates `message` to 1 KiB.

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

### Health endpoints

On by default: the relay serves the three Kubernetes-shaped probes on port `8080`, on all interfaces — the same names
and codes as tee-proxy's internal port. `8080` is the port the provided Dockerfile `EXPOSE`s.

```toml
[health]
disabled = false           # default; true turns the endpoints off and opens no port
port = 8080                # default
max_indexer_lag = "30s"    # default; /ready answers 503 once the indexer's last block is older than this
```

| endpoint | question | `200` | `503` |
| --- | --- | --- | --- |
| `GET /healthy` | is the process up | always, once the listener is bound | never |
| `GET /startup` | has the pipeline started | once the collector, router and sender are running; stays `200` | during the database connect and the indexer sync wait |
| `GET /ready` | can it process instructions now | started, indexer state observed, and the indexer's last block at most `max_indexer_lag` old | not started, nothing observed yet, or the block is stale |

Bodies are empty on `200`; a `503` carries a one-line reason, for example
`indexer block too old: block 12345678 is 1m35s old, max 30s`. The readiness signal is the indexer state the collector
already fetches every 2 s, so probes never query the database. One rule covers three faults, because all of them stop
the observed block timestamp from advancing: a stalled indexer, an unreachable indexer database, and a dead collector
loop. The lag is the age of the last _block's_ timestamp — the same measure startup uses — so a halted chain reads as
not ready with a perfectly healthy indexer.

```yaml
# 60 min of sync wait before a restart; the relay itself gives up after ~5 h worst case
startupProbe:   { httpGet: { path: /startup, port: 8080 }, periodSeconds: 30, failureThreshold: 120 }
livenessProbe:  { httpGet: { path: /healthy, port: 8080 }, periodSeconds: 10, failureThreshold: 3 }
readinessProbe: { httpGet: { path: /ready,   port: 8080 }, periodSeconds: 10, failureThreshold: 3 }
```

- Point `livenessProbe` at `/healthy`, never at `/ready`: `/ready` is `503` for the whole sync wait, and a liveness
  probe there restarts the pod mid-sync in a loop.
- The `startupProbe` budget is the sync wait the deployment tolerates. The relay's own ceiling is long — the sync wait
  retries 30 times with sleeps of up to 10 minutes, so an indexer an hour behind is waited on for about 90 minutes
  before the process exits — and a shorter budget does not fail the relay, it restarts it in a loop, each restart
  beginning the wait from zero.
- `disabled = true` logs a warning at startup. Nothing listens, so a probe gets connection refused rather than a status
  code, and probes configured against a disabled relay fail liveness from the first check. Configure both or neither.
- A `port` other than the default `8080` logs a warning at startup. The provided Dockerfile `EXPOSE`s `8080`, so
  probes and port mappings must use the configured port.
- `port` and `max_indexer_lag` have no effect while disabled.
- A busy port fails startup — when another process on the host holds `8080` (run directly or with host networking), or
  another container in the same pod does, move it with `port` or set `disabled = true`. A port outside 1–65535 is rejected when the config is read.
- No authentication and no TLS: the port must not be reachable from outside the pod/host. The only data exposed is an
  indexer block height. Run outside a container, the relay listens on all of the host's interfaces, so firewall it.
- The runtime image has no `curl`, so a Docker `HEALTHCHECK` or compose `healthcheck` cannot call the endpoints from
  inside the container.

## Environment variables

| Variable                             | Required                          | Description                                                                                                                                                             |
| ------------------------------------ | --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PRIVATE_KEY`                        | When `signer.local = true`        | Private key for local signing. Name is configurable via `signer.private_key_variable`. Must be a `0x`-prefixed 32-byte hex string.                                      |
| `FLARE_TEE_MANAGER_CONTRACT_ADDRESS` | When `flare_tee_manager` is unset | Address of the `FlareTeeManager` contract, `0x`-prefixed. Startup fails if the value is not an address, is zero, or contradicts `flare_tee_manager` in the config file. |
| `ALLOW_UNSAFE_URLS`                  | No                                | Set to `true` to disable SSRF protection on backup and TEE sender URLs. Intended for local end-to-end testing only. A warning is logged at startup when enabled.        |
