package gui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ICEPhase represents a phase in the ICE/DTLS negotiation lifecycle.
type ICEPhase int

const (
	ICEPhaseIdle ICEPhase = iota
	ICEPhaseGathering
	ICEPhaseTesting
	ICEPhaseDTLS
	ICEPhaseConnected
	ICEPhaseFailed
)

// ICEProgressWidget displays a progress bar and label for the ICE handshake.
type ICEProgressWidget struct {
	mu sync.Mutex

	bar       *widget.ProgressBar
	label     *canvas.Text
	root      fyne.CanvasObject
	current   ICEPhase
}

// NewICEProgressWidget builds a new progress widget in the idle state.
func NewICEProgressWidget() *ICEProgressWidget {
	w := &ICEProgressWidget{}
	w.bar = widget.NewProgressBar()
	w.bar.Min = 0
	w.bar.Max = 1
	w.bar.SetValue(0)
	w.label = canvas.NewText(phaseLabel(ICEPhaseIdle), colorGray())
	w.label.TextStyle = fyne.TextStyle{Bold: true}
	w.root = container.NewVBox(w.label, w.bar)
	return w
}

// Widget returns the renderable canvas object for embedding.
func (w *ICEProgressWidget) Widget() fyne.CanvasObject {
	return w.root
}

// SetPhase updates the progress bar and label according to the supplied phase.
func (w *ICEProgressWidget) SetPhase(phase ICEPhase) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current = phase
	value := phaseValue(phase)
	text := phaseLabel(phase)
	c := phaseColor(phase)
	w.bar.SetValue(value)
	w.label.Text = text
	w.label.Color = c
	canvas.Refresh(w.label)
}

// Phase reports the most recently set phase.
func (w *ICEProgressWidget) Phase() ICEPhase {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

func phaseValue(p ICEPhase) float64 {
	switch p {
	case ICEPhaseGathering:
		return 0.25
	case ICEPhaseTesting:
		return 0.5
	case ICEPhaseDTLS:
		return 0.75
	case ICEPhaseConnected:
		return 1.0
	case ICEPhaseFailed, ICEPhaseIdle:
		return 0
	default:
		return 0
	}
}

func phaseLabel(p ICEPhase) string {
	switch p {
	case ICEPhaseGathering:
		return "ICE 候选收集中..."
	case ICEPhaseTesting:
		return "ICE 连通性测试中..."
	case ICEPhaseDTLS:
		return "DTLS 握手中..."
	case ICEPhaseConnected:
		return "WebRTC 已建立"
	case ICEPhaseFailed:
		return "协商失败"
	case ICEPhaseIdle:
		return "等待协商"
	default:
		return "未知阶段"
	}
}

func phaseColor(p ICEPhase) color.Color {
	switch p {
	case ICEPhaseConnected:
		return colorGreen()
	case ICEPhaseFailed:
		return colorRed()
	case ICEPhaseGathering, ICEPhaseTesting, ICEPhaseDTLS:
		return colorYellow()
	default:
		return colorGray()
	}
}
