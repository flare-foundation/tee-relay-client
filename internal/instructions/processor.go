package instructions

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
	"github.com/flare-foundation/tee-relay-client/internal/config"
)

type Processor interface {
	Process(context.Context, *Base) error
}

type BaseProcessor struct {
	signer *Signer
	out    chan<- *Base
}

func (b *BaseProcessor) Process(ctx context.Context, ib *Base) error {
	err := ib.Sign(ctx, b.signer)
	if err != nil {
		return fmt.Errorf("signing: %v", err)
	}

	select {
	case b.out <- ib:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type FTDCProcessor struct {
	q *FTDCQueue
}

// Responder has method Response that gets attestation response for an attestation request.
type Responder interface {
	Response(context.Context, connector.IFtdcHubFtdcAttestationRequest) ([]byte, bool, error) // TODO: decide whether bytes are orig data or just att request
}

// Process adds instruction to the queue.
func (f *FTDCProcessor) Process(ctx context.Context, ib *Base) error {
	f.q.Add(ib, Weight{time.Now()})

	return nil
}

// Verifier holds credentials for verifier server.
//
// Implements Responder interface.
type Verifier struct {
	*config.Credentials
}

type VerifierResponse struct {
	ResponseBody hexutil.Bytes
}

type VerifierRequest struct {
	AttestationType common.Hash
	SourceID        common.Hash
	RequestBody     hexutil.Bytes
}

// Response sends request to the verifier server.
func (v *Verifier) Response(ctx context.Context, request connector.IFtdcHubFtdcAttestationRequest) ([]byte, bool, error) {
	verifierRequest := VerifierRequest{
		AttestationType: request.Header.AttestationType,
		SourceID:        request.Header.SourceId,
		RequestBody:     request.RequestBody,
	}
	res, err := call.PostWithRetry[VerifierRequest, VerifierResponse](ctx, v.URL, v.APIKey(), verifierRequest, call.Params{
		Timeout:         10 * time.Second,
		MaxResponseSize: 1000000000, // todo: set a reasonable value
	},
		[]int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       2 * time.Second,
			Timeout:     10 * time.Second,
		})

	if err != nil {
		return nil, false, err
	}
	if res.Status != http.StatusOK {
		return nil, false, nil
	}

	return res.Message.ResponseBody, true, nil
}
