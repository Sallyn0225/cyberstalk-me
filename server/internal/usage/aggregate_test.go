package usage

import (
	"testing"
	"time"

	"cyberstalk.me/shared"
)

// shanghai is a fixed +08:00 zone. The tests use a fixed offset rather than
// time.LoadLocation("Asia/Shanghai") so they do not depend on a tz database
// being present on the machine running them; the offset is what the
// aggregation actually uses.
var shanghai = time.FixedZone("UTC+8", 8*3600)

func hourUTC(day, hour int) time.Time {
	return time.Date(2026, 7, day, hour, 0, 0, 0, time.UTC)
}

func devices() []Device {
	return []Device{
		{DeviceID: "pc", DeviceName: "My PC", DeviceType: "windows"},
		{DeviceID: "phone", DeviceName: "My Phone", DeviceType: "android"},
	}
}

func TestAggregateTotalsAndRanking(t *testing.T) {
	rows := []Row{
		{DeviceID: "pc", HourStart: hourUTC(30, 1), State: StateActive, App: "Code", Description: "写代码", Seconds: 600},
		{DeviceID: "pc", HourStart: hourUTC(30, 2), State: StateActive, App: "Code", Description: "写代码", Seconds: 300},
		{DeviceID: "pc", HourStart: hourUTC(30, 2), State: StateActive, App: "Code", Description: "看文档", Seconds: 120},
		{DeviceID: "pc", HourStart: hourUTC(30, 2), State: StateActive, App: "Chrome", Description: "上网", Seconds: 900},
		{DeviceID: "pc", HourStart: hourUTC(30, 3), State: StateIdle, App: "Code", Description: "写代码", Seconds: 1200},
		{DeviceID: "pc", HourStart: hourUTC(30, 4), State: StateLocked, App: "锁屏", Description: "屏幕已锁定", Seconds: 3600},
	}
	from := time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 7, 30, 20, 0, 0, 0, shanghai)

	resp := Aggregate(devices(), rows, WindowToday, from, to, shanghai)

	if resp.Window != WindowToday {
		t.Errorf("window = %q, want %q", resp.Window, WindowToday)
	}
	if resp.Timezone != shanghai.String() {
		t.Errorf("timezone = %q, want %q", resp.Timezone, shanghai.String())
	}
	if !resp.From.Equal(from) || !resp.To.Equal(to) {
		t.Errorf("bounds = %s..%s, want %s..%s", resp.From, resp.To, from, to)
	}
	// Devices are ordered by device_id, matching ListStates and the frontend.
	if len(resp.Devices) != 2 || resp.Devices[0].DeviceID != "pc" || resp.Devices[1].DeviceID != "phone" {
		t.Fatalf("unexpected device order: %+v", resp.Devices)
	}

	pc := resp.Devices[0]
	if pc.Totals.ActiveSeconds != 1920 {
		t.Errorf("active total = %d, want 1920", pc.Totals.ActiveSeconds)
	}
	if pc.Totals.IdleSeconds != 1200 {
		t.Errorf("idle total = %d, want 1200", pc.Totals.IdleSeconds)
	}
	if pc.Totals.LockedSeconds != 3600 {
		t.Errorf("locked total = %d, want 3600", pc.Totals.LockedSeconds)
	}

	// Only active time is ranked; the locked app must not appear.
	if len(pc.Apps) != 2 {
		t.Fatalf("apps = %+v, want 2 entries", pc.Apps)
	}
	if pc.Apps[0].App != "Code" || pc.Apps[0].Seconds != 1020 {
		t.Errorf("top app = %q/%d, want Code/1020", pc.Apps[0].App, pc.Apps[0].Seconds)
	}
	if pc.Apps[1].App != "Chrome" || pc.Apps[1].Seconds != 900 {
		t.Errorf("second app = %q/%d, want Chrome/900", pc.Apps[1].App, pc.Apps[1].Seconds)
	}
	for _, app := range pc.Apps {
		if app.App == "锁屏" {
			t.Errorf("locked app leaked into the ranking: %+v", app)
		}
	}

	// AC: each app's activities sum to that app's seconds.
	for _, app := range pc.Apps {
		sum := 0
		for _, a := range app.Activities {
			sum += a.Seconds
		}
		if sum != app.Seconds {
			t.Errorf("app %q: activities sum %d != seconds %d", app.App, sum, app.Seconds)
		}
	}
	if len(pc.Apps[0].Activities) != 2 ||
		pc.Apps[0].Activities[0].Description != "写代码" ||
		pc.Apps[0].Activities[1].Description != "看文档" {
		t.Errorf("activities not sorted descending: %+v", pc.Apps[0].Activities)
	}

	// A device with no buckets in the window still appears, with zeros.
	phone := resp.Devices[1]
	if phone.Totals != (shared.UsageTotals{}) {
		t.Errorf("phone totals = %+v, want zeros", phone.Totals)
	}
	if phone.Apps == nil || len(phone.Apps) != 0 {
		t.Errorf("phone apps = %+v, want an empty non-nil slice", phone.Apps)
	}
}

