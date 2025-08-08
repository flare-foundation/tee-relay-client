package instructions

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/tee/constants"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
)

// Weight for ordering of the FTDC queues.
//
// An item has higher priority if it has arrived earlier.
type Weight struct{ time.Time }

func (w Weight) Self() Weight {
	return w
}

// Less returns true if t is before w.
func (w Weight) Less(t Weight) bool {
	return t.Before(w.Time)
}

type FTDCQueue struct {
	*priority.PriorityQueue[*Base, Weight]
}

// NewQueue creates a FTDC queue.
func NewQueue(params priority.Params, name string) FTDCQueue {
	queue := priority.New[*Base, Weight](params, name)

	return FTDCQueue{&queue}
}

// FTDCHandler links to verifiers for FTDC instructions.
type FTDCHandler struct {
	*BaseProcessor
	verifiers map[[64]byte]Responder
}

// Handle handles instruction base for opType FTDC opCommand PROVE.
func (h *FTDCHandler) Handle(ctx context.Context, ib *Base) error {
	fullRequest, err := structs.Decode[connector.IFtdcHubFtdcAttestationRequest](connector.MessageArguments[constants.Prove], ib.GeneralData.OriginalMessage)
	if err != nil {
		return fmt.Errorf("decoding request: %v", err) // should never happen
	}

	ats, err := attTypeAndSourceID(&fullRequest.Header)
	if err != nil {
		return fmt.Errorf("reading reading att type and source ID: %v", err) // should never happen
	}

	v, exists := h.verifiers[ats]
	if !exists {
		return fmt.Errorf("no verifier for %v", ats)
	}

	attResponse, retry, err := v.Response(ctx, fullRequest)
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
	hashToBeSigned, _, err := hashFTDCMessage(fullRequest, attResponse, ib.GeneralData.Timestamp)
	if err != nil {
		return fmt.Errorf("hashing ftdc message: %w", err)
	}

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

type Handler interface {
	Handle(context.Context, *Base) error
}

// ProcessOut spawns a go routine that dequeues and handles dequeues items.
// TODO move this to common.
func (q *FTDCQueue) ProcessOut(ctx context.Context, h Handler) {
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

// hashFTDCMessage is here temporarily.
func hashFTDCMessage(req connector.IFtdcHubFtdcAttestationRequest, responseBody []byte, timestamp uint64) (common.Hash, hexutil.Bytes, error) {
	header := connector.IFtdcHubFtdcResponseHeader{
		AttestationType:    req.Header.AttestationType,
		SourceId:           req.Header.SourceId,
		ThresholdBIPS:      req.Header.ThresholdBIPS,
		Cosigners:          req.Header.Cosigners,
		CosignersThreshold: req.Header.CosignersThreshold,
		Timestamp:          timestamp,
	}

	encHeader, err := EncodeFTDCResponse(header)
	if err != nil {
		return common.Hash{}, nil, err
	}

	headerHash := crypto.Keccak256Hash(encHeader)
	reqBodyHash := crypto.Keccak256Hash(req.RequestBody)
	resBodyHash := crypto.Keccak256Hash(responseBody)

	msgHash := crypto.Keccak256Hash(headerHash[:], reqBodyHash[:], resBodyHash[:])

	return msgHash, encHeader, nil
}

func EncodeFTDCResponse(header connector.IFtdcHubFtdcResponseHeader) (hexutil.Bytes, error) {
	return structs.Encode(connector.ResponseHeaderArg, &header)
}
