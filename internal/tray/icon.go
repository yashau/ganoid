package tray

import _ "embed"

// icon holds the raw ICO bytes for the system tray icon.
// The file must be placed at internal/tray/icon.ico.
// The same icon is referenced by cmd/*/versioninfo.json for the .exe resource.
//
//go:embed icon.ico
var icon []byte

// Icon returns the raw ICO bytes to pass to systray.SetIcon.
func Icon() []byte { return icon }
