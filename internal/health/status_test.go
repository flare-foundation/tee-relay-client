package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// requireProbe asserts a probe result: nil for a pass, or an error wrapping want.
func requireProbe(t *testing.T, got, want error) {
	t.Helper()
	if want == nil {
		require.NoError(t, got)
		return
	}
	require.ErrorIs(t, got, want)
}

func TestStatusProbes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	const lag = 30 * time.Second

	tests := []struct {
		name     string
		started  bool
		observed bool
		blockAge time.Duration // now minus the block timestamp; negative stamps the block ahead of the clock
		startup  error
		ready    error
	}{
		{name: "fresh", startup: ErrStartupNotFinished, ready: ErrStartupNotFinished},
		// the collector's first poll can land before Run latches startup
		{name: "observed before started", observed: true, blockAge: time.Second, startup: ErrStartupNotFinished, ready: ErrStartupNotFinished},
		{name: "started, nothing observed", started: true, ready: ErrNoIndexerState},
		{name: "started, fresh block", started: true, observed: true, blockAge: 3 * time.Second},
		{name: "started, block at the limit", started: true, observed: true, blockAge: lag},
		{name: "started, stale block", started: true, observed: true, blockAge: lag + time.Second, ready: ErrIndexerStale},
		{name: "started, block ahead of the clock", started: true, observed: true, blockAge: -2 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			s := NewStatus(lag)
			s.now = func() time.Time { return now }

			if test.started {
				s.SetStarted()
			}
			if test.observed {
				s.SetIndexer(42, now.Add(-test.blockAge))
			}

			requireProbe(t, s.Startup(), test.startup)
			requireProbe(t, s.Ready(), test.ready)
		})
	}
}

// The stale reason carries what an operator needs to tell a stalled indexer from a tight limit.
func TestStatusStaleReason(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	s := NewStatus(30 * time.Second)
	s.now = func() time.Time { return now }
	s.SetStarted()
	s.SetIndexer(12345678, now.Add(-95*time.Second-300*time.Millisecond))

	err := s.Ready()
	require.ErrorIs(t, err, ErrIndexerStale)
	require.EqualError(t, err, "indexer block too old: block 12345678 is 1m35s old, max 30s")
}

// A nil Status is what the pipeline runs with when no health server is configured.
func TestNilStatus(t *testing.T) {
	t.Parallel()

	var s *Status

	s.SetStarted()
	s.SetIndexer(1, time.Now())

	require.ErrorIs(t, s.Startup(), ErrStartupNotFinished)
	require.ErrorIs(t, s.Ready(), ErrStartupNotFinished)
}
