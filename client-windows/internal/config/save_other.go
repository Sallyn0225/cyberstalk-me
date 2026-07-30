//go:build !windows

package config

import (
	"fmt"
	"os"
)

// replaceFile is the non-Windows stand-in for the ReplaceFileW path.
//
// The agent only ever runs on Windows; this exists so the pure packages stay
// buildable and testable on a Linux CI runner. It reproduces the observable
// behaviour — dst's old contents end up at bak, dst ends up as tmp — but not
// the ACL preservation, which has no meaning here.
func replaceFile(dst, tmp, bak string) error {
	if err := os.Rename(dst, bak); err != nil {
		return fmt.Errorf("back up %s: %w", dst, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("move %s into place: %w", tmp, err)
	}
	return nil
}
