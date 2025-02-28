package collector

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/tee-relay-client/client/config"
	"gorm.io/gorm"
)

var TeeInstructionsSentSel common.Hash // set in init

func init() {
	teeInstructionsABI, err := teeinstructions.TeeInstructionsMetaData.GetAbi()
	if err != nil {
		logger.Panicf("getting teeInstructions abi: %v", err)
	}

	event, exits := teeInstructionsABI.Events["TeeInstructionsSent"]
	if !exits {
		logger.Panicf("invalid event TeeInstructionsSent")
	}

	TeeInstructionsSentSel = event.ID
}

type Collector struct {
	TeeInstructions common.Address
	DB              *gorm.DB
}

// New creates new Collector from user config
func New(cfg *config.User) *Collector {
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		logger.Panic("Could not connect to database:", err)
	}

	collector := Collector{DB: db}

	return &collector
}

// InstructionsListener repeatedly queries db for teeInstructionsSent events emitted by teeInstructions smart contracts and pushes them on to the instructions instructions channel.
func InstructionsListener(
	ctx context.Context,
	db *gorm.DB,
	teeInstructions common.Address,
	listenerInterval time.Duration,
	instructions chan<- []database.Log,
) {
	trigger := time.NewTicker(listenerInterval)

	state, err := database.FetchState(ctx, db, nil)
	if err != nil {
		logger.Panic("fetch initial state error:", err)
	}

	lastQueriedIndex := state.Index - 100 //TODO from where we start

	params := database.LogsParams{
		Address: teeInstructions,
		Topic0:  TeeInstructionsSentSel,
		From:    int64(lastQueriedIndex),
		To:      int64(state.Index),
	}

	for {
		select {
		case <-trigger.C:
		case <-ctx.Done():
			logger.Info("AttestationRequestListener exiting:", ctx.Err())
			return
		}

		state, err = database.FetchState(ctx, db, nil)
		if err != nil {
			logger.Error("fetch state error:", err)
			continue
		}

		params.To = int64(state.Index)

		logs, err := database.FetchLogsByAddressAndTopic0BlockNumber(
			ctx, db, params,
		)
		if err != nil {
			logger.Error("fetch logs error:", err)
			continue
		}

		params.From = params.To

		if len(logs) > 0 {
			select {
			case instructions <- logs:
			case <-ctx.Done():
				logger.Info("AttestationRequestListener exiting:", ctx.Err())
				return
			}
		}
	}
}
