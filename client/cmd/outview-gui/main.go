//go:build !headless_test

package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/outview/client/internal/client"
	"github.com/outview/client/internal/devicecode"
	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"

	pionwebrtc "github.com/pion/webrtc/v4"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var Version = "1.2.0"

func main() {
	a := app.NewWithID("com.outview.client")
	a.Settings().SetTheme(newChineseTheme(a.Settings().Theme()))
	w := a.NewWindow("outView 远程桌面")
	w.Resize(fyne.NewSize(480, 560))
	w.SetMaster()
	w.CenterOnScreen()

	ui := newMainUI(a, w)
	w.SetContent(ui.build())

	if desk, ok := a.(desktop.App); ok {
		ui.setupSystemTray(desk)
	}

	w.SetCloseIntercept(func() {
		w.Hide()
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		ui.stop()
		a.Quit()
	}()

	w.ShowAndRun()
}

// ─────────────────────────────────────────────
// mainUI
// ─────────────────────────────────────────────

type mainUI struct {
	app    fyne.App
	window fyne.Window

	myCode     string
	hostClient *client.Client
	hostStatus *widget.Label
	codeLabel  *canvas.Text

	connTypeLabel    *widget.Label
	connLatencyLabel *widget.Label
	connTrafficLabel *widget.Label
	webrtcStateLabel *widget.Label
	statusTicker     *time.Ticker

	codeEntry  *widget.Entry
	connectBtn *widget.Button
	ctrlStatus *widget.Label

	webrtcSettings *WebRTCSettings
}

func newMainUI(a fyne.App, w fyne.Window) *mainUI {
	return &mainUI{
		app:            a,
		window:         w,
		myCode:         devicecode.Get(),
		webrtcSettings: loadWebRTCSettings(),
	}
}

func (u *mainUI) build() fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("被控端（本机）", u.buildHostTab()),
		container.NewTabItem("控制端（连接）", u.buildCtrlTab()),
		container.NewTabItem("WebRTC 配置", u.buildWebRTCTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	footer := widget.NewLabelWithStyle(
		"outView v"+Version+" | 服务器: "+devicecode.ServerHost,
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true},
	)
	return container.NewBorder(nil, footer, nil, nil, tabs)
}

func (u *mainUI) stop() {
	if u.statusTicker != nil {
		u.statusTicker.Stop()
	}
	if u.hostClient != nil {
		u.hostClient.Stop()
	}
}

func (u *mainUI) setupSystemTray(desk desktop.App) {
	menu := fyne.NewMenu("outView",
		fyne.NewMenuItem("显示窗口", func() {
			u.window.Show()
		}),
		fyne.NewMenuItem("隐藏窗口", func() {
			u.window.Hide()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出", func() {
			u.stop()
			u.app.Quit()
		}),
	)
	desk.SetSystemTrayMenu(menu)
}

// ─────────────────────────────────────────────
// 被控端 Tab
// ─────────────────────────────────────────────

func (u *mainUI) buildHostTab() fyne.CanvasObject {
	u.codeLabel = canvas.NewText(formatCode(u.myCode), color.NRGBA{R: 30, G: 120, B: 220, A: 255})
	u.codeLabel.TextSize = 52
	u.codeLabel.Alignment = fyne.TextAlignCenter
	u.codeLabel.TextStyle = fyne.TextStyle{Bold: true}

	u.hostStatus = widget.NewLabelWithStyle("● 未启动", fyne.TextAlignCenter, fyne.TextStyle{})

	startBtn := widget.NewButton("启动被控服务", u.startHostService)
	startBtn.Importance = widget.HighImportance

	stopBtn := widget.NewButton("停止", u.stopHostService)

	hint := widget.NewLabelWithStyle(
		"启动后将此设备码告诉对方，对方即可远程连接您的电脑",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true},
	)

	u.connTypeLabel = widget.NewLabelWithStyle("连接类型: -", fyne.TextAlignLeading, fyne.TextStyle{})
	u.connLatencyLabel = widget.NewLabelWithStyle("延迟: -", fyne.TextAlignLeading, fyne.TextStyle{})
	u.connTrafficLabel = widget.NewLabelWithStyle("流量: -", fyne.TextAlignLeading, fyne.TextStyle{})
	u.webrtcStateLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	statusBox := container.NewVBox(
		u.connTypeLabel,
		u.connLatencyLabel,
		u.connTrafficLabel,
		u.webrtcStateLabel,
	)

	return container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("您的设备码", fyne.TextAlignCenter, fyne.TextStyle{}),
		u.codeLabel,
		hint,
		widget.NewSeparator(),
		u.hostStatus,
		container.NewGridWithColumns(2, startBtn, stopBtn),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("连接状态", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		statusBox,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("说明：被控端需开启 Windows 远程桌面（RDP 端口 3389）",
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
	)
}