func TestAggregateTieBreaksOnName(t *testing.T) {
	rows := []Row{
		{DeviceID: "pc", HourStart: hourUTC(30, 1), State: StateActive, App: "Zed", Description: "b", Seconds: 100},
		{DeviceID: "pc", HourStart: hourUTC(30, 1), State: StateActive, App: "Alpha", Description: "a", Seconds: 100},
		{DeviceID: "pc", HourStart: hourUTC(30, 1), State: StateActive, App: "Mid", Description: "c", Seconds: 100},
	}
	from := time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 7, 30, 20, 0, 0, 0, shanghai)

	// Run repeatedly: map iteration order is randomized, so an unstable sort
	// would only fail intermittently.
	for i := 0; i < 20; i++ {
		resp := Aggregate([]Device{{DeviceID: "pc"}}, rows, WindowToday, from, to, shanghai)
		apps := resp.Devices[0].Apps
		if len(apps) != 3 || apps[0].App != "Alpha" || apps[1].App != "Mid" || apps[2].App != "Zed" {
			t.Fatalf("iteration %d: unstable order %+v", i, apps)
		}
		if apps[0].Activities[0].Description != "a" {
			t.Fatalf("iteration %d: unexpected activity %+v", i, apps[0].Activities)
		}
	}
}

func TestAggregateHourlyHasAll24Slots(t *testing.T) {
	rows := []Row{
		// 2026-07-29T16:00Z is 2026-07-30 00:00 in +08:00 — local hour 0.
		{DeviceID: "pc", HourStart: hourUTC(29, 16), State: StateActive, App: "Code", Description: "写代码", Seconds: 60},
		// 2026-07-30T05:00Z is local hour 13.
		{DeviceID: "pc", HourStart: hourUTC(30, 5), State: StateActive, App: "Chrome", Description: "上网", Seconds: 300},
		{DeviceID: "pc", HourStart: hourUTC(30, 5), State: StateActive, App: "Code", Description: "写代码", Seconds: 100},
		// Idle time must not show up in the chart, which is active-only.
		{DeviceID: "pc", HourStart: hourUTC(30, 6), State: StateIdle, App: "Code", Description: "写代码", Seconds: 900},
	}
	from := time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 7, 30, 20, 0, 0, 0, shanghai)

	resp := Aggregate([]Device{{DeviceID: "pc"}}, rows, WindowToday, from, to, shanghai)
	pc := resp.Devices[0]
	if pc.Daily != nil {
		t.Errorf("daily should be nil for window today, got %+v", pc.Daily)
	}
	if len(pc.Hourly) != 24 {
		t.Fatalf("hourly has %d slots, want 24", len(pc.Hourly))
	}
	for h, slot := range pc.Hourly {
		if slot.Hour != h {
			t.Fatalf("slot %d has hour %d", h, slot.Hour)
		}
	}
	if pc.Hourly[0].Seconds != 60 || pc.Hourly[0].TopApp != "Code" {
		t.Errorf("hour 0 = %+v, want 60s of Code", pc.Hourly[0])
	}
	if pc.Hourly[13].Seconds != 400 || pc.Hourly[13].TopApp != "Chrome" {
		t.Errorf("hour 13 = %+v, want 400s topped by Chrome", pc.Hourly[13])
	}
	if pc.Hourly[14].Seconds != 0 || pc.Hourly[14].TopApp != "" {
		t.Errorf("hour 14 = %+v, want an empty slot (idle time is not charted)", pc.Hourly[14])
	}
	if pc.Hourly[7].Seconds != 0 || pc.Hourly[7].TopApp != "" {
		t.Errorf("hour 7 = %+v, want an empty slot", pc.Hourly[7])
	}
}

