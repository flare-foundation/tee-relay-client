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
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// Processor defines the interface for processing instructions.
// Implementations should handle the logic for processing of a Base instruction.
type Processor interface {
	Process(context.Context, *Base) error
}

// BaseProcessor provides common logic for instruction processing, including signing and out channel for processed instructions.
type BaseProcessor struct {
	// signer is used for signing instructions.
	signer *Signer
	// out is the channel to send processed instructions.
	out chan<- *Base
}

// Process signs the instruction and sends it to the output channel.
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

// FTDCProcessor processes FTDC instructions by adding them to a queue for the designated verifier.
type FTDCProcessor struct {
	q *FTDCQueue
}

// Responder provides attestation responses for attestation requests.
// Response returns the attestation response bytes, a success flag, and an error.
type Responder interface {
	Response(context.Context, connector.IFtdcHubFtdcAttestationRequest) ([]byte, bool, error)
}

// Process adds the instruction to the FTDC queue with the current timestamp as weight.
func (f *FTDCProcessor) Process(ctx context.Context, ib *Base) error {
	f.q.Add(ib, Weight{time.Now()})
	return nil
}

// Verifier holds credentials for the verifier server and implements the Responder interface.
type Verifier struct {
	*config.Credentials
}

// VerifierResponse contains the body of the attestation response.
type VerifierResponse struct {
	// ResponseBody is the body of the attestation response.
	ResponseBody hexutil.Bytes
}

// VerifierRequest is a request sent to the verifier server.
type VerifierRequest struct {
	AttestationType common.Hash
	SourceID        common.Hash
	RequestBody     hexutil.Bytes
}

// Response sends an attestation request to the verifier server and returns the response.
// It performs retries on failure and returns the response bytes, a success flag, and an error.
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
