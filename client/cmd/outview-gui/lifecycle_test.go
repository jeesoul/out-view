//go:build ci && !headless_test

package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/outview/client/internal/client"
)

// 原实现仅停止 ticker，等待 ticker.C 的协程永远无法退出。
func TestStopHostStopsStatusUpdates(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	u := &mainUI{app: a, myCode: "123456"}
	u.buildHostTab()
	u.hostClient = client.NewClient(client.DefaultConfig())
	u.startStatusUpdates(u.hostClient)
	done := u.statusDone
	u.stopHostService()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("停止被控服务后，状态更新协程仍未退出")
	}
	// 重复停止不应重复关闭 channel，也不应阻塞。
	u.stopHostService()
}

func TestNewInstallationUsesStableTCPTransport(t *testing.T) {
	if defaultWebRTCSettings().Enabled {
		t.Fatal("尚未接入生产数据链路的 WebRTC 不应默认启用")
	}
}
