package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopicHex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		topic string
		want  string
	}{
		{"bare hex", "1122334455667788990011223344556677889900112233445566778899001122", "0x1122334455667788990011223344556677889900112233445566778899001122"},
		{"empty string", "", "0x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, topicHex(tt.topic))
		})
	}
}
