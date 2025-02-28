package instructions

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/events"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

// teeFilterer is only used for TeeInstructionSent logs parsing. Set in init().
var teeFilterer *teeinstructions.TeeInstructionsFilterer

// init sets the fdcFilterer
func init() {
	var err error

	teeFilterer, err = teeinstructions.NewTeeInstructionsFilterer(common.Address{}, nil)
	if err != nil {
		logger.Panic("cannot get fdc contract:", err)
	}
}

// ParseTeeInstructionsSent tries to parse ParseTeeInstructionsSent log as stored in the c-chain indexer database
func ParseTeeInstructionsSent(instruction database.Log) (*teeinstructions.TeeInstructionsTeeInstructionsSent, error) {
	chainLog, err := events.ConvertDatabaseLogToChainLog(instruction)
	if err != nil {
		return nil, err
	}

	return teeFilterer.ParseTeeInstructionsSent(*chainLog)
}

type Instruction interface {
	Augment()
	Sign()
}
