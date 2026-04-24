package processors

import (
	"context"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// Verifier holds credentials for the verifier server and implements the Responder interface.
type Verifier struct {
	*config.Credentials
}

var _ Responder = &Verifier{}

// VerifierResponse contains the body of the attestation response.
type VerifierResponse struct {
	ResponseBody hexutil.Bytes // ResponseBody is the body of the attestation response.
}

// VerifierRequest is a request sent to the verifier server.
type VerifierRequest struct {
	AttestationType common.Hash
	SourceID        common.Hash
	RequestBody     hexutil.Bytes
}

// Response sends an attestation request to the verifier server and returns the response.
// It performs retries on failure and returns the response bytes, a success flag, and an error.
func (v *Verifier) Response(ctx context.Context, request fdc2.IFdc2HubFdc2AttestationRequest) ([]byte, bool, error) {
	verifierRequest := VerifierRequest{
		AttestationType: request.Header.AttestationType,
		SourceID:        request.Header.SourceId,
		RequestBody:     request.RequestBody,
	}
	res, err := call.PostWithRetry[VerifierRequest, VerifierResponse](ctx, v.URL, v.APIKey(), verifierRequest, call.Params{
		Timeout:         10 * time.Second,
		MaxResponseSize: 1 << 20, // 1 MiB
	},
		[]int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusUnprocessableEntity,
		},
		retry.Params{
			MaxAttempts: 3,
			Delay:       5 * time.Second,
			Timeout:     20 * time.Second,
		})

	if err != nil { // unexpected error in the process of verification
		return nil, false, err
	}
	if res.Status != http.StatusOK { // in this case the verifier rejected the request as unconformable
		return nil, false, nil
	}

	return res.Message.ResponseBody, true, nil
}
