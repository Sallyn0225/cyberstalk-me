//go:build windows

// Package collect gathers the raw device state through Win32: the foreground
// process, idle time, battery and network type.
//
// The package is Windows-only by construction — there is no cross-platform
// stub, because a stub would only buy a fake green build for code that can
// never run anywhere else.
//
// Privacy: the raw window title is never returned as a string field. It is
// reachable only through the lazy Snapshot.Title method, which the mapping
// package calls solely when a rule needs it.
//
// Every collector degrades on failure (zero value / nil) instead of aborting
// the cycle; one failing Win32 call must not cost a whole report.
package collect

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"cyberstalk.me/shared"
)

// UnknownProcess marks a foreground window whose process could not be
// identified (an elevated window denies OpenProcess to a non-elevated agent).
// It contains '?', which is illegal in a Windows file name, so it can never
// collide with a mapping rule and always falls through to the generic
// description.
const UnknownProcess = "?unknown"

// Network types reported to the server (mirrors shared.NetworkType).
const (
	networkEthernet = "ethernet"
	networkWiFi     = "wifi"
	networkCellular = "cellular"
	networkOffline  = "offline"
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	// Not exported by golang.org/x/sys/windows, so declared here.
	procGetWindowTextW       = modUser32.NewProc("GetWindowTextW")
	procGetLastInputInfo     = modUser32.NewProc("GetLastInputInfo")
	procGetTickCount64       = modKernel32.NewProc("GetTickCount64")
	procGetSystemPowerStatus = modKernel32.NewProc("GetSystemPowerStatus")
)

// Snapshot is one cycle's raw device state.
type Snapshot struct {
	// Process is the lowercased executable base name of the foreground
	// window ("code.exe"), UnknownProcess when it could not be read, or ""
	// when there is no foreground window at all (lock screen).
	Process string
	// IdleSeconds is the time since the last keyboard/mouse input.
	IdleSeconds int
	// Battery is nil on a machine without a battery.
	Battery *shared.Battery
	// Network is one of wifi/ethernet/cellular/offline, or "" if unknown.
	Network shared.NetworkType

	// title lazily reads the raw window title. It is unexported and has no
	// json tag on purpose: the raw title must never be serialized, logged or
	// passed around as a plain string.
	title func() string
}

// Title reads the raw window title of the foreground window. It is safe to
// call on a zero Snapshot and returns "" when there is no window.
//
// The result is unsanitized and must not be logged, stored or reported. Its
// only legitimate consumer is mapping.Resolve.
func (s Snapshot) Title() string {
	if s.title == nil {
		return ""
	}
	return s.title()
}

// Collect takes one snapshot of the current device state.
func Collect() Snapshot {
	snap := Snapshot{
		IdleSeconds: idleSeconds(),
		Battery:     battery(),
		Network:     network(),
	}
	if hwnd := windows.GetForegroundWindow(); hwnd != 0 {
		snap.Process = foregroundProcess(hwnd)
		snap.title = func() string { return windowText(hwnd) }
	}
	return snap
}

// foregroundProcess resolves the executable base name behind hwnd. Failures
// degrade to UnknownProcess: an elevated window is not something to crash or
// give up over.
func foregroundProcess(hwnd windows.HWND) string {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		slog.Debug("collect: foreground window has no pid", "err", err)
		return UnknownProcess
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// Typically ERROR_ACCESS_DENIED for an elevated process.
		slog.Debug("collect: open foreground process", "err", err)
		return UnknownProcess
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		slog.Debug("collect: query process image name", "err", err)
		return UnknownProcess
	}
	// Only the base name survives; the full path is as sensitive as a title
	// and is never logged or returned.
	return strings.ToLower(filepath.Base(windows.UTF16ToString(buf[:size])))
}

// windowText reads the raw window title. Called lazily, only from
// Snapshot.Title.
func windowText(hwnd windows.HWND) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// lastInputInfo mirrors the Win32 LASTINPUTINFO struct.
type lastInputInfo struct {
	cbSize uint32
	dwTime uint32 // 32-bit tick count, wraps every ~49.7 days
}

