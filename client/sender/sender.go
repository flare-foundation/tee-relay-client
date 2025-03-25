package sender

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/client/instructions"

	"github.com/ethereum/go-ethereum/rpc"
)

// const sendSignedInstructions = "/send-signed-instruction"

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

func SendToTee(ctx context.Context, url string, instr instruction.Instruction) error {
	// urlEndpoint := url + sendSignedInstructions

	// body, err := json.Marshal(instr)

	// if err != nil {
	// 	return err
	// }

	client, err := rpc.Dial(url)
	if err != nil {
		return err
	}

	// // todo handle response
	// _, err = utils.PostWithRetry[any](ctx, urlEndpoint, utils.NoAPIKey, body, utils.RetryParams{
	// 	MaxAttempts: 3,
	// 	Delay:       10 * time.Second,
	// 	Timeout:     time.Minute,
	// })
	var res any

	err = client.CallContext(ctx, &res, "instructionservice_sendSignedInstruction", instr)

	logger.Infof("sent instruction %s to %s, res: v, err: %v", instr.Data.InstructionID, url, res, err)

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
