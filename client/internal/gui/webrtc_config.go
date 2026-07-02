package gui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/outview/client/internal/webrtc"
)

// WebRTCConfigTab provides a GUI for editing WebRTC configuration.
type WebRTCConfigTab struct {
	cfg *webrtc.Config

	enableCheck       *widget.Check
	stunEntry         *widget.Entry
	turnEntry         *widget.Entry
	turnUserEntry     *widget.Entry
	turnPassEntry     *widget.Entry
	webrtcTimeoutEntry *widget.Entry
	dtlsTimeoutEntry  *widget.Entry
	idleTimeoutEntry  *widget.Entry
	policySelect      *widget.Select

	tab *container.TabItem
}

// NewWebRTCConfigTab creates a new WebRTC configuration tab.
func NewWebRTCConfigTab(cfg *webrtc.Config) *WebRTCConfigTab {
	w := &WebRTCConfigTab{cfg: cfg}
	w.buildUI()
	w.Load()
	return w
}

func (w *WebRTCConfigTab) buildUI() {
	w.enableCheck = widget.NewCheck("启用 WebRTC", nil)

	w.stunEntry = widget.NewEntry()
	w.stunEntry.SetPlaceHolder("stun:stun.l.google.com:19302, stun:stun.qq.com:3478")

	w.turnEntry = widget.NewEntry()
	w.turnEntry.SetPlaceHolder("turn:turn.example.com:3478")

	w.turnUserEntry = widget.NewEntry()
	w.turnUserEntry.SetPlaceHolder("TURN 用户名（可选）")

	w.turnPassEntry = widget.NewPasswordEntry()
	w.turnPassEntry.SetPlaceHolder("TURN 密码（可选）")

	w.webrtcTimeoutEntry = widget.NewEntry()
	w.webrtcTimeoutEntry.SetPlaceHolder("8")

	w.dtlsTimeoutEntry = widget.NewEntry()
	w.dtlsTimeoutEntry.SetPlaceHolder("10")

	w.idleTimeoutEntry = widget.NewEntry()
	w.idleTimeoutEntry.SetPlaceHolder("60")

	w.policySelect = widget.NewSelect([]string{"all", "relay"}, nil)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "", Widget: w.enableCheck},
			{Text: "STUN 服务器", Widget: w.stunEntry, HintText: "多个用逗号分隔"},
			{Text: "TURN 服务器", Widget: w.turnEntry},
			{Text: "TURN 用户名", Widget: w.turnUserEntry},
			{Text: "TURN 密码", Widget: w.turnPassEntry},
			{Text: "ICE 传输策略", Widget: w.policySelect},
			{Text: "WebRTC 超时（秒）", Widget: w.webrtcTimeoutEntry},
			{Text: "DTLS 超时（秒）", Widget: w.dtlsTimeoutEntry},
			{Text: "空闲超时（秒）", Widget: w.idleTimeoutEntry, HintText: "0 表示禁用"},
		},
		OnSubmit: func() { w.Save() },
		SubmitText: "保存配置",
	}

	content := container.NewVScroll(form)
	w.tab = container.NewTabItem("WebRTC 配置", content)
}

// Tab returns the Fyne TabItem for this configuration panel.
func (w *WebRTCConfigTab) Tab() *container.TabItem {
	return w.tab
}

// Load reads values from the config into the UI fields.
func (w *WebRTCConfigTab) Load() {
	if w.cfg == nil {
		return
	}
	w.enableCheck.SetChecked(w.cfg.EnableWebRTC)

	stunList := make([]string, 0, len(w.cfg.ICEServers))
	var turnURL, turnUser, turnPass string
	for _, s := range w.cfg.ICEServers {
		for _, u := range s.URLs {
			if strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:") {
				if turnURL == "" {
					turnURL = u
					turnUser = s.Username
					if cred, ok := s.Credential.(string); ok {
						turnPass = cred
					}
				}
			} else {
				stunList = append(stunList, u)
			}
		}
	}
	w.stunEntry.SetText(strings.Join(stunList, ", "))
	w.turnEntry.SetText(turnURL)
	w.turnUserEntry.SetText(turnUser)
	w.turnPassEntry.SetText(turnPass)

	policy := w.cfg.ICETransportPolicy
	if policy == "" {
		policy = "all"
	}
	w.policySelect.SetSelected(policy)

	w.webrtcTimeoutEntry.SetText(formatSeconds(w.cfg.WebRTCTimeout))
	w.dtlsTimeoutEntry.SetText(formatSeconds(w.cfg.DTLSTimeout))
	w.idleTimeoutEntry.SetText(formatSeconds(w.cfg.IdleTimeout))
}

// Save writes UI values back to the config.
func (w *WebRTCConfigTab) Save() {
	if w.cfg == nil {
		return
	}
	w.cfg.EnableWebRTC = w.enableCheck.Checked

	servers := make([]pionwebrtc.ICEServer, 0)
	stunText := strings.TrimSpace(w.stunEntry.Text)
	if stunText != "" {
		urls := splitAndTrim(stunText)
		if len(urls) > 0 {
			servers = append(servers, pionwebrtc.ICEServer{URLs: urls})
		}
	}
	turnURL := strings.TrimSpace(w.turnEntry.Text)
	if turnURL != "" {
		s := pionwebrtc.ICEServer{URLs: []string{turnURL}}
		if u := strings.TrimSpace(w.turnUserEntry.Text); u != "" {
			s.Username = u
		}
		if p := strings.TrimSpace(w.turnPassEntry.Text); p != "" {
			s.Credential = p
		}
		servers = append(servers, s)
	}
	w.cfg.ICEServers = servers

	if w.policySelect.Selected != "" {
		w.cfg.ICETransportPolicy = w.policySelect.Selected
	}

	if d, ok := parseSeconds(w.webrtcTimeoutEntry.Text); ok {
		w.cfg.WebRTCTimeout = d
	}
	if d, ok := parseSeconds(w.dtlsTimeoutEntry.Text); ok {
		w.cfg.DTLSTimeout = d
	}
	if d, ok := parseSeconds(w.idleTimeoutEntry.Text); ok {
		w.cfg.IdleTimeout = d
	}
}

// CanvasObject returns the underlying canvas object for embedding.
func (w *WebRTCConfigTab) CanvasObject() fyne.CanvasObject {
	return w.tab.Content
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func formatSeconds(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	return fmt.Sprintf("%d", int(d/time.Second))
}

func parseSeconds(s string) (time.Duration, bool) {
	v := strings.TrimSpace(s)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return time.Duration(n) * time.Second, true
}
