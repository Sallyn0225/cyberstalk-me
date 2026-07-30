package usage

import (
	"sort"
	"time"

	"cyberstalk.me/shared"
)

// The windows accepted by GET /api/v1/usage.
const (
	WindowToday = "today"
	Window7d    = "7d"
	Window30d   = "30d"
)

// dateLayout is the wire format of DayUsage.Date.
const dateLayout = "2006-01-02"

// WindowDays returns how many local days the window covers, today included.
// ok is false for an unknown window; the caller must reject it rather than
// silently falling back to a default.
func WindowDays(w shared.UsageWindow) (days int, ok bool) {
	switch w {
	case WindowToday:
		return 1, true
	case Window7d:
		return 7, true
	case Window30d:
		return 30, true
	default:
		return 0, false
	}
}

// Row is one stored usage bucket. usage keeps its own input type rather than
// accepting store's row struct, so it never imports store.
type Row struct {
	DeviceID    string
	HourStart   time.Time
	State       shared.UsageState
	App         string
	Description string
	Seconds     int
}

// Device is a device that must appear in the response even when it has no
// buckets in the window. It is also the only source of device identity, so a
// device that has never reported simply never appears.
type Device struct {
	DeviceID   string
	DeviceName string
	DeviceType shared.DeviceType
}

// Aggregate turns stored buckets into the wire response. loc is the site
// timezone: local hour-of-day and local date are derived here, so the SQL
// layer never deals with timezones.
//
// from and to are the window bounds already used for the query; they are
// echoed back and used to lay out the fixed day slots. Rows for devices
// absent from devices are ignored.
func Aggregate(devices []Device, rows []Row, w shared.UsageWindow, from, to time.Time, loc *time.Location) shared.UsageResponse {
	if loc == nil {
		loc = time.UTC
	}
	byDevice := make(map[string][]Row, len(devices))
	for _, r := range rows {
		byDevice[r.DeviceID] = append(byDevice[r.DeviceID], r)
	}

	sorted := make([]Device, len(devices))
	copy(sorted, devices)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DeviceID < sorted[j].DeviceID })

	out := make([]shared.DeviceUsage, 0, len(sorted))
	for _, d := range sorted {
		out = append(out, aggregateDevice(d, byDevice[d.DeviceID], w, from, to, loc))
	}
	return shared.UsageResponse{
		Window:   w,
		Timezone: loc.String(),
		From:     from.UTC(),
		To:       to.UTC(),
		Devices:  out,
	}
}

func aggregateDevice(d Device, rows []Row, w shared.UsageWindow, from, to time.Time, loc *time.Location) shared.DeviceUsage {
	du := shared.DeviceUsage{
		DeviceID:   d.DeviceID,
		DeviceName: d.DeviceName,
		DeviceType: d.DeviceType,
	}
	// app -> description -> seconds, active only.
	perApp := make(map[string]map[string]int)
	for _, r := range rows {
		switch r.State {
		case StateActive:
			du.Totals.ActiveSeconds += r.Seconds
		case StateIdle:
			du.Totals.IdleSeconds += r.Seconds
		case StateLocked:
			du.Totals.LockedSeconds += r.Seconds
		}
		if r.State != StateActive {
			// Idle and locked time is only ever a total: putting it in the app
			// ranking would read as "you used this app for eight hours".
			continue
		}
		if perApp[r.App] == nil {
			perApp[r.App] = make(map[string]int)
		}
		perApp[r.App][r.Description] += r.Seconds
	}
	du.Apps = rankApps(perApp)
	if w == WindowToday {
		du.Hourly = hourlySlots(rows, loc)
	} else {
		du.Daily = daySlots(rows, from, to, loc)
	}
	return du
}

// rankApps flattens the two-level map into the descending ranking. Ties break
// on name so the order is stable across requests.
func rankApps(perApp map[string]map[string]int) []shared.AppUsage {
	apps := make([]shared.AppUsage, 0, len(perApp))
	for app, byDesc := range perApp {
		total := 0
		acts := make([]shared.ActivityUsage, 0, len(byDesc))
		for desc, seconds := range byDesc {
			total += seconds
			acts = append(acts, shared.ActivityUsage{Description: desc, Seconds: seconds})
		}
		sort.Slice(acts, func(i, j int) bool {
			if acts[i].Seconds != acts[j].Seconds {
				return acts[i].Seconds > acts[j].Seconds
			}
			return acts[i].Description < acts[j].Description
		})
		apps = append(apps, shared.AppUsage{App: app, Seconds: total, Activities: acts})
	}
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Seconds != apps[j].Seconds {
			return apps[i].Seconds > apps[j].Seconds
		}
		return apps[i].App < apps[j].App
	})
	return apps
}

// hourlySlots returns all 24 hours of the local day, empty ones included, so
// the frontend can render a fixed chart without filling gaps itself.
func hourlySlots(rows []Row, loc *time.Location) []shared.HourUsage {
	perHour := make([]map[string]int, 24)
	for _, r := range rows {
		if r.State != StateActive {
			continue
		}
		h := r.HourStart.In(loc).Hour()
		if perHour[h] == nil {
			perHour[h] = make(map[string]int)
		}
		perHour[h][r.App] += r.Seconds
	}
	out := make([]shared.HourUsage, 24)
	for h := range out {
		seconds, top := sumAndTop(perHour[h])
		out[h] = shared.HourUsage{Hour: h, Seconds: seconds, TopApp: top}
	}
	return out
}

// daySlots returns one entry per local day covered by [from, to), empty days
// included.
func daySlots(rows []Row, from, to time.Time, loc *time.Location) []shared.DayUsage {
	perDay := make(map[string]map[string]int)
	for _, r := range rows {
		if r.State != StateActive {
			continue
		}
		key := r.HourStart.In(loc).Format(dateLayout)
		if perDay[key] == nil {
			perDay[key] = make(map[string]int)
		}
		perDay[key][r.App] += r.Seconds
	}
	out := make([]shared.DayUsage, 0, 31)
	// Step midnight to midnight in loc rather than adding 24h, so a DST
	// transition inside the window cannot skip or duplicate a date.
	for cur := startOfDay(from, loc); cur.Before(to); cur = startOfDay(cur.AddDate(0, 0, 1), loc) {
		key := cur.Format(dateLayout)
		seconds, top := sumAndTop(perDay[key])
		out = append(out, shared.DayUsage{Date: key, Seconds: seconds, TopApp: top})
	}
	return out
}

// startOfDay returns local midnight of the day containing t.
func startOfDay(t time.Time, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

// sumAndTop returns the total seconds and the app holding the most of them.
// Iteration is over sorted names so ties resolve to the first alphabetically
// instead of whatever the map hands back. TopApp is "" when there is no usage.
func sumAndTop(byApp map[string]int) (int, string) {
	apps := make([]string, 0, len(byApp))
	for app := range byApp {
		apps = append(apps, app)
	}
	sort.Strings(apps)

	total, top, best := 0, "", -1
	for _, app := range apps {
		seconds := byApp[app]
		total += seconds
		if seconds > best {
			best, top = seconds, app
		}
	}
	if total == 0 {
		return 0, ""
	}
	return total, top
}
