package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/outview/client/internal/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

var (
	Version   = "1.0.2-SNAPSHOT"
	BuildDate = "unknown"
)

// GUIApp represents the GUI application
type GUIApp struct {
	app     fyne.App
	window  fyne.Window
	client  *client.Client
	config  *client.Config

	// UI components
	hostEntry      *widget.Entry
	portEntry      *widget.Entry
	deviceIDEntry  *widget.Entry
	tokenEntry     *widget.PasswordEntry
	localPortEntry *widget.Entry

	connectBtn     *widget.Button
	disconnectBtn  *widget.Button
	statusLabel    *widget.Label
	logText        *widget.Entry

	// State
	isConnected binding.Bool
}

func main() {
	gui := &GUIApp{
		app: app.NewWithID("com.outview.client"),
	}
	gui.app.SetIcon(nil) // TODO: Add icon

	gui.createUI()
	gui.window.ShowAndRun()
}

func (g *GUIApp) createUI() {
	g.window = g.app.NewWindow("outView Client")
	g.window.Resize(fyne.NewSize(400, 500))
	g.window.SetMaster()

	// Initialize bindings
	g.isConnected = binding.NewBool()
	g.isConnected.Set(false)

	// Create form
	g.hostEntry = widget.NewEntry()
	g.hostEntry.SetPlaceHolder("例如: 192.168.1.100")
	g.hostEntry.Bind(binding.BindString(&g.config.ServerHost))

	g.portEntry = widget.NewEntry()
	g.portEntry.SetText("7000")
	g.portEntry.SetPlaceHolder("默认: 7000")

	g.deviceIDEntry = widget.NewEntry()
	g.deviceIDEntry.SetPlaceHolder("输入设备ID")

	g.tokenEntry = widget.NewPasswordEntry()
	g.tokenEntry.SetPlaceHolder("输入Token密钥")

	g.localPortEntry = widget.NewEntry()
	g.localPortEntry.SetText("3389")
	g.localPortEntry.SetPlaceHolder("默认: 3389 (RDP)")

	// Status
	g.statusLabel = widget.NewLabel("状态: 未连接")
	g.statusLabel.Alignment = fyne.TextAlignCenter

	// Buttons
	g.connectBtn = widget.NewButton("连接", g.onConnect)
	g.connectBtn.Importance = widget.HighImportance

	g.disconnectBtn = widget.NewButton("断开", g.onDisconnect)
	g.disconnectBtn.Disable()

	// Log area
	g.logText = widget.NewMultiLineEntry()
	g.logText.Disable()
	g.logText.SetPlaceHolder("连接日志将显示在这里...")

	// Form layout
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "服务器地址:", Widget: g.hostEntry},
			{Text: "端口:", Widget: g.portEntry},
			{Text: "设备ID:", Widget: g.deviceIDEntry},
			{Text: "Token:", Widget: g.tokenEntry},
			{Text: "本地端口:", Widget: g.localPortEntry},
		},
	}

	// Button container
	buttons := container.NewHBox(
		g.connectBtn,
		g.disconnectBtn,
	)

	// Main content
	content := container.NewVBox(
		form,
		widget.NewSeparator(),
		g.statusLabel,
		buttons,
		widget.NewSeparator(),
		widget.NewLabel("日志:"),
		container.NewMax(g.logText),
	)

	g.window.SetContent(container.NewPadded(content))
	g.window.CenterOnScreen()

	// Handle window close
	g.window.SetCloseIntercept(func() {
		if g.client != nil {
			g.client.Stop()
		}
		g.window.Close()
	})
}

func (g *GUIApp) onConnect() {
	// Validate inputs
	host := g.hostEntry.Text
	if host == "" {
		g.showError("请输入服务器地址")
		return
	}

	deviceID := g.deviceIDEntry.Text
	if deviceID == "" {
		g.showError("请输入设备ID")
		return
	}

	token := g.tokenEntry.Text
	if token == "" {
		g.showError("请输入Token")
		return
	}

	port := 7000
	fmt.Sscanf(g.portEntry.Text, "%d", &port)

	localPort := 3389
	fmt.Sscanf(g.localPortEntry.Text, "%d", &localPort)

	// Create config
	config := client.DefaultConfig()
	config.ServerHost = host
	config.ServerPort = port
	config.DeviceID = deviceID
	config.Token = token
	config.LocalPort = localPort

	// Create client
	g.client = client.NewClient(config)

	// Set callbacks
	g.client.OnStateChange = func(old, new client.State) {
		g.log(fmt.Sprintf("状态变更: %s -> %s", old, new))
		g.updateStatus(new)
	}

	g.client.OnRegisterResult = func(success bool, externalPort int, err error) {
		if success {
			g.log(fmt.Sprintf("✅ 注册成功! 外部端口: %d", externalPort))
			g.log(fmt.Sprintf("💡 现在可以通过 %s:%d 进行RDP连接", host, externalPort))
		} else {
			g.log(fmt.Sprintf("❌ 注册失败: %v", err))
		}
	}

	g.client.OnError = func(err error) {
		g.log(fmt.Sprintf("❌ 错误: %v", err))
	}

	// Start connection
	g.log("正在连接服务器...")
	if err := g.client.Start(); err != nil {
		g.showError(fmt.Sprintf("连接失败: %v", err))
		return
	}

	g.connectBtn.Disable()
	g.disconnectBtn.Enable()
	g.isConnected.Set(true)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		g.onDisconnect()
	}()
}

func (g *GUIApp) onDisconnect() {
	if g.client != nil {
		g.client.Stop()
		g.client = nil
	}

	g.connectBtn.Enable()
	g.disconnectBtn.Disable()
	g.isConnected.Set(false)
	g.statusLabel.SetText("状态: 未连接")
	g.log("已断开连接")
}

func (g *GUIApp) log(message string) {
	g.logText.SetText(g.logText.Text + message + "\n")
}

func (g *GUIApp) showError(message string) {
	dialog := widget.NewLabel(message)
	dialog.Alignment = fyne.TextAlignCenter
	w := g.app.NewWindow("错误")
	w.SetContent(container.NewPadded(dialog))
	w.Resize(fyne.NewSize(300, 100))
	w.Show()
}

func (g *GUIApp) updateStatus(state client.State) {
	switch state {
	case client.StateDisconnected:
		g.statusLabel.SetText("状态: 未连接")
	case client.StateConnecting:
		g.statusLabel.SetText("状态: 连接中...")
	case client.StateConnected:
		g.statusLabel.SetText("状态: 已连接")
	case client.StateRegistered:
		g.statusLabel.SetText("状态: 已注册")
	}
}