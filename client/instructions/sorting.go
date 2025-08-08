package instructions

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
	"github.com/flare-foundation/go-flare-common/pkg/tee/constants"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs"
	"github.com/flare-foundation/go-flare-common/pkg/tee/structs/connector"
	"github.com/flare-foundation/tee-relay-client/client/config"
)

// Instruction Class refers to an implementation of Instruction interface.
//
//   - Pl -> Plain
//   - Aug -> Augment
//   - AugNSign -> AugmentAndSign
type InstructionClass int

const (
	InvalidInstructionClass InstructionClass = iota
	Pl
	FTDC
)

// OPToInstClass is a mapping from OPCommand to InstructionClass.
var OPToInstClass map[common.Hash]InstructionClass

var plainCommands = []constants.OPCommand{
	// REG

	constants.TEEAttestation,

	// WALLET

	constants.KeyDataProviderRestore,
	constants.KeyDataProviderRestoreTest,
	constants.KeyGenerate,
	constants.KeyDelete,

	// XRP,BTC
	constants.Pay,
	constants.Reissue,
}

var ftdcCommands = []constants.OPCommand{
	// FTDC
	constants.Prove,
}

func init() {
	OPToInstClass = make(map[common.Hash]InstructionClass)

	for j := range plainCommands {
		hexCommand := plainCommands[j].Hash()
		OPToInstClass[hexCommand] = Pl
	}

	for j := range ftdcCommands {
		hexCommand := ftdcCommands[j].Hash()
		OPToInstClass[hexCommand] = FTDC
	}
}

// Router holds processors for instructions.
type Router struct {
	baseProcessor  *BaseProcessor
	ftdcProcessors map[[64]byte]*FTDCProcessor
	ftdcHandler    *FTDCHandler
}

// NewRouter assembles Router from configs.
func NewRouter(sigCfg *config.Credentials, ftdcCfg *config.FTDC) *Router {
	r := new(Router)

	r.baseProcessor = &BaseProcessor{
		signer: &Signer{sigCfg},
	}

	r.ftdcProcessors = make(map[[64]byte]*FTDCProcessor)

	if ftdcCfg != nil {
		queues := make(map[string]*FTDCProcessor)

		for name := range ftdcCfg.Queues {
			queue := NewQueue(ftdcCfg.Queues[name], name)
			queues[name] = &FTDCProcessor{&queue}
		}

		r.ftdcHandler = &FTDCHandler{BaseProcessor: r.baseProcessor, verifiers: make(map[[64]byte]Responder)}

		for _, v := range ftdcCfg.Verifiers {
			identifier, err := v.AttTypeAndSourceID()
			if err != nil {
				logger.Panicf("invalid verifier %v, %v", v, err)
			}
			var exists bool
			r.ftdcProcessors[identifier], exists = queues[v.QueueName]
			if !exists {
				logger.Panicf("undefined queue %s for %s, %s", v.QueueName, v.AttType, v.SourceID)
			}

			r.ftdcHandler.verifiers[identifier] = &Verifier{&v.Server}
		}
	}

	return r
}

// Start initiates router and sets the out channel.
func (r *Router) Start(ctx context.Context, out chan<- *Base) {
	r.baseProcessor.out = out
	for _, q := range r.ftdcProcessors {
		q.q.InitiateAndRun(ctx)
		q.q.ProcessOut(ctx, r.ftdcHandler)
	}
}

// Route returns the processor for the instruction base.
func (r *Router) Route(b *Base) (Processor, error) {
	ic, ok := OPToInstClass[b.Event.OpCommand]
	if !ok {
		return nil, fmt.Errorf("unsorted opCommand %v", string(b.Event.OpCommand[:]))
	}

	switch ic {
	case Pl:
		return r.baseProcessor, nil
	case FTDC: // currently only opCommand
		fullRequest, err := structs.Decode[connector.IFtdcHubFtdcAttestationRequest](connector.MessageArguments[constants.Prove], b.GeneralData.OriginalMessage)
		if err != nil {
			return nil, fmt.Errorf("decoding ftdc request: %v", err)
		}

		ats, err := attTypeAndSourceID(&fullRequest.Header)
		if err != nil {
			return nil, fmt.Errorf("reading att type and source: %v", err)
		}

		processor, exits := r.ftdcProcessors[ats]
		if !exits {
			return nil, fmt.Errorf("no processor for: %v", ats)
		}

		return processor, nil
	case InvalidInstructionClass: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	default: // should never happen
		return nil, fmt.Errorf("unexpected instructions.InstructionClass: %#v", ic)
	}
}

// attTypeAndSourceID returns concatenated attestation type and source ID each 32 bytes
// for and encoded attestationRequest.
func attTypeAndSourceID(header *connector.IFtdcHubFtdcRequestHeader) ([64]byte, error) {
	res := [64]byte{}

	copy(res[:32], header.AttestationType[:])
	copy(res[32:], header.SourceId[:])

	return res, nil
}
