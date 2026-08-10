// Package config defines the relay client configuration and helpers to load and validate it.
package config

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math"
	"net/url"
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

// DefaultStartInterval is the default Collector.StartInterval.
const DefaultStartInterval int64 = 100

// CutoverUnscheduled is the RelayCutover.StartingRewardEpoch value that keeps every
// reward epoch on the pre-cutover digest.
const CutoverUnscheduled int64 = -1

// Config holds the relay client configuration.
type Config struct {
	DB              database.Config `toml:"db"`
	Logging         logger.Config   `toml:"logger"`
	FlareTeeManager common.Address  `toml:"flare_tee_manager"`

	ChainID    uint64    `toml:"chain_id"`
	IsCosigner bool      `toml:"is_cosigner"`
	Signer     Signer    `toml:"signer"` // credentials for signer
	FDC        FDC       `toml:"fdc"`
	Collector  Collector `toml:"collector"`

	RelayCutover RelayCutover `toml:"relay_cutover"`

	// AllowUnsafeURLs is set from the ALLOW_UNSAFE_URLS env var, never from the config file.
	// toml:"-" is what enforces that: BurntSushi matches field names case-insensitively, so
	// without it `AllowUnsafeURLs = true` in config.toml would disable SSRF protection.
	AllowUnsafeURLs bool `toml:"-"`
}

// Default returns a Config carrying the default values for optional fields.
// Decode into it — absent keys keep the default, present keys override it, so an
// explicit start_interval = 0 stays 0.
func Default() Config {
	return Config{
		Collector: Collector{StartInterval: DefaultStartInterval},
	}
}

// RelayCutover schedules the switch to the Relay that binds the source chain id into the
// FDC2 signature digest. Only the reward epoch is configured: the relay reads no Relay
// contract, so the new address is nothing it could use.
//
// Absent means the switch has already happened — every reward epoch is chain-bound. The
// pre-cutover digest is opted into, either from a known epoch or, until one is announced,
// with CutoverUnscheduled.
type RelayCutover struct {
	// StartingRewardEpoch is the first reward epoch signed with the chain-bound digest.
	// Signed so CutoverUnscheduled is expressible and a typo'd negative is rejected
	// rather than read as "never".
	StartingRewardEpoch int64 `toml:"starting_reward_epoch"`
}

// ChainBound reports whether rewardEpochID's FDC2 response is signed with the chain-bound
// digest rather than the pre-cutover one.
func (c RelayCutover) ChainBound(rewardEpochID uint32) bool {
	return c.StartingRewardEpoch >= 0 && int64(rewardEpochID) >= c.StartingRewardEpoch
}

// Collector holds the configuration of the indexer database listener.
type Collector struct {
	// StartInterval is how many blocks below the indexer's last block the initial
	// scan starts. Instructions in that window are reprocessed on every restart,
	// so it trades restart recovery against duplicate work; 0 starts at the last block.
	// Signed so a negative value fails CheckStartInterval — as uint64 it would decode
	// to 2^64-1 and silently backfill from the indexer's earliest retained block.
	StartInterval int64 `toml:"start_interval"`
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

// CheckRelayCutover returns an error if the cutover's starting reward epoch is neither
// CutoverUnscheduled nor a reward epoch an instruction can carry.
func (c *Config) CheckRelayCutover() error {
	e := c.RelayCutover.StartingRewardEpoch

	switch {
	case e < 0 && e != CutoverUnscheduled:
		return fmt.Errorf("relay_cutover.starting_reward_epoch must be %d (unscheduled) or non-negative, got %d", CutoverUnscheduled, e)
	case e > math.MaxUint32:
		return fmt.Errorf("relay_cutover.starting_reward_epoch %d exceeds the largest reward epoch id %d", e, uint32(math.MaxUint32))
	}

	return nil
}

// CheckStartInterval returns an error if the collector's StartInterval is negative.
func (c *Config) CheckStartInterval() error {
	if c.Collector.StartInterval < 0 {
		return fmt.Errorf("collector start_interval must not be negative, got %d", c.Collector.StartInterval)
	}

	return nil
}

// CheckQueues returns an error if any FDC queue has unset or degenerate retry parameters.
func (c *Config) CheckQueues() error {
	for name, q := range c.FDC.Queues {
		if q.MaxAttempts < 1 {
			// 0 (or omitted) silently turns every queue-level retry into a one-shot drop
			return fmt.Errorf("queue %q: max_attempts must be at least 1, got %d", name, q.MaxAttempts)
		}
		if q.MaxAttempts > 1 && q.TimeOff <= 0 {
			// retried items keep their weight: zero time_off burns all attempts instantly
			return fmt.Errorf("queue %q: time_off must be positive when max_attempts > 1", name)
		}
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
// A URL or key that only fails at request time surfaces as status 0 and is
// retried as transient; validating here fails startup instead.
func (c *Credentials) Check() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}

	u, err := url.Parse(c.URL)
	switch {
	case err != nil:
		return fmt.Errorf("invalid URL: %w", err)
	case u.Scheme != "http" && u.Scheme != "https":
		// catches "localhost:8080", which parses as scheme "localhost"
		return fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)
	case u.Host == "":
		return errors.New("URL host not set")
	}

	if len(c.Key) != 0 && len(c.KeyName) == 0 {
		return errors.New("unnamed api key")
	}
	if err := checkHeaderName(c.KeyName); err != nil {
		return fmt.Errorf("key_name: %w", err)
	}
	if strings.ContainsAny(c.Key, "\r\n") {
		return errors.New("key must not contain CR or LF")
	}

	return nil
}

// headerTokenSpecials are the non-alphanumeric RFC 7230 tchar bytes.
const headerTokenSpecials = "!#$%&'*+-.^_`|~"

// checkHeaderName returns an error if name cannot be sent as an HTTP header
// field name. An empty name is valid — no API key header is sent.
func checkHeaderName(name string) error {
	for i := range len(name) {
		b := name[i]
		switch {
		case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9':
		case strings.IndexByte(headerTokenSpecials, b) >= 0:
		default:
			return fmt.Errorf("byte %q is not a valid HTTP header name character", b)
		}
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
