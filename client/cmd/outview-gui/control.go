//go:build !headless_test

package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/outview/client/internal/client"
	"github.com/outview/client/internal/protocol"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

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
	if !validDeviceCode(code) {
		u.ctrlStatus.SetText("❌ 设备码格式错误，应为6位数字")
		return
	}

	u.connectBtn.Disable()
	u.ctrlStatus.SetText("● 查询设备中...")

	cfg := *u.config
	go func() {
		port, err := queryDevice(&cfg, code)
		if err != nil {
			u.ctrlStatus.SetText("❌ " + err.Error())
			u.connectBtn.Enable()
			return
		}

		target := net.JoinHostPort(cfg.ServerHost, strconv.Itoa(port))
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

func queryDevice(serverConfig *client.Config, code string) (int, error) {
	config := *serverConfig
	cfg := &config
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
		select {
		case resultCh <- resp:
		default:
		}
	}

	// 启动读循环接收响应
	readError := make(chan error, 1)
	go func() { readError <- c.StartReadLoop() }()

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
		if resp.ExternalPort < 1 || resp.ExternalPort > 65535 {
			return 0, fmt.Errorf("服务端返回了无效的外网端口")
		}
		return resp.ExternalPort, nil
	case err := <-readError:
		return 0, fmt.Errorf("查询连接已断开: %v", err)
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
	if runtime.GOOS != "windows" {
		return fmt.Errorf("请使用本机 RDP 客户端连接 %s", target)
	}
	cmd := exec.Command("mstsc", "/v:"+target)
	if err := cmd.Start(); err != nil {
		return err
	}
	// 等待退出以释放进程句柄，远程桌面窗口由用户独立管理。
	go func() { _ = cmd.Wait() }()
	return nil
}

func validDeviceCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
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
