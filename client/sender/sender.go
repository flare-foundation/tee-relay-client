package sender

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
)

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const maxRespSize = 1 << 20     // 1 MB for maximal response size of the server  TODO: make this more restrictive

// Run starts a go routine that listens to instructions from in channel and sends them to Tees.
func Run(ctx context.Context, in <-chan *instructions.InstructionBase) {
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

					err = SendToTee(ctx, url, *msg)
					if err != nil {
						logger.Errorf("sending instruction %s for %s to %s: %v", msg.Data.InstructionID, msg.Data.TeeID, url, err)
						return
					}
				}()
			}
		}
	}()
}

// todo get this from the node repo (or somewhere else)
type TempRes struct {
	Status    string
	Token     string
	Data      []byte
	Finalized bool
}

// SendToTee sends the instruction instruction endpoint of tee at url.
func SendToTee(ctx context.Context, url string, instr instruction.Instruction) error {
	urlEndpoint := url + "/instruction"

	// todo handle response
	res, err := call.PostWithRetry[instruction.Instruction, TempRes](ctx, urlEndpoint, call.NoAPIKey, instr, call.CallParams{
		Timeout:         timeout,
		MaxResponseSize: maxRespSize,
	}, retry.Params{
		MaxAttempts: 3,
		Delay:       10 * time.Second,
		Timeout:     time.Minute,
	})

	if err == nil {
		logger.Infof("delivered instruction %s to %s, res: %v", instr.Data.InstructionID, url, *res)
	} else {
		logger.Errorf("error sending : %v", err)
	}

	return err
}

// PrepareInstruction prepares instruction for j-th tee machine.
func PrepareInstruction(ib instructions.InstructionBase, j int) (*instruction.Instruction, string, error) {
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
		Challenge: challenge,
		Data:      data,
		Signature: ib.Signatures[j],
	}

	return &instr, url, nil
}
