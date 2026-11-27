//go:build tray && !windows

package main

import _ "embed"

//go:embed assets/tray.png
var trayIcon []byte
