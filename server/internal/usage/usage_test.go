package usage

import (
	"testing"
	"time"

	"cyberstalk.me/shared"
)

// ts is a terse UTC timestamp constructor for the tables below.
func ts(hour, min, sec int) time.Time {
	return time.Date(2026, 7, 30, hour, min, sec, 0, time.UTC)
}

func TestStateOf(t *testing.T) {
	tests := []struct {
		name string
		act  shared.Activity
		want string
	}{
		{"plain activity is active", shared.Activity{App: "Code"}, StateActive},
		{"idle flag wins over active", shared.Activity{App: "Code", Idle: true}, StateIdle},
		{"locked wins over idle", shared.Activity{App: "Locked", Idle: true, Locked: true}, StateLocked},
		// The realistic lock-screen report: Windows stops advancing the input
		// timer while locked, so Idle is false and only Locked tells us.
		{"locked without idle", shared.Activity{App: "Locked", Locked: true}, StateLocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stateOf(tt.act); got != tt.want {
				t.Fatalf("stateOf(%+v) = %q, want %q", tt.act, got, tt.want)
			}
		})
	}
}

func TestAttribute(t *testing.T) {
	act := shared.Activity{App: "Code", Description: "写代码"}
	idle := shared.Activity{App: "Code", Description: "写代码", Idle: true, IdleSeconds: 400}
	locked := shared.Activity{App: "锁屏", Description: "屏幕已锁定", Locked: true}
	maxGap := 60 * time.Second

	tests := []struct {
		name     string
		observed shared.Activity
		from, to time.Time
		maxGap   time.Duration
		want     []Bucket
	}{
		{
			name:     "interval inside one hour",
			observed: act,
			from:     ts(13, 0, 0), to: ts(13, 0, 10),
			maxGap: maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 10},
			},
		},
		{
			name:     "interval split across the hour boundary",
			observed: act,
			from:     ts(13, 59, 55), to: ts(14, 0, 5),
			maxGap: maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
				{HourStart: ts(14, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
			},
		},
		{
			name:     "interval spanning a whole hour produces three buckets",
			observed: act,
			from:     ts(13, 30, 0), to: ts(15, 15, 0),
			maxGap: 3 * time.Hour,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 1800},
				{HourStart: ts(14, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 3600},
				{HourStart: ts(15, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 900},
			},
		},
		{
			name:     "idle interval is attributed to idle",
			observed: idle,
			from:     ts(13, 0, 0), to: ts(13, 0, 10),
			maxGap: maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateIdle, App: "Code", Description: "写代码", Seconds: 10},
			},
		},
		{
			name:     "locked interval is attributed to locked, never idle",
			observed: locked,
			from:     ts(13, 0, 0), to: ts(13, 0, 10),
			maxGap: maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateLocked, App: "锁屏", Description: "屏幕已锁定", Seconds: 10},
			},
		},
		{
			name:     "gap longer than maxGap is not attributed at all",
			observed: act,
			from:     ts(1, 0, 0), to: ts(9, 0, 0),
			maxGap: maxGap,
			want:   nil,
		},
		{
			name:     "gap exactly at maxGap is still attributed",
			observed: act,
			from:     ts(13, 0, 0), to: ts(13, 1, 0),
			maxGap: maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 60},
			},
		},
		{
			name:     "to equal to from produces nothing",
			observed: act,
			from:     ts(13, 0, 0), to: ts(13, 0, 0),
			maxGap: maxGap,
			want:   nil,
		},
		{
			name:     "to before from produces nothing",
			observed: act,
			from:     ts(13, 0, 10), to: ts(13, 0, 0),
			maxGap: maxGap,
			want:   nil,
		},
		{
			name:     "sub-second interval truncates to nothing",
			observed: act,
			from:     time.Date(2026, 7, 30, 13, 0, 0, 100_000_000, time.UTC),
			to:       time.Date(2026, 7, 30, 13, 0, 0, 900_000_000, time.UTC),
			maxGap:   maxGap,
			want:     nil,
		},
		{
			// Truncating both bounds once keeps the boundary split exact: a
			// per-hour truncation would have lost a second here and reported 9.
			name:     "sub-second remainder is dropped once, not per hour",
			observed: act,
			from:     time.Date(2026, 7, 30, 13, 59, 55, 500_000_000, time.UTC),
			to:       time.Date(2026, 7, 30, 14, 0, 5, 500_000_000, time.UTC),
			maxGap:   maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
				{HourStart: ts(14, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
			},
		},
		{
			name:     "non-UTC input is normalized to UTC hour buckets",
			observed: act,
			from:     ts(13, 59, 55).In(time.FixedZone("UTC+8", 8*3600)),
			to:       ts(14, 0, 5).In(time.FixedZone("UTC+8", 8*3600)),
			maxGap:   maxGap,
			want: []Bucket{
				{HourStart: ts(13, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
				{HourStart: ts(14, 0, 0), State: StateActive, App: "Code", Description: "写代码", Seconds: 5},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Attribute(tt.observed, tt.from, tt.to, tt.maxGap)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d buckets %+v, want %d %+v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if !got[i].HourStart.Equal(tt.want[i].HourStart) {
					t.Errorf("bucket %d hour_start = %s, want %s", i, got[i].HourStart, tt.want[i].HourStart)
				}
				if got[i].State != tt.want[i].State {
					t.Errorf("bucket %d state = %q, want %q", i, got[i].State, tt.want[i].State)
				}
				if got[i].App != tt.want[i].App || got[i].Description != tt.want[i].Description {
					t.Errorf("bucket %d activity = %q/%q, want %q/%q",
						i, got[i].App, got[i].Description, tt.want[i].App, tt.want[i].Description)
				}
				if got[i].Seconds != tt.want[i].Seconds {
					t.Errorf("bucket %d seconds = %d, want %d", i, got[i].Seconds, tt.want[i].Seconds)
				}
			}
		})
	}
}

// TestAttributeSecondsSumMatchesInterval is the invariant behind AC "sum of
// attributed time equals elapsed time": however the interval is split, the
// buckets must add up to the whole interval.
func TestAttributeSecondsSumMatchesInterval(t *testing.T) {
	act := shared.Activity{App: "Code", Description: "写代码"}
	from := ts(9, 42, 17)
	for _, d := range []time.Duration{time.Second, 10 * time.Second, 43 * time.Minute, 90 * time.Minute, 3 * time.Hour} {
		to := from.Add(d)
		total := 0
		for _, b := range Attribute(act, from, to, 4*time.Hour) {
			total += b.Seconds
		}
		if want := int(d / time.Second); total != want {
			t.Errorf("interval %s: attributed %d seconds, want %d", d, total, want)
		}
	}
}

func TestWindowDays(t *testing.T) {
	tests := []struct {
		window   string
		wantDays int
		wantOK   bool
	}{
		{WindowToday, 1, true},
		{Window7d, 7, true},
		{Window30d, 30, true},
		{"", 0, false},
		{"1d", 0, false},
		{"TODAY", 0, false},
	}
	for _, tt := range tests {
		days, ok := WindowDays(tt.window)
		if days != tt.wantDays || ok != tt.wantOK {
			t.Errorf("WindowDays(%q) = %d, %v; want %d, %v", tt.window, days, ok, tt.wantDays, tt.wantOK)
		}
	}
}
