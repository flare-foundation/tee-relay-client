package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeinstructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"gorm.io/gorm"
)

const startInterval = 100               // TODO: set it; indexing starts from the last block minus start interval
const requestInterval = 2 * time.Second // TODO: set the frequency of database requests

var TeeInstructionsSentSel common.Hash // set in init

func init() {
	teeInstructionsABI, err := teeinstructions.TeeInstructionsMetaData.GetAbi()
	if err != nil {
		panic("getting teeInstructionsABI abi: " + err.Error())
	}

	event, exits := teeInstructionsABI.Events["TeeInstructionsSent"]
	if !exits {
		panic("invalid event TeeInstructionsSent")
	}

	TeeInstructionsSentSel = event.ID
}

type Collector struct {
	DB              *gorm.DB // c-chain indexer db
	flareTeeManager common.Address
}

// New creates a new Collector that connects to database.
func New(db *gorm.DB, flareTeeManager common.Address) *Collector {
	collector := Collector{DB: db, flareTeeManager: flareTeeManager}

	return &collector
}

// Run waits for db to sync starts a goroutine in which collector listens to TeeInstructionsSent events and sends them to out channel.
func (c *Collector) Run(ctx context.Context, out chan<- []database.Log) error {
	syncParams := database.SyncParams{
		Retries:            30,
		OutOfSyncTolerance: 30 * time.Second,
		MaxSleepTime:       10 * time.Minute,
		MinSleepTime:       5 * time.Second,
	}

	err := database.WaitCIndexerToSync(ctx, c.DB, syncParams, logger.Logger())
	if err != nil {
		return fmt.Errorf("waiting for indexer to sync: %w", err)
	}

	go instructionsListener(ctx, c.DB, c.flareTeeManager, requestInterval, out)

	return nil
}

// instructionsListener repeatedly queries db for TeeInstructionsSent events emitted by the FlareTeeManager diamond and pushes them on to the instructions channel.
func instructionsListener(
	ctx context.Context,
	db *gorm.DB,
	flareTeeManager common.Address,
	listenerInterval time.Duration,
	out chan<- []database.Log,
) {
	trigger := time.NewTicker(listenerInterval)
	defer trigger.Stop()

	state, err := database.FetchState(ctx, db, nil)
	if err != nil {
		logger.Panicf("fetching initial state: %v", err)
	}

	lastQueriedIndex := max(0, state.Index-startInterval)

	params := database.LogsParams{
		Address: flareTeeManager,
		Topic0:  TeeInstructionsSentSel,
		From:    int64(lastQueriedIndex),
		To:      int64(state.Index),
	}

	for {
		select {
		case <-trigger.C:
		case <-ctx.Done():
			logger.Infof("instructionsListener exiting: %v", ctx.Err())
			return
		}

		state, err = database.FetchState(ctx, db, nil)
		if err != nil {
			logger.Errorf("fetching state: %v", err)
			continue
		}

		params.To = int64(state.Index)

		logs, err := database.FetchLogsByAddressAndTopic0BlockNumber(
			ctx, db, params,
		)
		if err != nil {
			logger.Errorf("fetching logs: %v", err)
			continue
		}

		params.From = params.To

		if len(logs) > 0 {
			select {
			case out <- logs:
			case <-ctx.Done():
				logger.Infof("instructionsListener exiting: %v", ctx.Err())
				return
			}
		}
	}
}
