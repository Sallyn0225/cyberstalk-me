package shared

import (
	"encoding/json"
	"strings"
	"testing"
)

// A report sent by a client built before Activity.Locked existed, captured
// from a real locked Windows session: note idle_seconds 0. Windows stops
// advancing GetLastInputInfo while the screen is locked, so a locked machine
// does NOT look idle. Decoding must not fail, and the values below are why the
// flag had to be added — see TestUnmarshalReportWithoutLocked.
const legacyReportJSON = `{
  "device_id": "win-desktop",
  "device_name": "我的台式机",
  "device_type": "windows",
  "activity": {
    "app": "已锁屏",
    "description": "人不在",
    "idle": false,
    "idle_seconds": 0
  },
  "battery": null,
  "network": "wifi",
  "reported_at": "2026-07-29T12:00:00Z"
}`

func TestUnmarshalReportWithoutLocked(t *testing.T) {
	var p ReportPayload
	if err := json.Unmarshal([]byte(legacyReportJSON), &p); err != nil {
		t.Fatalf("decoding a pre-Locked report failed: %v", err)
	}
	if p.Activity.Locked {
		t.Errorf("Locked = true for a payload that has no locked key, want false")
	}
	// The rest of the payload must be unaffected by the added field.
	if p.Activity.App != "已锁屏" || p.Activity.Idle || p.Activity.IdleSeconds != 0 {
		t.Errorf("legacy activity decoded wrong: %+v", p.Activity)
	}
	// This is the whole argument for the flag, spelled out: without it, a
	// locked machine is indistinguishable from one actively using an app named
	// whatever locked_app happens to be. There is no field combination the
	// server could key on instead, so an old agent's locked time will be
	// counted as active. The fix is to upgrade the agent, not to guess here.
	if p.Activity.Locked || p.Activity.Idle {
		t.Errorf("this payload is supposed to be the ambiguous case: %+v", p.Activity)
	}
}

func TestActivityLockedRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		locked bool
		want   string
	}{
		{"locked", true, `"locked":true`},
		{"not locked", false, `"locked":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(Activity{App: "VS Code", Locked: tt.locked})
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			// The JSON tag is what the frontend mirror and the server both key
			// on; a rename here is a silent cross-layer break.
			if !strings.Contains(string(encoded), tt.want) {
				t.Errorf("encoded activity = %s, want it to contain %s", encoded, tt.want)
			}
			var back Activity
			if err := json.Unmarshal(encoded, &back); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if back.Locked != tt.locked {
				t.Errorf("round-tripped Locked = %v, want %v", back.Locked, tt.locked)
			}
		})
	}
}
