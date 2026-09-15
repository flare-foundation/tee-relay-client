package collector

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type logRecord struct {
	level string
	msg   string
}

type recorder struct {
	logs []logRecord
}

func (r *recorder) at(level string) func(string, ...any) {
	return func(msg string, args ...any) {
		r.logs = append(r.logs, logRecord{level: level, msg: fmt.Sprintf(msg, args...)})
	}
}

func (r *recorder) count(level string) int {
	n := 0
	for _, l := range r.logs {
		if l.level == level {
			n++
		}
	}
	return n
}

func (r *recorder) last(level string) string {
	for _, l := range slices.Backward(r.logs) {
		if l.level == level {
			return l.msg
		}
	}
	return ""
}

// newTestDamper returns a damper wired to a recorder and a fake clock. Advance
// time by assigning through the returned pointer.
func newTestDamper(t *testing.T) (*damper, *recorder, *time.Time) {
	t.Helper()
	rec := &recorder{}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := &damper{
		op:     "op",
		now:    func() time.Time { return clock },
		errorf: rec.at("error"),
		debugf: rec.at("debug"),
		infof:  rec.at("info"),
	}
	return d, rec, &clock
}

func TestDamperFirstFailIsError(t *testing.T) {
	t.Parallel()
	d, rec, _ := newTestDamper(t)

	d.fail(errors.New("boom"))

	require.Equal(t, 1, rec.count("error"))
	require.Equal(t, 0, rec.count("debug"))
	require.Contains(t, rec.last("error"), "op: boom")
}

func TestDamperRepeatsWithinIntervalAreDebug(t *testing.T) {
	t.Parallel()
	d, rec, _ := newTestDamper(t)

	d.fail(errors.New("boom"))
	// Clock unchanged: still inside summaryInterval.
	d.fail(errors.New("boom"))
	d.fail(errors.New("boom"))

	require.Equal(t, 1, rec.count("error"))
	require.Equal(t, 2, rec.count("debug"))
}

func TestDamperSummaryAfterInterval(t *testing.T) {
	t.Parallel()
	d, rec, clock := newTestDamper(t)

	d.fail(errors.New("boom"))
	d.fail(errors.New("boom")) // debug
	*clock = clock.Add(summaryInterval)
	d.fail(errors.New("boom")) // periodic summary at error

	require.Equal(t, 2, rec.count("error"))
	require.Equal(t, 1, rec.count("debug"))
	summary := rec.last("error")
	require.Contains(t, summary, "still failing")
	require.Contains(t, summary, "3 failures")
}

func TestDamperOkAfterFailuresLogsRecovery(t *testing.T) {
	t.Parallel()
	d, rec, clock := newTestDamper(t)

	d.fail(errors.New("boom"))
	d.fail(errors.New("boom"))
	*clock = clock.Add(90 * time.Second)
	d.ok()

	require.Equal(t, 1, rec.count("info"))
	line := rec.last("info")
	require.Contains(t, line, "recovered after 2 failures")
	require.Contains(t, line, "1m30s")
	require.Equal(t, 0, d.failures)
}

func TestDamperOkWithNoFailuresIsSilent(t *testing.T) {
	t.Parallel()
	d, rec, _ := newTestDamper(t)

	d.ok()

	require.Empty(t, rec.logs)
}

func TestDamperCycleRestartsAfterRecovery(t *testing.T) {
	t.Parallel()
	d, rec, _ := newTestDamper(t)

	d.fail(errors.New("boom")) // error
	d.ok()                     // recovered
	d.fail(errors.New("boom")) // error again, fresh cycle

	require.Equal(t, 2, rec.count("error"))
	require.Equal(t, 0, rec.count("debug"))
	require.Equal(t, 1, rec.count("info"))
}

func TestDamperLevelSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		steps []func(d *damper, clock *time.Time)
		want  []string
	}{
		{
			name: "fail then recover then fail",
			steps: []func(d *damper, clock *time.Time){
				func(d *damper, _ *time.Time) { d.fail(errors.New("x")) },
				func(d *damper, _ *time.Time) { d.fail(errors.New("x")) },
				func(d *damper, _ *time.Time) { d.ok() },
				func(d *damper, _ *time.Time) { d.fail(errors.New("x")) },
			},
			want: []string{"error", "debug", "info", "error"},
		},
		{
			name: "summary then more debug",
			steps: []func(d *damper, clock *time.Time){
				func(d *damper, _ *time.Time) { d.fail(errors.New("x")) },
				func(d *damper, clock *time.Time) { *clock = clock.Add(summaryInterval); d.fail(errors.New("x")) },
				func(d *damper, _ *time.Time) { d.fail(errors.New("x")) },
			},
			want: []string{"error", "error", "debug"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, rec, clock := newTestDamper(t)
			for _, step := range tt.steps {
				step(d, clock)
			}
			levels := make([]string, len(rec.logs))
			for i, l := range rec.logs {
				levels[i] = l.level
			}
			require.Equal(t, tt.want, levels, strings.Join(levels, ","))
		})
	}
}
