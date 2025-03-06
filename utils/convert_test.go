package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToBytes32(t *testing.T) {
	tests := []struct {
		in  string
		err bool
	}{
		{
			in:  "",
			err: false,
		},
		{
			in:  " x ",
			err: false,
		},
		{
			in:  "12 \n",
			err: false,
		},
		{
			in:  strings.Repeat("a", 33),
			err: true,
		},
	}

	for _, test := range tests {
		out, err := ToBytes32(test.in)

		if test.err {
			require.Error(t, err, test.in)
		} else {
			wordEnd := len(test.in)
			word := out[0:wordEnd]
			rest := out[wordEnd:]
			restExpected := make([]byte, 32-wordEnd)

			require.Equal(t, []byte(test.in), word, test.in)
			require.Equal(t, restExpected, rest, test.in)
		}
	}
}
