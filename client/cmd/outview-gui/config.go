//go:build !headless_test

package main

import (
	"github.com/outview/client/internal/client"
	"github.com/outview/client/internal/devicecode"
)

// GUI 与 CLI 共用配置解析；仅 GUI 的服务器缺省值保持旧版兼容。
func loadDesktopConfig(path string) (*client.Config, error) {
	cfg := client.DefaultConfig()
	cfg.ServerHost = devicecode.ServerHost
	cfg.ServerPort = devicecode.ServerPort
	if path == "" {
		path = client.FindConfigFile()
	}
	if path == "" {
		return cfg, nil
	}
	return client.LoadFromFileWithDefaults(path, cfg)
}