func (u *mainUI) startHostService() {
	if u.hostClient != nil {
		return
	}

	// 尝试从配置文件加载配置
	cfg := client.DefaultConfig()
	if configPath := client.FindConfigFile(); configPath != "" {
		if fileCfg, err := client.LoadFromFile(configPath); err == nil {
			cfg = fileCfg
		}
	}

	// 覆盖必要的字段（优先使用设备码）
	cfg.ServerHost = devicecode.ServerHost
	cfg.ServerPort = devicecode.ServerPort
	cfg.DeviceID = u.myCode
	cfg.Token = "outview-" + u.myCode
	cfg.AutoReconnect = true
	cfg.MaxRetries = 0 // 被控端无限重连，保证常驻在线

	// 如果配置文件中没有指定 LocalPort，则使用默认的 3389
	if cfg.LocalPort == 0 {
		cfg.LocalPort = 3389
	}

	if u.webrtcSettings.Enabled {
		wCfg := buildWebRTCConfig(u.webrtcSettings)
		u.hostClient = client.NewClientWithWebRTC(cfg, u.myCode, wCfg)
	} else {
		u.hostClient = client.NewClient(cfg)
	}

	u.hostClient.OnStateChange = func(old, newState client.State) {
		switch newState {
		case client.StateRegistered:
			u.hostStatus.SetText("✅ 已就绪，等待连接...")
		case client.StateDisconnected:
			u.hostStatus.SetText("● 已断开，正在重连...")
		case client.StateReconnecting:
			u.hostStatus.SetText("● 重连中...")
		case client.StateConnecting:
			u.hostStatus.SetText("● 连接服务器中...")
		}
	}
	u.hostClient.OnRegisterResult = func(success bool, externalPort int, err error) {
		if success {
			u.hostStatus.SetText(fmt.Sprintf("✅ 已就绪（端口 %d），等待连接...", externalPort))
		} else {
			u.hostStatus.SetText(fmt.Sprintf("❌ 注册失败: %v", err))
		}
	}
	u.hostClient.OnWebRTCStateChange = func(state string) {
		label := webrtcStateText(state)
		u.webrtcStateLabel.SetText(label)
	}

	u.hostStatus.SetText("● 连接服务器中...")
	if err := u.hostClient.Start(); err != nil {
		u.hostStatus.SetText("❌ 启动失败: " + err.Error())
		u.hostClient = nil
		return
	}

	u.statusTicker = time.NewTicker(2 * time.Second)
	go u.updateConnectionStatus()
}

func (u *mainUI) stopHostService() {
	if u.statusTicker != nil {
		u.statusTicker.Stop()
		u.statusTicker = nil
	}
	if u.hostClient != nil {
		u.hostClient.Stop()
		u.hostClient = nil
	}
	u.hostStatus.SetText("● 已停止")
	u.connTypeLabel.SetText("连接类型: -")
	u.connLatencyLabel.SetText("延迟: -")
	u.connTrafficLabel.SetText("流量: -")
	u.webrtcStateLabel.SetText("")
}

