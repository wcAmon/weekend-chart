package tray

import (
	"github.com/getlantern/systray"
)

var (
	onExit   func()
	onReady  func()
	mQuit    *systray.MenuItem
	mStatus  *systray.MenuItem
)

// Icon is a simple 16x16 ICO format icon (green circle)
var iconData = []byte{
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00,
	0x20, 0x00, 0x68, 0x04, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00, 0x28, 0x00,
	0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x01, 0x00,
	0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00,
}

// Start starts the system tray
func Start(readyFunc, exitFunc func()) {
	onReady = readyFunc
	onExit = exitFunc
	systray.Run(onReadyInternal, onExitInternal)
}

// Quit exits the system tray
func Quit() {
	systray.Quit()
}

// SetStatus updates the status text in the tray menu
func SetStatus(status string) {
	if mStatus != nil {
		mStatus.SetTitle(status)
	}
}

func onReadyInternal() {
	systray.SetIcon(iconData)
	systray.SetTitle("Weekend Chart")
	systray.SetTooltip("Weekend Chart Agent")

	mStatus = systray.AddMenuItem("狀態: 啟動中...", "當前狀態")
	mStatus.Disable()

	systray.AddSeparator()

	mQuit = systray.AddMenuItem("結束", "關閉 Agent")

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()

	if onReady != nil {
		onReady()
	}
}

func onExitInternal() {
	if onExit != nil {
		onExit()
	}
}
