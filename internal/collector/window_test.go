package collector

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		index         uint64
		startInterval uint64
		want          int64
	}{
		{"index below interval clamps to 0", 12, 100, 0},
		{"index equals interval clamps to 0", 100, 100, 0},
		{"index just above interval", 101, 100, 1},
		{"index above interval", 150, 100, 50},
		{"zero index", 0, 100, 0},
		{"large index does not underflow", 1_000_000, 100, 999_900},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, windowStart(tt.index, tt.startInterval))
		})
	}
}