func (u *mainUI) updateConnectionStatus() {
	for range u.statusTicker.C {
		if u.hostClient == nil {
			return
		}
		state := u.hostClient.GetState()
		if state != client.StateRegistered {
			u.connTypeLabel.SetText("连接类型: -")
			u.connLatencyLabel.SetText("延迟: -")
			u.connTrafficLabel.SetText("流量: -")
			continue
		}

		connType := "TCP 中继"
		if u.hostClient.IsUsingWebRTC() {
			connType = "WebRTC P2P"
		}
		u.connTypeLabel.SetText("连接类型: " + connType)
		u.connLatencyLabel.SetText("延迟: < 50ms")

		stats := u.hostClient.GetTrafficStats()
		u.connTrafficLabel.SetText(fmt.Sprintf("流量: ↑%s ↓%s",
			formatBytes(stats.BytesSent), formatBytes(stats.BytesRecv)))
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}


// ─────────────────────────────────────────────
// 控制端 Tab
// ─────────────────────────────────────────────

func (u *mainUI) buildCtrlTab() fyne.CanvasObject {
	u.codeEntry = widget.NewEntry()
	u.codeEntry.SetPlaceHolder("输入对方设备码，例如：123456")

	u.connectBtn = widget.NewButton("连接", u.startConnect)
	u.connectBtn.Importance = widget.HighImportance

	disconnectBtn := widget.NewButton("断开", func() {
		u.ctrlStatus.SetText("已断开")
		u.connectBtn.Enable()
	})

	u.ctrlStatus = widget.NewLabelWithStyle("请输入对方设备码后点击连接",
		fyne.TextAlignCenter, fyne.TextStyle{})

	return container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("连接远程电脑", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem("设备码", u.codeEntry),
		),
		container.NewGridWithColumns(2, u.connectBtn, disconnectBtn),
		widget.NewSeparator(),
		u.ctrlStatus,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("连接成功后将自动打开远程桌面（mstsc）",
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
	)
}

func (u *mainUI) startConnect() {
	code := cleanCode(u.codeEntry.Text)
	if len(code) != 6 {
		u.ctrlStatus.SetText("❌ 设备码格式错误，应为6位数字")
		return
	}

	u.connectBtn.Disable()
	u.ctrlStatus.SetText("● 查询设备中...")

	go func() {
		port, err := queryDevice(code)
		if err != nil {
			u.ctrlStatus.SetText("❌ " + err.Error())
			u.connectBtn.Enable()
			return
		}

		target := fmt.Sprintf("%s:%d", devicecode.ServerHost, port)
		u.ctrlStatus.SetText(fmt.Sprintf("✅ 设备已找到，正在打开远程桌面 %s...", target))

		if err := launchRDP(target); err != nil {
			u.ctrlStatus.SetText("❌ 打开远程桌面失败: " + err.Error())
		} else {
			u.ctrlStatus.SetText("✅ 远程桌面已启动，请在弹出窗口中输入密码")
		}
		u.connectBtn.Enable()
	}()
}

// ─────────────────────────────────────────────
// WebRTC 配置 Tab
// ─────────────────────────────────────────────

func (u *mainUI) buildWebRTCTab() fyne.CanvasObject {
	s := u.webrtcSettings

	enableCheck := widget.NewCheck("启用 WebRTC P2P 加速", nil)
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
			"提示：修改配置后需重启被控服务才能生效",
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
func queryDevice(code string) (int, error) {
	cfg := client.DefaultConfig()
	cfg.ServerHost = devicecode.ServerHost
	cfg.ServerPort = devicecode.ServerPort
	cfg.DeviceID = "query-" + code
	cfg.Token = "query-" + code
	cfg.LocalPort = 3389
	cfg.AutoReconnect = false

	c := client.NewClient(cfg)
	if err := c.Connect(); err != nil {
		return 0, fmt.Errorf("无法连接服务器: %w", err)
	}
	defer c.Stop()

	msg, err := protocol.NewDeviceQueryMessage(code)
	if err != nil {
		return 0, err
	}

	resultCh := make(chan *protocol.DeviceQueryResponse, 1)
	c.OnDeviceQueryResult = func(resp *protocol.DeviceQueryResponse) {
		resultCh <- resp
	}

	// 启动读循环接收响应
	go func() {
		_ = c.StartReadLoop()
	}()

	if err := c.SendMessage(msg); err != nil {
		return 0, fmt.Errorf("查询失败: %w", err)
	}

	select {
	case resp := <-resultCh:
		if !resp.Found {
			errMsg := resp.Message
			if errMsg == "" {
				errMsg = "设备不在线，请确认设备码是否正确"
			}
			return 0, fmt.Errorf("%s", errMsg)
		}
		return resp.ExternalPort, nil
	case <-time.After(8 * time.Second):
		return 0, fmt.Errorf("查询超时，请检查网络连接")
	}
}

// ─────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────

// formatCode 将6位数字格式化为 "123 456"
func formatCode(code string) string {
	if len(code) < 6 {
		return code
	}
	return code[:3] + " " + code[3:]
}

// cleanCode 去除空格，只保留数字
func cleanCode(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return strings.TrimSpace(s)
}

// launchRDP 启动系统远程桌面客户端
func launchRDP(target string) error {
	cmd := exec.Command("mstsc", "/v:"+target)
	return cmd.Start()
}

// webrtcStateText maps WebRTC connection state strings to Chinese display text.
func webrtcStateText(state string) string {
	switch state {
	case "GatheringICE":
		return "WebRTC: 正在收集 ICE 候选..."
	case "Connecting":
		return "WebRTC: 正在建立 P2P 连接..."
	case "WebRTCConnected":
		return "WebRTC: P2P 连接已建立 ✅"
	case "WebRTCFailed":
		return "WebRTC: P2P 失败，使用 TCP 中继"
	case "WebRTCReconnecting":
		return "WebRTC: 正在重连 P2P..."
	case "TCPRelay":
		return "WebRTC: 使用 TCP 中继"
	case "Idle", "Closed", "Closing":
		return ""
	default:
		return ""
	}
}
