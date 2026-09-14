//go:build !headless_test

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/outview/client/internal/client"
	"github.com/outview/client/internal/devicecode"
)

var Version = "1.2.1"
var BuildDate = "unknown"

func main() {
	configPath := flag.String("config", "", "客户端配置文件，默认自动查找 config.txt")
	host := flag.String("host", "", "服务器地址，覆盖配置文件")
	port := flag.Int("port", 0, "服务器控制端口，覆盖配置文件")
	autoStart := flag.Bool("auto-start", false, "启动程序时同时启动被控服务")
	version := flag.Bool("version", false, "显示版本")
	flag.Parse()
	if *version {
		fmt.Printf("outView GUI v%s (built: %s)\n", Version, BuildDate)
		return
	}
	cfg, err := loadDesktopConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *host != "" {
		cfg.ServerHost = *host
	}
	if *port != 0 {
		cfg.ServerPort = *port
	}
	a := app.NewWithID("com.outview.client")
	a.Settings().SetTheme(newChineseTheme(a.Settings().Theme()))
	w := a.NewWindow("outView 远程桌面")
	w.Resize(fyne.NewSize(480, 560))
	w.SetMaster()
	w.CenterOnScreen()
	u := newMainUI(a, w, cfg)
	w.SetContent(u.build())
	if desk, ok := a.(desktop.App); ok {
		u.setupSystemTray(desk)
	}
	w.SetCloseIntercept(w.Hide)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigChan:
			u.stopHostService()
			a.Quit()
		case <-done:
		}
	}()
	if *autoStart {
		u.startHostService()
	}
	w.ShowAndRun()
	close(done)
	u.stopHostService()
}

// mainUI 保存界面与被控服务生命周期，页面及查询逻辑按职责拆分。
type mainUI struct {
	app              fyne.App
	window           fyne.Window
	config           *client.Config
	myCode           string
	hostMu           sync.Mutex
	hostClient       *client.Client
	hostActive       *atomic.Bool
	statusCancel     context.CancelFunc
	statusDone       chan struct{}
	hostStatus       *widget.Label
	codeLabel        *canvas.Text
	connTypeLabel    *widget.Label
	connLatencyLabel *widget.Label
	connTrafficLabel *widget.Label
	webrtcStateLabel *widget.Label
	codeEntry        *widget.Entry
	connectBtn       *widget.Button
	ctrlStatus       *widget.Label
	webrtcSettings   *WebRTCSettings
}

func newMainUI(a fyne.App, w fyne.Window, cfg *client.Config) *mainUI {
	return &mainUI{app: a, window: w, config: cfg, myCode: devicecode.Get(), webrtcSettings: loadWebRTCSettings()}
}

func (u *mainUI) build() fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("被控端（本机）", u.buildHostTab()),
		container.NewTabItem("控制端（连接）", u.buildCtrlTab()),
		container.NewTabItem("WebRTC 实验配置", u.buildWebRTCTab()),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	footer := widget.NewLabelWithStyle("outView v"+Version+" | 服务器: "+u.config.ServerAddr(),
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	return container.NewBorder(nil, footer, nil, nil, tabs)
}

func (u *mainUI) setupSystemTray(desk desktop.App) {
	desk.SetSystemTrayMenu(fyne.NewMenu("outView",
		fyne.NewMenuItem("显示窗口", u.window.Show),
		fyne.NewMenuItem("隐藏窗口", u.window.Hide),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出", func() { u.stopHostService(); u.app.Quit() }),
	))
}
