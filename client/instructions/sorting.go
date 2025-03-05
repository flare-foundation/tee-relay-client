package instructions

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/payment"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/registry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/walletmanager"
)

// Instruction Class refers to an implementation of Instruction interface
//
//   - Pl -> Plain
//   - Aug -> Augment
type InstructionClass int

const (
	Pl InstructionClass = iota
	Aug
)

// OPToClass is a mapping from OPCommand to InstructionClass
var OPToClass map[common.Hash]InstructionClass

var plainCommands = []string{
	// REG
	string(registry.ToPauseForUpgrade),
	string(registry.ReplicateFrom),

	// WALLET
	string(walletmanager.KeyGenerate),
	string(walletmanager.KeyDelete),
	string(walletmanager.KeyMachineBackup),
	string(walletmanager.KeyMachineRestore),
	string(walletmanager.KeyMachineBackupRemove),
	string(walletmanager.KeyCustodianBackup),
	string(walletmanager.KeyCustodianRestore),
}

var augmentCommands = []string{
	// PAY
	string(payment.Pay),
	string(payment.Reissue),
}

func init() {
	OPToClass = make(map[common.Hash]InstructionClass)

	for j := range plainCommands {
		hexCommand, err := toBytes32(plainCommands[j])
		if err != nil {
			logger.Panicf("populating OPToClass: %v", err)
		}
		OPToClass[hexCommand] = Pl
	}

	for j := range augmentCommands {
		hexCommand, err := toBytes32(plainCommands[j])
		if err != nil {
			logger.Panicf("populating OPToClass: %v", err)
		}
		OPToClass[hexCommand] = Aug
	}
}

// toBytes32 returns Solidity's bytes32(s) ([]byte(s) appended with zeros to length 32)
//
// String s can be at most 32 characters long, otherwise an error is returned.
func toBytes32(s string) (common.Hash, error) {
	if len(s) > 32 {
		return common.Hash{}, fmt.Errorf("String %s too long. At most 32 characters allowed", s)
	}
	x := [32]byte{}
	copy(x[:], s)

	return x, nil
}
