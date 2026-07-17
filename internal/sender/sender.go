// Package sender sends instructions to the TEE machines.
package sender

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/flare-foundation/go-flare-common/pkg/call"
	"github.com/flare-foundation/go-flare-common/pkg/convert"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/retry"
	"github.com/flare-foundation/go-flare-common/pkg/safeurl"
	"github.com/flare-foundation/go-flare-common/pkg/tee/instruction"
	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
)

const timeout = 5 * time.Second // maximal duration for the server to resolve the query
const maxRespSize = 10 << 10    // 10 KiB for maximal response size of the server

// Run starts a go routine that listens to instructions from in channel and sends them to tees.
// When allowUnsafeURLs is true SSRF protection is disabled — only for local testing.
func Run(ctx context.Context, wg *sync.WaitGroup, in <-chan *instructions.Base, allowUnsafeURLs bool) {
	transport := newTransport(allowUnsafeURLs)

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				logger.Infof("closing sender Run: %v", ctx.Err())
				return
			case instr, ok := <-in:
				if !ok {
					logger.Infof("closing sender Run: in channel closed")
					return
				}

				for j := range instr.Tees {
					go func() {
						msg, url, err := PrepareInstruction(*instr, j)
						if err != nil {
							logger.Errorf("preparing instruction %s for tee %s: %v", instructionOPLogging(instr.GeneralData), instr.Tees[j].TeeId, err)
							return
						}

						err = SendToTEE(ctx, url, *msg, transport)
						if err != nil {
							// quote: err can carry the endpoint's raw response body.
							logger.Errorf("sending instruction %s for %s to %s: %s", instructionOPLogging(msg.Data), msg.Data.TeeID, strconv.Quote(url), strconv.Quote(err.Error()))
							return
						}
					}()
				}
			}
		}
	})
}

func newTransport(allowUnsafe bool) http.RoundTripper {
	if allowUnsafe {
		return http.DefaultTransport
	}
	return safeurl.NewTransport()
}

// SignedReceipt combines Receipt and its signature. TEMP
type SignedReceipt struct {
	Receipt   Receipt       `json:"receipt"`
	Signature hexutil.Bytes `json:"signature"`
}

// Receipt is the result returned by a TEE for a delivered instruction.
type Receipt struct {
	InstructionHash               common.Hash   `json:"instructionHash"`
	Sequence                      uint64        `json:"sequence"`
	Signature                     hexutil.Bytes `json:"signature"`
	AdditionalVariableMessageHash common.Hash   `json:"additionalVariableMessageHash"`
	Timestamp                     uint64        `json:"timestamp"`
	VoteHash                      common.Hash   `json:"voteHash"`
}

// SendToTEE sends the instruction to the instruction endpoint of the TEE at url.
func SendToTEE(ctx context.Context, url string, instr instruction.Instruction, transport http.RoundTripper) error {
	urlEndpoint := url + "/instruction"

	start := time.Now()

	// todo handle response
	res, err := call.PostWithRetry[instruction.Instruction, SignedReceipt](ctx, urlEndpoint, call.NoAPIKey, instr, call.Params{
		Timeout:         timeout,
		MaxResponseSize: maxRespSize,
		Transport:       transport,
	}, []int{},
		retry.Params{
			MaxAttempts: 3,
			Delay:       10 * time.Second,
			Timeout:     time.Minute,
		})

	if err == nil {
		logger.Debugf("delivered instruction %s to %s in %s, receipt: seq %d, ts %d, vote %s",
			instructionOPLogging(instr.Data), strconv.Quote(url), time.Since(start),
			res.Message.Receipt.Sequence, res.Message.Receipt.Timestamp, res.Message.Receipt.VoteHash)
	}

	return err
}

// PrepareInstruction prepares instruction for j-th tee machine and returns its url.
func PrepareInstruction(ib instructions.Base, j int) (*instruction.Instruction, string, error) {
	if j < 0 || j >= len(ib.Tees) {
		return nil, "", fmt.Errorf("invalid tee index %d. Should be in [0,%d)", j, len(ib.Tees))
	}

	data := ib.GeneralData
	data.TeeID = ib.Tees[j].TeeId

	url := ib.Tees[j].Url

	instr := instruction.Instruction{
		Data:      data,
		Signature: ib.Signatures[j],
	}

	return &instr, url, nil
}

func instructionOPLogging(data instruction.Data) string {
	return fmt.Sprintf("id: %s opType: %s, opCommand: %s", data.InstructionID.String(), convert.CommonHashToString(data.OPType), convert.CommonHashToString(data.OPCommand))
}
