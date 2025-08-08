package sender

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/tee"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
)

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const maxRespSize = 1 << 20     // 1 MB for maximal response size of the server  TODO: make this more restrictive

// Run starts a go routine that listens to instructions from in channel and sends them to tees.
func Run(ctx context.Context, in <-chan *instructions.Base) {
	go func() {
		for {
			if err := ctx.Err(); err != nil {
				logger.Infof("closing sender Run: %v", err)
				return
			}

			instr, ok := <-in

			if !ok {
				logger.Infof("closing sender Run: in channel closed")
				return
			}

			for j := range instr.Event.TeeMachines {
				go func() {
					msg, url, err := PrepareInstruction(*instr, j)
					if err != nil {
						logger.Errorf("preparing instruction %s for %d: %v", instr.Event.InstructionId, j, err)
						return
					}

					err = SendToTEE(ctx, url, *msg)
					if err != nil {
						logger.Errorf("sending instruction %s for %s to %s: %v", msg.Data.InstructionID, msg.Data.TeeID, url, err)
						return
					}
				}()
			}
		}
	}()
}

// todo get this from the node repo (or somewhere else).
type SignedReceipt struct {
	Receipt   tee.TeeStructsVoteReceipt `json:"receipt"`
	Signature hexutil.Bytes             `json:"signature"`
}

// SendToTEE sends the instruction instruction endpoint of tee at url.
func SendToTEE(ctx context.Context, url string, instr instruction.Instruction) error {
	urlEndpoint := url + "/instruction"

	// todo handle response
	res, err := call.PostWithRetry[instruction.Instruction, SignedReceipt](ctx, urlEndpoint, call.NoAPIKey, instr, call.Params{
		Timeout:         timeout,
		MaxResponseSize: maxRespSize,
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     time.Minute,
		})

	if err == nil {
		logger.Infof("delivered instruction %s to %s, res: %v", instr.Data.InstructionID.String(), url, res.Message)
	} else {
		logger.Errorf("error sending: %v", err)
	}

	return err
}

// PrepareInstruction prepares instruction for j-th tee machine and returns its url.
func PrepareInstruction(ib instructions.Base, j int) (*instruction.Instruction, string, error) {
	if j < 0 || j >= len(ib.Event.TeeMachines) {
		return nil, "", fmt.Errorf("invalid tee index %d. Should be in [0,%d)", j, len(ib.Event.TeeMachines))
	}

	data := ib.GeneralData
	data.TeeID = ib.Event.TeeMachines[j].TeeId

	url := ib.Event.TeeMachines[j].Url

	challenge := common.Hash{}
	_, err := rand.Read(challenge[:])
	if err != nil {
		return nil, "", err
	}

	instr := instruction.Instruction{
		Data:      data,
		Signature: ib.Signatures[j],
	}

	return &instr, url, nil
}
