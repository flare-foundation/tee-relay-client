// Package collector listens for TeeInstructionsSent events in the indexer database.
package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	teeinstructions "github.com/flare-foundation/go-flare-common/pkg/contracts/tee/instructions"
	"github.com/flare-foundation/go-flare-common/pkg/database"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const requestInterval = 2 * time.Second // TODO: set the frequency of database requests

// TeeInstructionsSentSel is the event selector (topic0) of the TeeInstructionsSent event.
var TeeInstructionsSentSel common.Hash // set in init

func init() {
	teeInstructionsABI, err := teeinstructions.InstructionsMetaData.GetAbi()
	if err != nil {
		panic("getting teeInstructionsABI abi: " + err.Error())
	}

	event, exits := teeInstructionsABI.Events["TeeInstructionsSent"]
	if !exits {
		panic("invalid event TeeInstructionsSent")
	}

	TeeInstructionsSentSel = event.ID
}

// Collector listens for TeeInstructionsSent events emitted by the FlareTeeManager in the indexer database.
type Collector struct {
	DB              *gorm.DB // c-chain indexer db
	flareTeeManager common.Address
	startInterval   int64
}

// New creates a new Collector that connects to database.
// startInterval is how many blocks below the indexer's last block the initial scan starts.
func New(db *gorm.DB, flareTeeManager common.Address, startInterval int64) *Collector {
	collector := Collector{DB: db, flareTeeManager: flareTeeManager, startInterval: startInterval}

	return &collector
}

// Run waits for db to sync starts a goroutine in which collector listens to TeeInstructionsSent events and sends them to out channel.
func (c *Collector) Run(ctx context.Context, wg *sync.WaitGroup, out chan<- []database.Log) error {
	syncParams := database.SyncParams{
		Retries:            30,
		OutOfSyncTolerance: 30 * time.Second,
		MaxSleepTime:       10 * time.Minute,
		MinSleepTime:       5 * time.Second,
	}

	// AddCallerSkip(-1) attributes WaitCIndexerToSync's lines to that call, not here.
	err := database.WaitCIndexerToSync(ctx, c.DB, syncParams, logger.Logger().WithOptions(zap.AddCallerSkip(-1)))
	if err != nil {
		return fmt.Errorf("waiting for indexer to sync: %w", err)
	}

	wg.Add(1)
	go instructionsListener(ctx, wg, c.DB, c.flareTeeManager, c.startInterval, requestInterval, out)

	return nil
}

// windowStart returns the lower bound (exclusive, per the (From, To] query) for
// the initial log scan: startInterval blocks below index, clamped at 0 so a fresh
// indexer (index < startInterval) does not scan from a negative block.
func windowStart(index uint64, startInterval int64) int64 {
	i := int64(index)
	if i < startInterval {
		return 0
	}

	return i - startInterval
}

// instructionsListener repeatedly queries db for TeeInstructionsSent events emitted by the FlareTeeManager diamond and pushes them on to the instructions channel.
func instructionsListener(
	ctx context.Context,
	wg *sync.WaitGroup,
	db *gorm.DB,
	flareTeeManager common.Address,
	startInterval int64,
	listenerInterval time.Duration,
	out chan<- []database.Log,
) {
	defer wg.Done()

	trigger := time.NewTicker(listenerInterval)
	defer trigger.Stop()

	state, err := database.FetchState(ctx, db, nil)
	if err != nil {
		// A cancelled ctx here means shutdown raced startup, not a real fault.
		if ctx.Err() != nil {
			logger.Infof("closing collector Run: %v", ctx.Err())
			return
		}
		logger.Panicf("fetching initial state: %v", err)
	}

	params := database.LogsParams{
		Address: flareTeeManager,
		Topic0:  TeeInstructionsSentSel,
		From:    windowStart(state.Index, startInterval),
		To:      int64(state.Index),
	}

	logger.Infof("collector watching FlareTeeManager %s from block %d to %d", flareTeeManager, params.From, params.To)

	stateDamper := newDamper("fetching state")
	logsDamper := newDamper("fetching logs")

	for {
		select {
		case <-trigger.C:
		case <-ctx.Done():
			logger.Infof("closing collector Run: %v", ctx.Err())
			return
		}

		state, err = database.FetchState(ctx, db, nil)
		if err != nil {
			stateDamper.fail(err)
			continue
		}
		stateDamper.ok()

		params.To = int64(state.Index)

		from := params.From
		logs, err := database.FetchLogsByAddressAndTopic0BlockNumber(
			ctx, db, params,
		)
		if err != nil {
			logsDamper.fail(err)
			continue
		}
		logsDamper.ok()

		params.From = params.To

		if len(logs) > 0 {
			logger.Debugf("collected %d instruction logs in blocks (%d,%d]", len(logs), from, params.To)

			select {
			case out <- logs:
			case <-ctx.Done():
				logger.Infof("closing collector Run: %v", ctx.Err())
				return
			}
		}
	}
}
