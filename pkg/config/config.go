package config

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
)

type Config struct {
	DB                   database.Config `toml:"db"`
	Logging              logger.Config   `toml:"logger"`
	TeeExtensionRegistry common.Address  `toml:"tee_extension_registry"`
	Signer               Credentials     `toml:"signer"` // credentials for signer
	FTDC                 FTDC            `toml:"ftdc"`
}

func (c *Config) CheckAddress() error {
	zeroAddress := common.Address{}

	if c.TeeExtensionRegistry == zeroAddress {
		return errors.New("TeeExtensionRegistry address not set")
	}

	return nil
}

type Credentials struct {
	KeyName string `toml:"key_name"`
	Key     string `toml:"key"`
	URL     string `toml:"url"`
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
