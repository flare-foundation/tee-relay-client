package sender

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/client/instructions"
	"github.com/flare-foundation/tee-relay-client/utils"
)

const sendSignedInstructions = "/send-signed-instruction"

type Sender struct {
}

func (s Sender) Run(ctx context.Context, in <-chan *instructions.InstructionBase) {
	for {
		if ctx.Err() != nil {
			//TODO
			return
		}

		instr := <-in

		for j := range instr.Event.TeeMachines {
			go func() {
				in, url, err := prepareInstruction(*instr, j)
				if err != nil {
					return //TODO error handling
				}

				err = sendToTee(ctx, url, *in)
				if err != nil {
					return //TODO error handling
				}
			}()
		}
	}
}

func sendToTee(ctx context.Context, url string, instr instruction.Instruction) error {
	urlEndpoint := url + sendSignedInstructions

	body, err := json.Marshal(instr)

	if err != nil {
		return err
	}

	var x any //TODO

	err = utils.POST[any](ctx, urlEndpoint, utils.NoAPIKey, body, &x)
	return err
}

func prepareInstruction(ib instructions.InstructionBase, j int) (*instruction.Instruction, string, error) {
	if j < 0 || j > len(ib.Event.TeeMachines) {
		return nil, "", fmt.Errorf("invalid tee index %d. Should be in  [0,%d)", j, len(ib.Event.TeeMachines))
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
