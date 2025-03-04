package instructions

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/payment"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/registry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/walletmanager"
)

type InstructionClass int

const (
	Pl InstructionClass = iota
	Aug
	Sgn
	AugAndSign
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

// toBytes32 returns Solidity's bytes32(s)
//
// TODO optimize
func toBytes32(s string) (common.Hash, error) {
	if len(s) > 32 {
		return common.Hash{}, fmt.Errorf("String %s too long. At most 32 characters allowed", s)
	}

	b := []byte(s)
	x := [32]byte{}
	copy(x[:], b)

	return x, nil
}
