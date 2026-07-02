package gui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/outview/client/internal/webrtc"
)

// ConnectionStatusWidget shows the current WebRTC and TCP connection state.
type ConnectionStatusWidget struct {
	mu sync.Mutex

	webrtcLabel *canvas.Text
	tcpLabel    *canvas.Text
	root        *fyne.Container
}

// NewConnectionStatusWidget builds a new status widget with default values.
func NewConnectionStatusWidget() *ConnectionStatusWidget {
	w := &ConnectionStatusWidget{}
	w.webrtcLabel = canvas.NewText("WebRTC 未连接", colorGray())
	w.webrtcLabel.TextStyle = fyne.TextStyle{Bold: true}
	w.tcpLabel = canvas.NewText("TCP 未连接", colorGray())
	w.tcpLabel.TextStyle = fyne.TextStyle{Bold: true}
	w.root = container.NewHBox(w.webrtcLabel, w.tcpLabel)
	return w
}

// Widget returns the canvas object to embed in a parent container.
func (w *ConnectionStatusWidget) Widget() fyne.CanvasObject {
	return w.root
}

// UpdateWebRTC updates the WebRTC portion of the status widget.
func (w *ConnectionStatusWidget) UpdateWebRTC(state webrtc.ConnectionState) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var text string
	var c color.Color

	switch state {
	case webrtc.StateWebRTCConnected:
		text = "WebRTC ✓"
		c = colorGreen()
	case webrtc.StateWebRTCReconnecting:
		text = "WebRTC 重连中..."
		c = colorYellow()
	case webrtc.StateWebRTCFailed, webrtc.StateTCPRelay:
		text = "TCP 降级"
		c = colorOrange()
	case webrtc.StateClosing, webrtc.StateClosed:
		text = "已断开"
		c = colorRed()
	default:
		text = "连接中..."
		c = colorGray()
	}

	w.webrtcLabel.Text = text
	w.webrtcLabel.Color = c
	canvas.Refresh(w.webrtcLabel)
}

// UpdateTCP updates the TCP-side status indicator.
func (w *ConnectionStatusWidget) UpdateTCP(connected bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if connected {
		w.tcpLabel.Text = "TCP ✓"
		w.tcpLabel.Color = colorGreen()
	} else {
		w.tcpLabel.Text = "TCP 未连接"
		w.tcpLabel.Color = colorGray()
	}
	canvas.Refresh(w.tcpLabel)
}

func colorGreen() color.Color  { return color.NRGBA{R: 0x2e, G: 0xa0, B: 0x44, A: 0xff} }
func colorYellow() color.Color { return color.NRGBA{R: 0xd8, G: 0xa1, B: 0x17, A: 0xff} }
func colorOrange() color.Color { return color.NRGBA{R: 0xe0, G: 0x6c, B: 0x1c, A: 0xff} }
func colorRed() color.Color    { return color.NRGBA{R: 0xc4, G: 0x2b, B: 0x1f, A: 0xff} }
func colorGray() color.Color   { return color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff} }
