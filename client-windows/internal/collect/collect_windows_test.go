//go:build windows

package collect

import "testing"

// TestIsLockScreenProcess pins the set of process names Collect treats as the
// lock/logon screen (and thus reports as locked via Process=""). The set is
// small on purpose: only Windows system processes that exist solely to render
// the lock screen or sign-in prompt, so a user app can never trip it.
func TestIsLockScreenProcess(t *testing.T) {
	for _, p := range []string{"lockapp.exe", "logonui.exe"} {
		if !isLockScreenProcess(p) {
			t.Errorf("isLockScreenProcess(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"chrome.exe", "code.exe", "explorer.exe", "", UnknownProcess} {
		if isLockScreenProcess(p) {
			t.Errorf("isLockScreenProcess(%q) = true, want false", p)
		}
	}
}
