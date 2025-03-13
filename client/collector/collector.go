package collector

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
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
	DB              *gorm.DB // c-chain indexer db
	teeInstructions common.Address
}

// New creates a new Collector that connects to database.
func New(cfg *database.Config, teeInstructions common.Address) *Collector {
	db, err := database.Connect(cfg)
	if err != nil {
		logger.Panic("Could not connect to database:", err)
	}

	collector := Collector{DB: db, teeInstructions: teeInstructions}

	return &collector
}

// Run waits for db to sync starts a goroutine in which collector listens to TeeInstructionsSent events and sends them to out channel.
func Run(ctx context.Context, c *Collector, out chan<- []database.Log) {
	syncParams := database.SyncParams{
		Retries:            30,
		OutOfSyncTolerance: 10 * time.Second,
		MaxSleepTime:       10 * time.Minute,
		MinSleepTime:       5 * time.Second,
	}

	database.WaitCIndexerToSync(ctx, c.DB, syncParams)

	go instructionsListener(ctx, c.DB, c.teeInstructions, 5*time.Second, out) // todo interval length
}

// instructionsListener repeatedly queries db for teeInstructionsSent events emitted by teeInstructions smart contracts and pushes them on to the instructions instructions channel.
func instructionsListener(
	ctx context.Context,
	db *gorm.DB,
	teeInstructions common.Address,
	listenerInterval time.Duration,
	out chan<- []database.Log,
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
			logger.Info("instructionsListener exiting:", ctx.Err())
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
			case out <- logs:
			case <-ctx.Done():
				logger.Info("instructionsListener exiting:", ctx.Err())
				return
			}
		}
	}
}
