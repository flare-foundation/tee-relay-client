package config

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/call"
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

func (c Credentials) Check() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	return nil
}

func (c Credentials) APIKey() call.APIKey {
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

	at, err := toBytes32(attType)
	if err != nil {
		return x, fmt.Errorf("att type: %v", err)
	}
	si, err := toBytes32(sourceID)
	if err != nil {
		return x, fmt.Errorf("source ID: %v", err)
	}

	copy(x[0:32], at.Bytes())
	copy(x[32:], si.Bytes())

	return x, nil
}

// toBytes32 returns Solidity's bytes32(s) ([]byte(s) appended with zeros to length 32)
// String s can be at most 32 characters long, otherwise an error is returned.
func toBytes32(s string) (common.Hash, error) {
	if len(s) > 32 {
		return common.Hash{}, fmt.Errorf("string %s too long. At most 32 characters allowed", s)
	}
	x := [32]byte{}
	copy(x[:], s)

	return x, nil
}

// PrivateKeyFromEnv retrieves private key from environment variable
// or from default environment variable if variableName is empty
// It returns an error if the private key is not valid or the environment variable is not set.
func PrivateKeyFromEnv(variableName string) (*ecdsa.PrivateKey, error) {
	if len(variableName) == 0 {
		variableName = DefaultPrivateKeyVariable
	}
	skStr := os.Getenv(variableName)

	skStr, _ = strings.CutPrefix(skStr, "0x")
	skStr, _ = strings.CutPrefix(skStr, "0X")

	if len(skStr)%2 != 0 {
		skStr = "0" + skStr
	}

	skB, err := hex.DecodeString(skStr)
	if err != nil {
		return nil, fmt.Errorf("invalid string for private key")
	}

	skB = prefixTo32Bytes(skB)

	return crypto.ToECDSA(skB)
}

func prefixTo32Bytes(s []byte) []byte {
	if len(s) >= 32 {
		return s
	}

	rs := make([]byte, 32-len(s), 32)

	rs = append(rs, s...)

	return rs
}
