package instructions

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSorting(t *testing.T) {
	require.Equal(t, len(OPToInstClass), len(plainCommands)+len(augmentCommands))
}
