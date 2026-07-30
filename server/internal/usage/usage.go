// Package usage turns device reports into hourly usage buckets and turns
// stored buckets back into the wire response for GET /api/v1/usage.
//
// It is pure logic with no I/O: it does not import store, so the dependency
// direction stays one-way (api -> usage, api -> store; store -/-> usage) and
// the attribution rules can be unit-tested without a database. The api layer
// does the trivial conversion between these types and store's row structs.
package usage

import (
	"time"

	"cyberstalk.me/shared"
)

// The three states a second of wall-clock time can be attributed to.
const (
	StateActive = "active"
	StateIdle   = "idle"
	StateLocked = "locked"
)

// Bucket is one hour's increment for a single (state, app, description).
type Bucket struct {
	HourStart   time.Time // UTC, truncated to the hour
	State       shared.UsageState
	App         string
	Description string
	Seconds     int
}

// stateOf decides which state an observed activity belongs to. Priority is
// locked > idle > active.
func stateOf(a shared.Activity) shared.UsageState {
	switch {
	case a.Locked:
		// Checked first because Idle cannot be trusted here: Windows reports
		// idle_seconds 0 while the screen is locked (measured 2026-07-30), so
		// a locked report usually arrives with Idle == false. Locking also
		// cannot be "active use" of anything, whatever the input timer says.
		return StateLocked
	case a.Idle:
		return StateIdle
	default:
		return StateActive
	}
}

// Attribute splits the interval (from, to] across UTC hour boundaries and
// attributes every second to the activity that was observed at from — the
// previous report, not the one that just arrived. Attributing to the new
// report would credit a freshly-switched app with the whole interval.
//
// Both bounds are truncated to whole seconds first, so the sub-second
// remainder is dropped once for the interval rather than once per hour
// boundary; the error is bounded by one second per report.
//
// It returns nil when:
//   - to is not after from (clock skew / duplicate report), or
//   - to.Sub(from) > maxGap, meaning the device was silent long enough that we
//     do not know what it was doing. A powered-off night must not become eight
//     hours of usage.
func Attribute(observed shared.Activity, from, to time.Time, maxGap time.Duration) []Bucket {
	from = from.UTC().Truncate(time.Second)
	to = to.UTC().Truncate(time.Second)
	if !to.After(from) {
		return nil
	}
	if to.Sub(from) > maxGap {
		return nil
	}

	state := stateOf(observed)
	var out []Bucket
	for cur := from; cur.Before(to); {
		hourStart := cur.Truncate(time.Hour)
		next := hourStart.Add(time.Hour)
		if next.After(to) {
			next = to
		}
		seconds := int(next.Sub(cur) / time.Second)
		if seconds > 0 {
			out = append(out, Bucket{
				HourStart:   hourStart,
				State:       state,
				App:         observed.App,
				Description: observed.Description,
				Seconds:     seconds,
			})
		}
		cur = next
	}
	return out
}
