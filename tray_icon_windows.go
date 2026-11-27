//go:build tray

package main

import _ "embed"

// Windows tray icons must be .ico.
//
//go:embed assets/tray.ico
var trayIcon []byte
