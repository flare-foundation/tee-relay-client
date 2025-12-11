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

const DefaultPrivateKeyVariable = "PRIVATE_KEY"

type Config struct {
	DB                   database.Config `toml:"db"`
	Logging              logger.Config   `toml:"logger"`
	TeeExtensionRegistry common.Address  `toml:"tee_extension_registry"`

	IsCosigner bool   `toml:"is_cosigner"`
	Signer     Signer `toml:"signer"` // credentials for signer
	FTDC       FTDC   `toml:"ftdc"`
}

func (c *Config) CheckAddress() error {
	zeroAddress := common.Address{}

	if c.TeeExtensionRegistry == zeroAddress {
		return errors.New("TeeExtensionRegistry address not set")
	}

	return nil
}

// Signer holds credentials for the signer.
// If Local is true, privet key the set env variable is used for local signing.
type Signer struct {
	Credentials

	Local              bool   `toml:"local"`
	PrivateKeyVariable string `toml:"private_key_variable"`
}

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

func (c *Credentials) APIKey() call.APIKey {
	return call.APIKey{
		Name: c.KeyName,
		Key:  c.Key,
	}
}

type FTDC struct {
	Queues    map[string]priority.Params `toml:"queues"`
	Verifiers map[string]Verifier        `toml:"verifiers"`
}

type Verifier struct {
	AttType   string      `toml:"type"`
	SourceID  string      `toml:"source"`
	Server    Credentials `toml:"server"`
	QueueName string      `toml:"queue"`
}

func (v *Verifier) AttTypeAndSourceID() ([64]byte, error) {
	return JoinAttTypeAndSourceID(v.AttType, v.SourceID)
}

func JoinAttTypeAndSourceID(attType string, sourceID string) ([64]byte, error) {
	x := [64]byte{}

	at, err := convert.StringToCommonHash(attType)
	if err != nil {
		return x, fmt.Errorf("att type: %v", err)
	}
	si, err := convert.StringToCommonHash(sourceID)
	if err != nil {
		return x, fmt.Errorf("source ID: %v", err)
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
