<p align="left">
  <a href="https://flare.network/" target="blank"><img src="https://content.flare.network/Flare-2.svg" width="500" height="100" alt="Flare Logo" /></a>
</p>

# TEE relay client

TEE relay client is a connector between smart contracts on Flare's C-chain and TEE clients.
It listens to TeeInstructionsSent events emitted by TeeExtensionRegistry smart contract, processes them and sends them to the TEE nodes.

## Modes of operation.

TEE relay client can be run in _provider_ or _cosigner_ mode.
The boolean field `is_cosigner` in configs sets the mode - _true_ for cosigner, _false_ for provider.
The default mode is provider.

### Provider mode

The provider mode is for all Flare entities that are included in the current signing policy.
In this mode, the relay client considers all instructions.

```toml
is_cosigner = false # default is false
```

### Cosigner mode

The cosigner mode is for all cosigners defined by protocols that use TEEs that are not included in the signing policy.
In this mode, the relay client considers only the instructions that contain address corresponding to the used private key among cosigners.

```toml
is_cosigner = true  # default is false
```

## Signer

The relay client needs access to a private key.
In the case of providers, to the signing policy key.
In the case of cosigners, to the designated private key for cosigning.

It needs the private key to do the following:

- sign the instructions
- identify and decipher packages for key recovery
- (only in cosigner mode) identify the relevant instructions

There are two ways to achieve this.

### Local signer

Private key can be held by the relay client.
It is read from env variable.

To enable the following should be in the config.

```toml
[signer]
local = true
private_key_variable = "ENV_PRIVATE_KEY_VARIABLE" # default is "PRIVATE_KEY"
```

Private key should be held as an env variable under the set name (private_key_variable) as 0x (or 0X) prefixed 32-byte hex string.

### External signer

Private key can be held by an external signer (usually FSP client) that serves endpoints `/sign`, `/decrypt`, and `/id`.

To enable the following should be in the config.

```toml
[signer]
local = false
url =
key_name = "X-API-KEY"
key =
```

## FTDC

On of the protocols operated on Flare TEEs is FTDC (Flare TEE Data Connector).
The instructions for FTDC have to be additionally processed by the relay clients - they have to be sent to designated verifier servers to get attestation responses.

For each supported pair of attestation type and source an access to a verifier should be configured.
To avoid overloading the servers, each verifier has a queue.
A queue can be shared by more verifiers, which should be done if more verifiers are hosted on the same server.

### Queues

To configure a queue with name "serverX" add the following to the configurations:

```toml
[ftdc.queues.exampleQueue]
max_dequeues_per_second = 100 # zero for unlimited
max_workers = 50              # zero for unlimited
max_attempts = 3
time_off = "2s"
```

### Verifiers

To configure a verifier for a pair of attestation type and source, and bind it to a queue add the following to the configuration:

```toml
[ftdc.verifiers.availability]
type = "AttestationTypeExampleName"
source = "ExampleSource"
queue = "exampleQueue"
server.url = "example.com/to/the/right/endpoint"
server.key_name = "X-API-KEY"
server.key = "exampleKey"
```

## Other Configurations

Address of TeeExtensionRegistry smart contract:

```toml
# address of
tee_extension_registry = "0xdE25c06982Ab8e4b6B4F910896E3f93Ac77FB44d"
```

C-chain indexer database credentials:

```toml
[db]
host = "localhost"
port = 3306
database = "flare_ftso_indexer"
username = "root"
password = "root"
log_queries = false
```

Logging configurations:

```toml
# logging config
[logger]
max_file_size = 10 # 10MB
file = ""
level = "INFO"
console = true
```
