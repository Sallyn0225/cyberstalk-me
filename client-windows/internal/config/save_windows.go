//go:build windows

package config

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ReplaceFileW is not exported by golang.org/x/sys/windows, so it is declared
// here the same way collect declares its user32 calls.
var procReplaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

// replaceFile atomically replaces dst with tmp, moving the old dst to bak.
//
// os.Rename would be the obvious choice and is the wrong one: on Windows it
// becomes MoveFileEx(MOVEFILE_REPLACE_EXISTING), which gives the destination
// the *source* file's ACL. config.yaml holds a device token and the README
// tells users to restrict who can read it, so renaming over it would quietly
// widen those permissions on every save.
//
// ReplaceFileW exists for exactly this: it keeps the destination's ACL and
// attributes, and its backup parameter writes the old contents to bak in the
// same operation.
func replaceFile(dst, tmp, bak string) error {
	dstPtr, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return fmt.Errorf("config path %s: %w", dst, err)
	}
	tmpPtr, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return fmt.Errorf("temp path %s: %w", tmp, err)
	}
	bakPtr, err := windows.UTF16PtrFromString(bak)
	if err != nil {
		return fmt.Errorf("backup path %s: %w", bak, err)
	}

	ok, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(dstPtr)),
		uintptr(unsafe.Pointer(tmpPtr)),
		uintptr(unsafe.Pointer(bakPtr)),
		0, // dwReplaceFlags
		0, // lpExclude, reserved
		0, // lpReserved
	)
	if ok == 0 {
		return fmt.Errorf("ReplaceFileW: %w", callErr)
	}
	return nil
}
