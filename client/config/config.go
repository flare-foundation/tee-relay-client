package config

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/tee-relay-client/utils"
)

type Config struct {
	DB              database.Config `toml:"db"`
	Logging         logger.Config   `toml:"logger"`
	TeeInstructions common.Address  `toml:"tee_instructions"`
	Signer          Credentials     `toml:"signer"` // credentials for signer
	FTDC            FTDC            `toml:"ftdc"`
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
	Verifiers []Verifier                 `toml:"verifiers"`
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

	at, err := utils.ToBytes32(attType)
	if err != nil {
		return x, fmt.Errorf("att type: %v", err)
	}
	si, err := utils.ToBytes32(sourceID)
	if err != nil {
		return x, fmt.Errorf("source ID: %v", err)
	}

	copy(x[0:32], at.Bytes())
	copy(x[32:], si.Bytes())

	return x, nil
}
