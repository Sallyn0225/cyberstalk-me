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
