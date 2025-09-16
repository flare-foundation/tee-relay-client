package instructions

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/priority"
	"github.com/flare-foundation/go-flare-common/pkg/tee/op"
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
	fullRequest, err := structs.Decode[connector.IFtdcHubFtdcAttestationRequest](connector.MessageArguments[op.Prove], ib.GeneralData.OriginalMessage)
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
			logger.Debugf("request %s failed %v, not retrying", ib.GeneralData.InstructionID.String(), err)
			return nil
		}
	}

	ib.GeneralData.AdditionalFixedMessage = attResponse
	hashToBeSigned, err := hashFTDCMessage(fullRequest, attResponse, ib.Event.Cosigners, ib.Event.CosignersThreshold, ib.GeneralData.Timestamp)
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
func hashFTDCMessage(req connector.IFtdcHubFtdcAttestationRequest, responseBody []byte, cosigners []common.Address, cosignersThreshold uint64, timestamp uint64) (common.Hash, error) {
	header := connector.IFtdcHubFtdcResponseHeader{
		AttestationType:    req.Header.AttestationType,
		SourceId:           req.Header.SourceId,
		ThresholdBIPS:      req.Header.ThresholdBIPS,
		Cosigners:          cosigners,
		CosignersThreshold: cosignersThreshold,
		Timestamp:          timestamp,
	}

	encHeader, err := EncodeFTDCResponse(header)
	if err != nil {
		return common.Hash{}, err
	}

	headerHash := crypto.Keccak256Hash(encHeader)
	reqBodyHash := crypto.Keccak256Hash(req.RequestBody)
	resBodyHash := crypto.Keccak256Hash(responseBody)

	msgHash := crypto.Keccak256Hash(headerHash[:], reqBodyHash[:], resBodyHash[:])

	tempBuffer := bytes.NewBuffer(nil)

	tempBuffer.WriteByte(1)           // 1 byte (protocolId=1)
	tempBuffer.Write(make([]byte, 5)) // 4 bytes (votingRoundId=0), 1 byte (isSecureRandom=false)
	tempBuffer.Write(msgHash[:])      // Type (1 byte)

	hashToBeSigned := crypto.Keccak256Hash(tempBuffer.Bytes())

	return hashToBeSigned, nil
}

func EncodeFTDCResponse(header connector.IFtdcHubFtdcResponseHeader) (hexutil.Bytes, error) {
	return structs.Encode(connector.ResponseHeaderArg, &header)
}