func TestAggregateDailyCoversEveryDayInWindow(t *testing.T) {
	rows := []Row{
		{DeviceID: "pc", HourStart: hourUTC(24, 2), State: StateActive, App: "Code", Description: "写代码", Seconds: 120},
		{DeviceID: "pc", HourStart: hourUTC(30, 2), State: StateActive, App: "Chrome", Description: "上网", Seconds: 60},
		{DeviceID: "pc", HourStart: hourUTC(30, 3), State: StateActive, App: "Code", Description: "写代码", Seconds: 90},
	}
	// "7d" means seven local days including today, not the last 168 hours.
	from := time.Date(2026, 7, 24, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 7, 30, 15, 30, 0, 0, shanghai)

	resp := Aggregate([]Device{{DeviceID: "pc"}}, rows, Window7d, from, to, shanghai)
	pc := resp.Devices[0]
	if pc.Hourly != nil {
		t.Errorf("hourly should be nil for window 7d, got %+v", pc.Hourly)
	}
	if len(pc.Daily) != 7 {
		t.Fatalf("daily has %d entries, want 7: %+v", len(pc.Daily), pc.Daily)
	}
	wantDates := []string{"2026-07-24", "2026-07-25", "2026-07-26", "2026-07-27", "2026-07-28", "2026-07-29", "2026-07-30"}
	for i, want := range wantDates {
		if pc.Daily[i].Date != want {
			t.Fatalf("daily[%d].Date = %q, want %q", i, pc.Daily[i].Date, want)
		}
	}
	if pc.Daily[0].Seconds != 120 || pc.Daily[0].TopApp != "Code" {
		t.Errorf("first day = %+v, want 120s of Code", pc.Daily[0])
	}
	if pc.Daily[3].Seconds != 0 || pc.Daily[3].TopApp != "" {
		t.Errorf("empty day = %+v, want zeros", pc.Daily[3])
	}
	if pc.Daily[6].Seconds != 150 || pc.Daily[6].TopApp != "Code" {
		t.Errorf("today = %+v, want 150s topped by Code", pc.Daily[6])
	}
}

func TestAggregateEmptyInputs(t *testing.T) {
	from := time.Date(2026, 7, 30, 0, 0, 0, 0, shanghai)
	to := time.Date(2026, 7, 30, 20, 0, 0, 0, shanghai)

	resp := Aggregate(nil, nil, WindowToday, from, to, shanghai)
	if resp.Devices == nil || len(resp.Devices) != 0 {
		t.Fatalf("devices = %+v, want an empty non-nil slice", resp.Devices)
	}

	// Rows for a device that is not in the device list are dropped, which is
	// how "never reported" devices stay out of the response.
	orphan := []Row{{DeviceID: "ghost", HourStart: hourUTC(30, 1), State: StateActive, App: "Code", Seconds: 60}}
	resp = Aggregate(nil, orphan, WindowToday, from, to, shanghai)
	if len(resp.Devices) != 0 {
		t.Fatalf("orphan rows produced devices: %+v", resp.Devices)
	}
}

func TestAggregateNilLocationFallsBackToUTC(t *testing.T) {
	from := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 30, 20, 0, 0, 0, time.UTC)
	rows := []Row{{DeviceID: "pc", HourStart: hourUTC(30, 5), State: StateActive, App: "Code", Seconds: 60}}

	resp := Aggregate([]Device{{DeviceID: "pc"}}, rows, WindowToday, from, to, nil)
	if resp.Timezone != "UTC" {
		t.Errorf("timezone = %q, want UTC", resp.Timezone)
	}
	if resp.Devices[0].Hourly[5].Seconds != 60 {
		t.Errorf("hour 5 = %+v, want 60s", resp.Devices[0].Hourly[5])
	}
}
