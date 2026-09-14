//go:build !headless_test

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	clientwebrtc "github.com/outview/client/internal/webrtc"
	pionwebrtc "github.com/pion/webrtc/v4"
	"strings"
)

func (u *mainUI) buildWebRTCTab() fyne.CanvasObject {
	s := u.webrtcSettings

	enableCheck := widget.NewCheck("启用实验性 WebRTC 信令", nil)
	enableCheck.SetChecked(s.Enabled)

	stunEntry := widget.NewMultiLineEntry()
	stunEntry.SetText(s.STUNServers)
	stunEntry.SetMinRowsVisible(3)
	stunEntry.SetPlaceHolder("每行一个 STUN 服务器")

	turnEntry := widget.NewEntry()
	turnEntry.SetText(s.TURNServer)
	turnEntry.SetPlaceHolder("turn:server.com:3478")

	turnUserEntry := widget.NewEntry()
	turnUserEntry.SetText(s.TURNUsername)
	turnUserEntry.SetPlaceHolder("TURN 用户名")

	turnPassEntry := widget.NewPasswordEntry()
	turnPassEntry.SetText(s.TURNPassword)
	turnPassEntry.SetPlaceHolder("TURN 密码")

	policySelect := widget.NewSelect([]string{"all", "relay"}, nil)
	policySelect.SetSelected(s.TransportPolicy)

	saveStatus := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})

	saveBtn := widget.NewButton("保存配置", func() {
		updated := &WebRTCSettings{
			Enabled:         enableCheck.Checked,
			STUNServers:     stunEntry.Text,
			TURNServer:      strings.TrimSpace(turnEntry.Text),
			TURNUsername:    strings.TrimSpace(turnUserEntry.Text),
			TURNPassword:    turnPassEntry.Text,
			TransportPolicy: policySelect.Selected,
		}
		if err := saveWebRTCSettings(updated); err != nil {
			saveStatus.SetText("❌ 保存失败: " + err.Error())
			return
		}
		u.webrtcSettings = updated
		saveStatus.SetText("✅ 已保存（重启被控服务后生效）")
	})
	saveBtn.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("STUN 服务器", stunEntry),
		widget.NewFormItem("TURN 服务器", turnEntry),
		widget.NewFormItem("TURN 用户名", turnUserEntry),
		widget.NewFormItem("TURN 密码", turnPassEntry),
		widget.NewFormItem("传输策略", policySelect),
	)

	return container.NewVBox(
		widget.NewSeparator(),
		enableCheck,
		widget.NewSeparator(),
		form,
		widget.NewSeparator(),
		saveBtn,
		saveStatus,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(
			"实验功能：当前发布版数据仍经 TCP 转发；修改后重启被控服务生效。",
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true},
		),
	)
}

func buildWebRTCConfig(s *WebRTCSettings) *clientwebrtc.Config {
	cfg := clientwebrtc.DefaultConfig()
	cfg.EnableWebRTC = s.Enabled
	cfg.ICETransportPolicy = s.TransportPolicy

	var iceServers []pionwebrtc.ICEServer
	for _, line := range strings.Split(s.STUNServers, "\n") {
		url := strings.TrimSpace(line)
		if url != "" {
			iceServers = append(iceServers, pionwebrtc.ICEServer{URLs: []string{url}})
		}
	}
	if s.TURNServer != "" {
		turn := pionwebrtc.ICEServer{URLs: []string{s.TURNServer}}
		if s.TURNUsername != "" {
			turn.Username = s.TURNUsername
			turn.Credential = s.TURNPassword
		}
		iceServers = append(iceServers, turn)
	}
	if len(iceServers) > 0 {
		cfg.ICEServers = iceServers
	}
	return cfg
}

// queryDevice 连接服务器查询设备码对应的外部端口
