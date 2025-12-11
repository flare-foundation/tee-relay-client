package processors

import (
	"context"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// Verifier holds credentials for the verifier server and implements the Responder interface.
type Verifier struct {
	*config.Credentials
}

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
