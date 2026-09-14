//go:build ci && !headless_test

package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/outview/client/internal/client"
	"github.com/outview/client/internal/protocol"
)

// 使用真实 GUI 配置解析和 Client.Start，防止配置中的自建服务器被内置地址覆盖。
func TestHostUsesConfiguredEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte("host=127.0.0.1\nport="+strconv.Itoa(port)+"\nlocal-port=3390\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	request := make(chan protocol.RegisterRequest, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		message, err := protocol.NewDecoder(conn).Decode()
		if err != nil {
			return
		}
		var body protocol.RegisterRequest
		if json.Unmarshal(message.Body, &body) == nil {
			request <- body
		}
	}()
	a := test.NewApp()
	defer a.Quit()
	u := &mainUI{app: a, config: cfg, myCode: "123456", webrtcSettings: defaultWebRTCSettings()}
	u.buildHostTab()
	u.startHostService()
	defer u.stopHostService()
	select {
	case got := <-request:
		if got.DeviceID != "123456" || got.LocalPort != 3390 {
			t.Fatalf("错误注册参数: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("被控服务没有连接配置文件中的服务器")
	}
}

func TestDesktopConfigKeepsExplicitLocalhost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte("host=localhost\nport=17001\nlocal-port=3390\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadDesktopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerHost != "localhost" || cfg.ServerPort != 17001 || cfg.LocalPort != 3390 {
		t.Fatalf("配置未保留: %+v", cfg)
	}
}

func TestQueryUsesConfiguredEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		msg, err := protocol.NewDecoder(conn).Decode()
		if err != nil || msg.Header.Type != protocol.TypeDeviceQuery {
			return
		}
		body := []byte(`{"found":true,"deviceCode":"123456","externalPort":6123}`)
		_ = protocol.NewEncoder(conn).Encode(&protocol.Message{Header: &protocol.MessageHeader{Magic: protocol.MagicNumber, Version: 1, Type: protocol.TypeDeviceQueryAck, Length: int32(len(body))}, Body: body})
		<-done
	}()
	cfg := client.DefaultConfig()
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = listener.Addr().(*net.TCPAddr).Port
	port, err := queryDevice(cfg, "123456")
	if err != nil || port != 6123 {
		t.Fatalf("query port=%d err=%v", port, err)
	}
}
