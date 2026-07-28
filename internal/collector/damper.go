package collector

import (
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

// summaryInterval is the minimum gap between periodic still-failing summaries.
const summaryInterval = 5 * time.Minute

// damper rate-limits repeated failure logging for one operation: first
// failure at Error, repeats at Debug, a periodic Error summary, and a
// single Info line on recovery. Not safe for concurrent use.
type damper struct {
	op                    string
	failures              int
	first                 time.Time
	lastErr               time.Time
	now                   func() time.Time
	errorf, debugf, infof func(string, ...any)
}

// newDamper creates a damper for op wired to the global logger and the real clock.
func newDamper(op string) *damper {
	return &damper{
		op:     op,
		now:    time.Now,
		errorf: logger.Errorf,
		debugf: logger.Debugf,
		infof:  logger.Infof,
	}
}

// fail records a failure: Error on the first, a periodic Error summary once past
// summaryInterval, Debug otherwise.
func (d *damper) fail(err error) {
	d.failures++
	if d.failures == 1 {
		d.errorf("%s: %v", d.op, err)
		now := d.now()
		d.first, d.lastErr = now, now
		return
	}
	if d.now().Sub(d.lastErr) >= summaryInterval {
		d.errorf("%s still failing: %d failures since %s: %v", d.op, d.failures, d.first.Format(time.RFC3339), err)
		d.lastErr = d.now()
		return
	}
	d.debugf("%s: %v", d.op, err)
}

// ok logs a single recovery line if failures had accumulated, then resets.
func (d *damper) ok() {
	if d.failures == 0 {
		return
	}
	d.infof("%s recovered after %d failures in %s", d.op, d.failures, d.now().Sub(d.first).Round(time.Second))
	d.failures = 0
}
