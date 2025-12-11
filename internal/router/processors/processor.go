package processors

import (
	"context"
	"fmt"

	"github.com/flare-foundation/tee-relay-client/internal/router/instructions"
	"github.com/flare-foundation/tee-relay-client/pkg/signer"
)

// Processor defines the interface for processing instructions.
// Implementations should handle the logic for processing of a Base instruction.
type Processor interface {
	Process(context.Context, *instructions.Base) error
}

// Base provides common logic for instruction processing, including signing and out channel for processed instructions.
type Base struct {
	// signer is used for signing instructions.
	signer signer.Signer
	// out is the channel to send processed instructions.
	out chan<- *instructions.Base
}

func NewBase(signer signer.Signer) (b *Base) {
	return &Base{
		signer: signer,
	}
}

func (b *Base) SetOut(out chan<- *instructions.Base) {
	b.out = out
}

// Process signs the instruction and sends it to the output channel.
func (b *Base) Process(ctx context.Context, ib *instructions.Base) error {
	if b.out == nil {
		return fmt.Errorf("out chanel not set")
	}

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
