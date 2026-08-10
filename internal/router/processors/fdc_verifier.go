package processors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/fdc2"
	"github.com/flare-foundation/tee-relay-client/internal/post"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
)

// Status is the verifier's verdict on an attestation request.
type Status string

// Verifier response statuses. Any other value is a protocol violation.
const (
	StatusVerified Status = "VERIFIED"
	StatusRetry    Status = "RETRY"
	StatusRejected Status = "REJECTED"
)

// ErrUnknownStatus marks a verifier response whose status is none of the defined values.
var ErrUnknownStatus = errors.New("unknown verifier status")

// ErrEmptyResponseBody marks a VERIFIED verifier response with an empty response body.
var ErrEmptyResponseBody = errors.New("VERIFIED verifier response with empty response body")

// ErrResponseBodyTooBig marks a VERIFIED verifier response whose body exceeds maxResponseBodyLen.
var ErrResponseBodyTooBig = errors.New("verifier response body exceeds the instruction size limit")

// VerifierRequest is a request sent to the verifier server.
type VerifierRequest struct {
	AttestationType common.Hash   `json:"attestationType"`
	SourceID        common.Hash   `json:"sourceId"`
	RequestBody     hexutil.Bytes `json:"requestBody"`
}

// VerifierResponse is the verifier server's answer to a VerifierRequest.
type VerifierResponse struct {
	Status       Status        `json:"status"`
	ResponseBody hexutil.Bytes `json:"responseBody,omitempty"` // nonempty iff Status is VERIFIED
	Message      string        `json:"message,omitempty"`      // reason when Status is not VERIFIED
}

// UnmarshalJSON decodes a verifier response, treating a JSON null responseBody
// as absent — hexutil.Bytes alone rejects null, a common encoding of "no body".
func (r *VerifierResponse) UnmarshalJSON(b []byte) error {
	var aux struct {
		Status       Status          `json:"status"`
		ResponseBody json.RawMessage `json:"responseBody"`
		Message      string          `json:"message"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}

	r.Status, r.Message, r.ResponseBody = aux.Status, aux.Message, nil
	if len(aux.ResponseBody) == 0 || bytes.Equal(aux.ResponseBody, []byte("null")) {
		return nil
	}

	return json.Unmarshal(aux.ResponseBody, &r.ResponseBody)
}

// Verifier holds credentials for the verifier server and implements the Responder interface.
type Verifier struct {
	*config.Credentials

	params post.Params
}

var _ Responder = &Verifier{}

// verifierTransport caps response headers next to the body cap in NewVerifier.
// A clone: run_test.go pins http.DefaultTransport by identity, never mutate it.
var verifierTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // stdlib guarantees the concrete type
	t.MaxResponseHeaderBytes = 64 << 10                  // stdlib default is 10 MiB

	return t
}()

// NewVerifier returns a Verifier for the server described by c with default call and retry parameters.
func NewVerifier(c *config.Credentials) *Verifier {
	return &Verifier{
		Credentials: c,
		params: post.Params{
			Call: call.Params{
				Timeout:         10 * time.Second,
				MaxResponseSize: 1 << 20, // 1 MiB
				Transport:       verifierTransport,
			},
			// Timeout covers the worst-case schedule (3 x 10s calls + jittered
			// delays up to 6.5s + 13s ≈ 49.5s) with slack, so a final attempt
			// finishing near the deadline is not discarded.
			Retry: retry.Params{
				MaxAttempts: 3,
				Delay:       5 * time.Second,
				Multiplier:  2,   // back off a struggling verifier
				Jitter:      0.3, // desynchronize concurrent workers retrying it
				Timeout:     60 * time.Second,
			},
		},
	}
}

// Response sends the attestation request to the verifier server and returns its
// validated response. Transient HTTP failures are retried; see post.WithRetry.
func (v *Verifier) Response(ctx context.Context, request fdc2.IFdc2HubFdc2AttestationRequest) (VerifierResponse, error) {
	verifierRequest := VerifierRequest{
		AttestationType: request.Header.AttestationType,
		SourceID:        request.Header.SourceId,
		RequestBody:     request.RequestBody,
	}

	res, err := post.WithRetry[VerifierRequest, VerifierResponse](ctx, v.URL, v.APIKey(), verifierRequest, v.params)
	if err != nil {
		return VerifierResponse{}, err
	}

	return validate(*res)
}

// maxMessageLen and maxStatusLen cap untrusted response fields quoted into errors and logs.
const (
	maxMessageLen = 1024
	maxStatusLen  = 64
)

// maxResponseBodyLen mirrors tee-node's op.Prove additionalFixedMessage constraint
// (pkg/constraints); a bigger body would be signed only to be rejected by every TEE.
const maxResponseBodyLen = 100 * 1024

// validate enforces the wire contract on a decoded response.
// ResponseBody of a non-VERIFIED response and Message of a VERIFIED one are ignored, not rejected.
func validate(res VerifierResponse) (VerifierResponse, error) {
	res.Message = truncate(res.Message, maxMessageLen)

	switch res.Status {
	case StatusVerified:
		if len(res.ResponseBody) == 0 {
			return VerifierResponse{}, ErrEmptyResponseBody
		}
		if len(res.ResponseBody) > maxResponseBodyLen {
			return VerifierResponse{}, fmt.Errorf("%w: %d bytes", ErrResponseBodyTooBig, len(res.ResponseBody))
		}
	case StatusRetry, StatusRejected:
	default:
		return VerifierResponse{}, fmt.Errorf("%w: %q", ErrUnknownStatus, truncate(string(res.Status), maxStatusLen))
	}

	return res, nil
}

// truncate returns s unchanged, or its first n bytes plus "..."; may split a UTF-8 rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
