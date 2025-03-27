<!-- <p align="left">
  <a href="https://flare.network/" target="blank"><img src="https://flare.network/wp-content/uploads/Artboard-1-1.svg" width="400" height="300" alt="Flare Logo" /></a>
</p> -->

# TEE relay client

TEE relay client is a connector between contracts on Flare's C-chain and TEE clients.
It listens to events emitted by TeeInstructions smart contract, processes them and sends them to the TEE nodes.

TEE relay client should be run by all entities included in Flare's signing policy.

## Configurations

Configurations are set in config.toml file.

```toml
# C-chain indexer db credentials
[db]
host = "localhost"
port = 3306
database = "flare_ftso_indexer"
username = "root"
password = "root"
log_queries = false
```

```toml
# logging config
[logger]
max_file_size = 10 # 10MB
file = ""
level = "DEBUG"
console = true
```

```toml
# credentials for signer
[signer]
url = ""
key_name = ""
key = ""

# credentials for xrp augmenter
[xrp]
url = ""
key_name = ""
key = ""

# credentials for btc augmenter
[btc]
url = ""
key_name = ""
key = ""
```
