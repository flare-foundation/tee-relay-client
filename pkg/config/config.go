// Package config defines the relay client configuration and helpers to load and validate it.
package config

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/convert"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
)

// DefaultPrivateKeyVariable is the default environment variable name holding the signer private key.
const DefaultPrivateKeyVariable = "PRIVATE_KEY"

// Config holds the relay client configuration.
type Config struct {
	DB              database.Config `toml:"db"`
	Logging         logger.Config   `toml:"logger"`
	FlareTeeManager common.Address  `toml:"flare_tee_manager"`

	ChainID         uint64 `toml:"chain_id"`
	IsCosigner      bool   `toml:"is_cosigner"`
	Signer          Signer `toml:"signer"` // credentials for signer
	FDC             FDC    `toml:"fdc"`
	AllowUnsafeURLs bool   // set from ALLOW_UNSAFE_URLS env var — never from config file
}

// CheckAddress returns an error if the FlareTeeManager address is unset.
func (c *Config) CheckAddress() error {
	zeroAddress := common.Address{}

	if c.FlareTeeManager == zeroAddress {
		return errors.New("FlareTeeManager address not set")
	}

	return nil
}

// CheckChainID returns an error if the ChainID is zero.
func (c *Config) CheckChainID() error {
	if c.ChainID == 0 {
		return errors.New("chain id should be a positive integer")
	}

	return nil
}

// Signer holds credentials for the signer.
// If Local is true, the private key from the set env variable is used for local signing.
type Signer struct {
	Credentials

	Local              bool   `toml:"local"`
	PrivateKeyVariable string `toml:"private_key_variable"`
}

// Credentials holds the API key and URL used to reach a server.
type Credentials struct {
	KeyName string `toml:"key_name"`
	Key     string `toml:"key"`
	URL     string `toml:"url"`
}

// Check checks if the credentials are valid.
func (c *Credentials) Check() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}

	if len(c.Key) != 0 && len(c.KeyName) == 0 {
		return errors.New("unnamed api key")
	}

	return nil
}

// APIKey returns the credentials as a call.APIKey.
func (c *Credentials) APIKey() call.APIKey {
	return call.APIKey{
		Name: c.KeyName,
		Key:  c.Key,
	}
}

// FDC holds the FDC queue and verifier configuration.
type FDC struct {
	Queues    map[string]priority.Params `toml:"queues"`
	Verifiers map[string]Verifier        `toml:"verifiers"`
}

// Verifier holds the configuration for a single attestation verifier.
type Verifier struct {
	AttType   string      `toml:"type"`
	SourceID  string      `toml:"source"`
	Server    Credentials `toml:"server"`
	QueueName string      `toml:"queue"`
}

// AttTypeAndSourceID returns the verifier's attestation type and source ID joined into a 64-byte array.
func (v *Verifier) AttTypeAndSourceID() ([64]byte, error) {
	return JoinAttTypeAndSourceID(v.AttType, v.SourceID)
}

// JoinAttTypeAndSourceID joins an attestation type and source ID into a 64-byte array.
func JoinAttTypeAndSourceID(attType string, sourceID string) ([64]byte, error) {
	x := [64]byte{}

	at, err := convert.StringToCommonHash(attType)
	if err != nil {
		return x, fmt.Errorf("att type: %w", err)
	}
	si, err := convert.StringToCommonHash(sourceID)
	if err != nil {
		return x, fmt.Errorf("source ID: %w", err)
	}

	copy(x[0:32], at.Bytes())
	copy(x[32:], si.Bytes())

	return x, nil
}

// PrivateKeyFromEnv retrieves private key from environment variable
// or from default environment variable if variableName is empty.
// It returns an error if the private key is not valid or the environment variable is not set.
func PrivateKeyFromEnv(variableName string) (*ecdsa.PrivateKey, error) {
	if len(variableName) == 0 {
		variableName = DefaultPrivateKeyVariable
	}

	skStr, exists := os.LookupEnv(variableName)
	if !exists {
		return nil, errors.New("private key not set")
	}

	skStr, _ = strings.CutPrefix(skStr, "0x")
	skStr, _ = strings.CutPrefix(skStr, "0X")

	return crypto.HexToECDSA(skStr)
}
