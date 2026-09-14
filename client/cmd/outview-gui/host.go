//go:build !headless_test

package main

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/outview/client/internal/client"
	"image/color"
	"sync/atomic"
	"time"
)

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
	u.hostMu.Lock()
	defer u.hostMu.Unlock()
	if u.hostClient != nil {
		return
	}
	cfg := *u.config
	cfg.DeviceID = u.myCode
	cfg.Token = "outview-" + u.myCode
	cfg.AutoReconnect = true
	cfg.MaxRetries = 0
	if err := cfg.Validate(); err != nil {
		u.hostStatus.SetText("配置错误: " + err.Error())
		return
	}
	c := client.NewClient(&cfg)
	if u.webrtcSettings.Enabled {
		c = client.NewClientWithWebRTC(&cfg, u.myCode, buildWebRTCConfig(u.webrtcSettings))
	}
	active := &atomic.Bool{}
	active.Store(true)
	u.hostActive = active
	c.OnStateChange = func(old, state client.State) {
		if !active.Load() {
			return
		}
		switch state {
		case client.StateRegistered:
			u.hostStatus.SetText("已就绪，等待连接...")
		case client.StateDisconnected:
			u.hostStatus.SetText("连接已断开，等待恢复...")
		case client.StateReconnecting:
			u.hostStatus.SetText("正在重连服务器...")
		case client.StateConnecting:
			u.hostStatus.SetText("正在连接服务器...")
		}
	}
	c.OnRegisterResult = func(success bool, port int, err error) {
		if !active.Load() {
			return
		}
		if success {
			u.hostStatus.SetText(fmt.Sprintf("已就绪（固定外网端口 %d）", port))
		} else {
			u.hostStatus.SetText(fmt.Sprintf("注册失败: %v", err))
		}
	}
	c.OnError = func(err error) {
		if active.Load() && c.GetState() != client.StateRegistered {
			u.hostStatus.SetText("连接异常，等待重试: " + err.Error())
		}
	}
	c.OnWebRTCStateChange = func(state string) {
		if active.Load() {
			u.webrtcStateLabel.SetText(webrtcStateText(state))
		}
	}
	u.hostClient = c
	u.hostStatus.SetText("正在连接服务器...")
	if err := c.Start(); err != nil {
		active.Store(false)
		c.Stop()
		u.hostClient = nil
		u.hostStatus.SetText("启动失败: " + err.Error())
		return
	}
	u.startStatusUpdates(c)
}

// 创建时捕获本轮客户端，取消时等待退出，避免旧循环刷新新一轮服务。
func (u *mainUI) startStatusUpdates(c *client.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	u.statusCancel = cancel
	u.statusDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				u.updateConnectionStatus(c)
			}
		}
	}()
}

func (u *mainUI) stopHostService() {
	u.hostMu.Lock()
	defer u.hostMu.Unlock()
	if u.hostActive != nil {
		u.hostActive.Store(false)
	}
	if u.statusCancel != nil {
		u.statusCancel()
		<-u.statusDone
		u.statusCancel = nil
		u.statusDone = nil
	}
	if u.hostClient != nil {
		u.hostClient.Stop()
		u.hostClient = nil
	}
	if u.hostStatus != nil {
		u.hostStatus.SetText("已停止")
		u.connTypeLabel.SetText("连接类型: -")
		u.connLatencyLabel.SetText("心跳往返: -")
		u.connTrafficLabel.SetText("流量: -")
		u.webrtcStateLabel.SetText("")
	}
}

func (u *mainUI) updateConnectionStatus(c *client.Client) {
	if c.GetState() != client.StateRegistered {
		u.connTypeLabel.SetText("连接类型: -")
		u.connLatencyLabel.SetText("心跳往返: -")
		u.connTrafficLabel.SetText("流量: -")
		return
	}
	u.connTypeLabel.SetText("连接类型: TCP 中继")
	if latency, valid := c.GetLatency(); valid {
		u.connLatencyLabel.SetText(fmt.Sprintf("心跳往返: %d ms", latency.Milliseconds()))
	} else {
		u.connLatencyLabel.SetText("心跳往返: 等待采样")
	}
	stats := c.GetTrafficStats()
	u.connTrafficLabel.SetText(fmt.Sprintf("流量: ↑%s ↓%s",
		formatBytes(stats.BytesSent), formatBytes(stats.BytesRecv)))
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
