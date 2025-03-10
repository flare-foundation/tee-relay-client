package instructions

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/payment"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/registry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/wallet"
	"github.com/flare-foundation/tee-relay-client/utils"
)

// Instruction Class refers to an implementation of Instruction interface
//
//   - Pl -> Plain
//   - Aug -> Augment
type InstructionClass int

const (
	InvalidInstructionClass InstructionClass = iota
	Pl
	Aug
)

// OPToInstClass is a mapping from OPCommand to InstructionClass
var OPToInstClass map[common.Hash]InstructionClass

var plainCommands = []string{
	// REG
	string(registry.ToPauseForUpgrade),
	string(registry.ReplicateFrom),

	// WALLET
	string(wallet.KeyGenerate),
	string(wallet.KeyDelete),
	string(wallet.KeyMachineBackup),
	string(wallet.KeyMachineRestore),
	string(wallet.KeyMachineBackupRemove),
	string(wallet.KeyCustodianBackup),
	string(wallet.KeyCustodianRestore),
}

var augmentCommands = []string{
	// PAY
	string(payment.Pay),
	string(payment.Reissue),
}

func init() {
	OPToInstClass = make(map[common.Hash]InstructionClass)

	for j := range plainCommands {
		hexCommand, err := utils.ToBytes32(plainCommands[j])
		if err != nil {
			logger.Panicf("populating OPToClass: %v", err)
		}
		OPToInstClass[hexCommand] = Pl
	}

	for j := range augmentCommands {
		hexCommand, err := utils.ToBytes32(plainCommands[j])
		if err != nil {
			logger.Panicf("populating OPToClass: %v", err)
		}
		OPToInstClass[hexCommand] = Aug
	}
}
