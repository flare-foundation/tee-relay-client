package instructions

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/tee-relay-client/client/config"
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
	Response(context.Context, []byte) ([]byte, bool, error) // TODO: decide whether bytes are orig data or just att request
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

// TODO sync with verifiers.
type VerifierRequest struct {
	Request hexutil.Bytes
}
type VerifierResponse struct {
	Response hexutil.Bytes
}

// Response sends request to the verifier server.
func (v *Verifier) Response(ctx context.Context, request []byte) ([]byte, bool, error) {
	r := VerifierRequest{Request: request}
	res, err := call.PostWithRetry[VerifierRequest, VerifierResponse](ctx, v.URL, v.APIKey(), r, call.Params{
		Timeout:         0,
		MaxResponseSize: 0,
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

	return res.Message.Response, true, nil
}
