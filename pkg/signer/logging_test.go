package signer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/flare-foundation/tee-relay-client/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// *zap.SugaredLogger is the concrete logger the client injects; it must satisfy Logger.
var _ Logger = &zap.SugaredLogger{}

// recordingLogger captures rendered messages logged through the Logger interface.
type recordingLogger struct {
	msgs []string
}

func (l *recordingLogger) record(f string, a ...any) { l.msgs = append(l.msgs, fmt.Sprintf(f, a...)) }
func (l *recordingLogger) Debugf(f string, a ...any) { l.record(f, a...) }
func (l *recordingLogger) Infof(f string, a ...any)  { l.record(f, a...) }
func (l *recordingLogger) Warnf(f string, a ...any)  { l.record(f, a...) }
func (l *recordingLogger) Errorf(f string, a ...any) { l.record(f, a...) }

func (l *recordingLogger) contains(sub string) bool {
	for _, m := range l.msgs {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}

// TestRemoteSignLogsDebug checks that a successful Sign records a Debug line with the hash count.
func TestRemoteSignLogsDebug(t *testing.T) {
	t.Parallel()
	srv := jsonServer("/sign", `{"signatures":["0x01"]}`)
	defer srv.Close()

	rec := &recordingLogger{}
	rs := NewRemoteWithLogger(&config.Credentials{URL: srv.URL, KeyName: "X-API-KEY", Key: "k"}, rec)

	_, err := rs.Sign(context.Background(), []common.Hash{common.HexToHash("0x1")})
	require.NoError(t, err)
	require.True(t, rec.contains("signed 1 hashes"))
}
