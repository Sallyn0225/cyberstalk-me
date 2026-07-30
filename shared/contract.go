// Package shared defines the data contract shared between the Go backend
// and the Go Windows client. It contains ONLY data types — no business
// logic, and no third-party imports beyond the standard library.
//
// This is the single source of truth for the wire contract. Raw window
// titles, process paths, and device tokens are intentionally absent: the
// contract only ever carries already-sanitized fields, so unsanitized data
// can never leave a device. The frontend TypeScript types in web/src/types/
// are expected to mirror these structs (see the cross-layer thinking guide).
package shared

import "time"

// DeviceType is the kind of reporting device. Known values: "windows",
// "android". It is a plain string (not a typed enum) so JSON encoding is
// trivial and adding a new device type needs no contract-wide change.
type DeviceType = string

// NetworkType describes the device's current network. Known values:
// "wifi", "cellular", "ethernet", "offline". Nil means unknown/unreported.
type NetworkType = string

// ReportPayload is the body of POST /api/v1/report. Every field has already
// been sanitized on the device; the server never receives raw titles.
type ReportPayload struct {
	DeviceID   string       `json:"device_id"`
	DeviceName string       `json:"device_name"`
	DeviceType DeviceType   `json:"device_type"` // "windows" | "android"
	Activity   Activity     `json:"activity"`
	Battery    *Battery     `json:"battery"`     // desktop without battery -> null
	Network    *NetworkType `json:"network"`     // wifi|cellular|ethernet|offline|null
	ReportedAt time.Time    `json:"reported_at"` // client clock, display/debug only
}

// Activity is the sanitized description of what the user is currently doing.
// App and Description are produced by the device-side mapping rules; the raw
// window title never appears here.
type Activity struct {
	App         string `json:"app"`          // mapped app name
	Description string `json:"description"`  // mapped friendly description
	Idle        bool   `json:"idle"`         // user is idle (no input for a while)
	IdleSeconds int    `json:"idle_seconds"` // seconds since last input
	// Locked reports that there was no foreground window at all (lock screen
	// or session switch). It is a structured flag rather than something the
	// server infers from App, because App is a user-configured string
	// (locked_app in the agent config) that the server cannot match on.
	//
	// A client that predates this field omits it, which decodes to false. That
	// degrades badly, not safely: Windows keeps reporting zero idle seconds
	// while the screen is locked, so an old agent's locked stretches look like
	// active use of whatever locked_app is named. Upgrading the agent is the
	// only fix — the server cannot recover the distinction on its own, which
	// is exactly why this flag exists.
	Locked bool `json:"locked"`
}

// Battery is the device power state. Level may be nil when the OS cannot
// report a percentage (e.g. some desktops). Charging is false when not on
// AC / not charging.
type Battery struct {
	Level    *int `json:"level"` // 0-100, may be nil
	Charging bool `json:"charging"`
}

// EventType is the kind of SSE event. Known values: "update", "offline".
type EventType = string

// Event is a single Server-Sent Event payload pushed to all subscribers.
type Event struct {
	Type   EventType   `json:"type"` // "update" | "offline"
	Device DeviceState `json:"device"`
}

// UsageWindow is the requested aggregation window for GET /api/v1/usage.
// Known values: "today", "7d", "30d". Plain string for the same reason as
// DeviceType.
type UsageWindow = string

// UsageState is which bucket a second of wall-clock time was attributed to.
// Known values: "active", "idle", "locked".
type UsageState = string

// UsageResponse is the body of GET /api/v1/usage.
type UsageResponse struct {
	Window   UsageWindow   `json:"window"`
	Timezone string        `json:"timezone"` // IANA name the window was computed in
	From     time.Time     `json:"from"`     // inclusive, UTC
	To       time.Time     `json:"to"`       // exclusive, UTC
	Devices  []DeviceUsage `json:"devices"`
}

// DeviceUsage is one device's usage over the requested window.
type DeviceUsage struct {
	DeviceID   string      `json:"device_id"`
	DeviceName string      `json:"device_name"`
	DeviceType DeviceType  `json:"device_type"`
	Totals     UsageTotals `json:"totals"`
	// Apps is the active-time ranking, descending. Idle and locked time never
	// appear here — only in Totals.
	Apps []AppUsage `json:"apps"`
	// Hourly is set for window "today" and null otherwise; Daily is set for
	// "7d"/"30d" and null otherwise. Exactly one of them is non-null.
	Hourly []HourUsage `json:"hourly"`
	Daily  []DayUsage  `json:"daily"`
}

// UsageTotals is the per-state second count for one device.
type UsageTotals struct {
	ActiveSeconds int `json:"active_seconds"`
	IdleSeconds   int `json:"idle_seconds"`
	LockedSeconds int `json:"locked_seconds"`
}

// AppUsage is one app's active time.
type AppUsage struct {
	App     string `json:"app"`
	Seconds int    `json:"seconds"` // active only
	// Activities is the per-description breakdown of Seconds, descending.
	// Its Seconds sum equals AppUsage.Seconds.
	Activities []ActivityUsage `json:"activities"`
}

// ActivityUsage is one mapped description's active time within an app.
type ActivityUsage struct {
	Description string `json:"description"`
	Seconds     int    `json:"seconds"`
}

// HourUsage is one hour of the local day. Hours with no usage are still
// present with Seconds 0, so the frontend can render a fixed 24-slot chart
// without filling gaps itself.
type HourUsage struct {
	Hour    int    `json:"hour"`    // 0-23 in Timezone
	Seconds int    `json:"seconds"` // active
	TopApp  string `json:"top_app"` // "" when Seconds is 0
}

// DayUsage is one local day. Days with no usage are present with Seconds 0.
type DayUsage struct {
	Date    string `json:"date"`    // YYYY-MM-DD in Timezone
	Seconds int    `json:"seconds"` // active
	TopApp  string `json:"top_app"` // "" when Seconds is 0
}

// DeviceState is the full state of a device as delivered to the frontend
// (both via GET /api/v1/snapshot and SSE events). It is the server's
// projection of the latest report plus its own online/offline judgment.
type DeviceState struct {
	DeviceID   string       `json:"device_id"`
	DeviceName string       `json:"device_name"`
	DeviceType DeviceType   `json:"device_type"`
	Activity   Activity     `json:"activity"`
	Battery    *Battery     `json:"battery"`
	Network    *NetworkType `json:"network"`
	Online     bool         `json:"online"`
	ReportedAt time.Time    `json:"reported_at"`  // client clock
	LastSeenAt time.Time    `json:"last_seen_at"` // server clock, basis for online judgment
}