// idleSeconds returns the seconds since the last keyboard or mouse input.
func idleSeconds() int {
	info := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	ok, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		slog.Debug("collect: get last input info", "err", err)
		return 0
	}
	ticks, _, _ := procGetTickCount64.Call()
	// dwTime is 32-bit. Truncating the 64-bit tick count to 32 bits makes the
	// subtraction wrap the same way, so uptime past 49.7 days stays correct;
	// subtracting from the 64-bit value would produce astronomical idle times.
	elapsed := uint32(ticks) - info.dwTime
	return int(elapsed / 1000)
}

// systemPowerStatus mirrors the Win32 SYSTEM_POWER_STATUS struct.
type systemPowerStatus struct {
	acLineStatus        byte
	batteryFlag         byte
	batteryLifePercent  byte
	systemStatusFlag    byte
	batteryLifeTime     uint32
	batteryFullLifeTime uint32
}

// battery returns the power state, or nil when the machine has no battery or
// Windows cannot report one. Nil means the site hides the battery block
// entirely — better than inventing a 0%.
func battery() *shared.Battery {
	const (
		batteryFlagNoSystemBattery = 128
		unknownValue               = 255
		acLineStatusOnline         = 1
	)
	var status systemPowerStatus
	ok, _, err := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		slog.Debug("collect: get system power status", "err", err)
		return nil
	}
	if status.batteryFlag&batteryFlagNoSystemBattery != 0 ||
		status.batteryFlag == unknownValue ||
		status.batteryLifePercent == unknownValue {
		return nil
	}
	level := int(status.batteryLifePercent)
	return &shared.Battery{
		Level:    &level,
		Charging: status.acLineStatus == acLineStatusOnline,
	}
}

// network reports the active connection type. Adapters that are down, are
// loopback, or have no default gateway are ignored; among the rest wired wins
// over Wi-Fi, which wins over cellular (a laptop on a docking station has both
// up). "" means the lookup failed; "offline" means nothing is connected.
func network() shared.NetworkType {
	const (
		gaaFlagSkipUnicast     = 0x0001
		gaaFlagSkipAnycast     = 0x0002
		gaaFlagSkipMulticast   = 0x0004
		gaaFlagSkipDNSServer   = 0x0008
		gaaFlagIncludeGateways = 0x0080
		ifTypeWWANPP           = 243 // GSM
		ifTypeWWANPP2          = 244 // CDMA
		initialBufferSize      = 15000
		maxGetAdaptersAttempts = 3
		rankNone, rankCellular = 0, 1
		rankWiFi, rankEthernet = 2, 3
	)
	flags := uint32(gaaFlagSkipUnicast | gaaFlagSkipAnycast | gaaFlagSkipMulticast |
		gaaFlagSkipDNSServer | gaaFlagIncludeGateways)

	size := uint32(initialBufferSize)
	var adapters *windows.IpAdapterAddresses
	for attempt := 0; ; attempt++ {
		buf := make([]byte, size)
		adapters = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, adapters, &size)
		if err == nil {
			break
		}
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) && attempt < maxGetAdaptersAttempts {
			// size now holds the required length; retry with it.
			continue
		}
		slog.Debug("collect: get adapters addresses", "err", err)
		return ""
	}

	best, bestRank := networkOffline, rankNone
	for a := adapters; a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		// No default gateway means the adapter carries no internet traffic
		// (virtual switches, VPN leftovers, disconnected NICs).
		if a.FirstGatewayAddress == nil {
			continue
		}
		kind, rank := "", rankNone
		switch a.IfType {
		case windows.IF_TYPE_ETHERNET_CSMACD:
			kind, rank = networkEthernet, rankEthernet
		case windows.IF_TYPE_IEEE80211:
			kind, rank = networkWiFi, rankWiFi
		case ifTypeWWANPP, ifTypeWWANPP2:
			kind, rank = networkCellular, rankCellular
		default:
			continue
		}
		if rank > bestRank {
			best, bestRank = kind, rank
		}
	}
	return best
}
