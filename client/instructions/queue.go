package instructions

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
)

type Weight struct{ time.Time }

func (w Weight) Self() Weight {
	return w
}

func (w Weight) Less(t Weight) bool {
	return t.Before(w.Time)
}

type FTDCQueue struct {
	*priority.PriorityQueue[*Base, Weight]
}

func NewQueue(params priority.Params, name string) FTDCQueue {
	queue := priority.New[*Base, Weight](params, name)

	return FTDCQueue{&queue}
}

type FTDCHandler struct {
	BaseProcessor
	verifiers map[[64]byte]Responder
}

func (h *FTDCHandler) Handle(ctx context.Context, ib *Base) error {
	fullRequest, err := structs.Decode[connector.IFtdcHubFtdcProve](connector.MessageArguments[connector.Prove], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding request: %v", err) // should never happen
	}

	ats, err := attTypeAndSourceID(fullRequest.AttestationRequest)
	if err != nil {
		return fmt.Errorf("reading reading att type and source ID: %v", err) // should never happen
	}

	v, exists := h.verifiers[ats]
	if !exists {
		return fmt.Errorf("no verifier for %v", ats)
	}

	attResponse, retry, err := v.Response(ctx, fullRequest.AttestationRequest)
	if err != nil {
		switch retry {
		case true:
			return fmt.Errorf("getting response: %v", err)
		case false:
			logger.Debugf("request from %s event failed %v, not retrying", ib.Event.InstructionId, err)
			return nil
		}
	}

	ib.GeneralData.AdditionalFixedMessage = attResponse
	hashToBeSigned := crypto.Keccak256Hash(attResponse)

	signature, err := h.signer.FetchSignatures(ctx, []common.Hash{hashToBeSigned})
	if err != nil {
		return fmt.Errorf("signing response: %v", err)
	}

	ib.GeneralData.AdditionalVariableMessage = signature[0] // if err != nil, len(signature)=1

	err = ib.Sign(ctx, h.signer)
	if err != nil {
		return fmt.Errorf("signing instruction: %v", err)
	}

	select {
	case h.out <- ib:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *FTDCQueue) ProcessOut(ctx context.Context, h *FTDCHandler) {
	go func() {
		for {
			if err := ctx.Err(); err != nil {
				logger.Infof("processing out of queue %s stopped: %v", q.Name(), err)
				return
			}

			q.Dequeue(ctx, h.Handle, nil)
		}
	}()
}
