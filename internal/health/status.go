// Package health answers the liveness, startup and readiness probes over HTTP.
package health

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Probe failures. Ready wraps ErrIndexerStale with the block and its age.
var (
	ErrStartupNotFinished = errors.New("startup not finished yet")
	ErrNoIndexerState     = errors.New("no indexer state observed yet")
	ErrIndexerStale       = errors.New("indexer block too old")
)

// Status holds the signals the probes are answered from. Its methods are nil-safe: a nil
// Status drops every report and answers every probe as not started, so callers running
// without a health server need no branches.
type Status struct {
	mu        sync.RWMutex
	started   bool
	block     uint64
	blockTime time.Time
	maxLag    time.Duration
	now       func() time.Time
}

// NewStatus creates a Status whose readiness tolerates an indexer block up to maxLag old.
func NewStatus(maxLag time.Duration) *Status {
	return &Status{maxLag: maxLag, now: time.Now}
}

// SetStarted latches the pipeline as started; Startup passes from here on.
func (s *Status) SetStarted() {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
}

// SetIndexer records the indexer's last block and its timestamp.
func (s *Status) SetIndexer(block uint64, blockTime time.Time) {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.block, s.blockTime = block, blockTime
	s.mu.Unlock()
}

// Startup returns ErrStartupNotFinished until SetStarted has been called, nil after.
func (s *Status) Startup() error {
	if s == nil {
		return ErrStartupNotFinished
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.started {
		return ErrStartupNotFinished
	}

	return nil
}

// Ready returns nil when the pipeline has started, an indexer block has been observed,
// and that block is at most maxLag old.
func (s *Status) Ready() error {
	if s == nil {
		return ErrStartupNotFinished
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	switch {
	case !s.started:
		return ErrStartupNotFinished
	case s.blockTime.IsZero():
		return ErrNoIndexerState
	}

	age := max(s.now().Sub(s.blockTime), 0) // a block stamped ahead of this clock is fresh, not negative
	if age > s.maxLag {
		return fmt.Errorf("%w: block %d is %s old, max %s", ErrIndexerStale, s.block, age.Round(time.Second), s.maxLag)
	}

	return nil
}
