package gui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
)

// TrayManager wraps a Fyne desktop tray menu and icon.
type TrayManager struct {
	mu sync.Mutex

	app    fyne.App
	window fyne.Window

	connected bool
	supported bool
}

// NewTrayManager creates a TrayManager. The system tray is only available on
// desktop platforms; on others the manager becomes a no-op.
func NewTrayManager(app fyne.App, window fyne.Window) *TrayManager {
	t := &TrayManager{app: app, window: window}
	if desk, ok := app.(desktop.App); ok {
		t.supported = true
		t.installMenu(desk)
		t.applyIcon(desk)
	}
	return t
}

func (t *TrayManager) installMenu(desk desktop.App) {
	showItem := fyne.NewMenuItem("显示窗口", func() {
		if t.window != nil {
			t.window.Show()
			t.window.RequestFocus()
		}
	})
	hideItem := fyne.NewMenuItem("隐藏窗口", func() {
		if t.window != nil {
			t.window.Hide()
		}
	})
	quitItem := fyne.NewMenuItem("退出", func() {
		if t.app != nil {
			t.app.Quit()
		}
	})
	menu := fyne.NewMenu("outView", showItem, hideItem, fyne.NewMenuItemSeparator(), quitItem)
	desk.SetSystemTrayMenu(menu)
}

func (t *TrayManager) applyIcon(desk desktop.App) {
	if t.connected {
		desk.SetSystemTrayIcon(theme.ConfirmIcon())
	} else {
		desk.SetSystemTrayIcon(theme.CancelIcon())
	}
}

// SetWebRTCConnected updates the tray icon and tooltip based on connection state.
func (t *TrayManager) SetWebRTCConnected(connected bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = connected
	if !t.supported {
		return
	}
	if desk, ok := t.app.(desktop.App); ok {
		t.applyIcon(desk)
	}
}

// Show makes the main window visible.
func (t *TrayManager) Show() {
	if t.window != nil {
		t.window.Show()
		t.window.RequestFocus()
	}
}

// Hide hides the main window (the tray menu remains available).
func (t *TrayManager) Hide() {
	if t.window != nil {
		t.window.Hide()
	}
}

// Supported reports whether the underlying platform supports a system tray.
func (t *TrayManager) Supported() bool {
	return t.supported
}
