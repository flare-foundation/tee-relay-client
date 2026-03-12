package collector

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/teeextensionregistry"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"gorm.io/gorm"
)

const startInterval = 100               // TODO: set it; indexing starts from the last block minus start interval
const requestInterval = 2 * time.Second // TODO: set the frequency of database requests

var TeeInstructionsSentSel common.Hash // set in init

func init() {
	teeExtensionRegistryABI, err := teeextensionregistry.TeeExtensionRegistryMetaData.GetAbi()
	if err != nil {
		logger.Panicf("getting teeExtensionRegistryABI abi: %v", err)
	}

	event, exits := teeExtensionRegistryABI.Events["TeeInstructionsSent"]
	if !exits {
		logger.Panicf("invalid event TeeInstructionsSent")
	}

	TeeInstructionsSentSel = event.ID
}

type Collector struct {
	DB                   *gorm.DB // c-chain indexer db
	teeExtensionRegistry common.Address
}

// New creates a new Collector that connects to database.
func New(db *gorm.DB, teeExtensionRegistry common.Address) *Collector {
	collector := Collector{DB: db, teeExtensionRegistry: teeExtensionRegistry}

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

	err := database.WaitCIndexerToSync(ctx, c.DB, syncParams, logger.GetLogger())
	if err != nil {
		return err
	}

	go instructionsListener(ctx, c.DB, c.teeExtensionRegistry, requestInterval, out)

	return nil
}

// instructionsListener repeatedly queries db for TeeInstructionsSent events emitted by TeeExtensionRegistry smart contracts and pushes them on to the instructions instructions channel.
func instructionsListener(
	ctx context.Context,
	db *gorm.DB,
	teeExtensionRegistry common.Address,
	listenerInterval time.Duration,
	out chan<- []database.Log,
) {
	trigger := time.NewTicker(listenerInterval)
	defer trigger.Stop()

	state, err := database.FetchState(ctx, db, nil)
	if err != nil {
		logger.Panic("fetch initial state error:", err)
	}

	lastQueriedIndex := max(0, state.Index-startInterval)

	params := database.LogsParams{
		Address: teeExtensionRegistry,
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
