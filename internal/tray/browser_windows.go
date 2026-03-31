//go:build windows

package tray

import (
	"golang.org/x/sys/windows"
)

// OpenBrowser opens url in the default browser using ShellExecuteW.
func OpenBrowser(url string) {
	urlPtr, _ := windows.UTF16PtrFromString(url)
	verbPtr, _ := windows.UTF16PtrFromString("open")
	_ = windows.ShellExecute(0, verbPtr, urlPtr, nil, nil, windows.SW_SHOWNORMAL)
}
