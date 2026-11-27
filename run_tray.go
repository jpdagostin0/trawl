//go:build tray

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/systray"
)

// run shows a tray icon with the server status, Reload and Quit.
func run(a *app) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		systray.Quit()
	}()
	systray.Run(func() { trayReady(a) }, nil)
}

func trayReady(a *app) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("trawl")
	systray.SetTooltip("trawl: running on port " + a.port())

	status := systray.AddMenuItem("trawl is running", "")
	status.Disable()
	port := systray.AddMenuItem("Port "+a.port(), "")
	port.Disable()
	systray.AddSeparator()
	summaries := systray.AddMenuItemCheckbox("Local search summaries", "Summarize the top results with the local Ollama model", a.summary != nil && a.summary.Enabled.Load())
	if a.summary == nil {
		summaries.Disable()
	}
	reload := systray.AddMenuItem("Reload", "Restart the server (clears caches and paused engines)")
	quit := systray.AddMenuItem("Quit", "Stop trawl")

	go func() {
		for {
			select {
			case <-summaries.ClickedCh:
				if a.summary != nil {
					enabled := !a.summary.Enabled.Load()
					a.summary.Enabled.Store(enabled)
					if enabled {
						summaries.Check()
					} else {
						summaries.Uncheck()
					}
				}
			case <-reload.ClickedCh:
				if err := a.reload(); err != nil {
					log.Printf("reload: %v", err)
					status.SetTitle("trawl is stopped: " + err.Error())
					systray.SetTooltip("trawl: stopped")
				} else {
					status.SetTitle("trawl is running")
					systray.SetTooltip("trawl: running on port " + a.port())
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
