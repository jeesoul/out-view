package devicecode

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// ServerHost 硬编码的公网服务器地址
	ServerHost = "120.27.214.55"
	ServerPort = 7000
)

type deviceInfo struct {
	Code string `json:"code"`
}

// Get 返回本机设备码（6位数字字符串），首次调用时生成并持久化
func Get() string {
	// 先尝试从本地文件读取
	if code := loadSaved(); code != "" {
		return code
	}
	// 生成新设备码
	code := generate()
	save(code)
	return code
}

// generate 基于硬件信息生成6位数字设备码
func generate() string {
	h := sha256.New()
	// MAC 地址
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) > 0 {
				h.Write(iface.HardwareAddr)
				break
			}
		}
	}
	// 主机名
	if hostname, err := os.Hostname(); err == nil {
		h.Write([]byte(hostname))
	}
	// 操作系统
	h.Write([]byte(runtime.GOOS + runtime.GOARCH))

	sum := h.Sum(nil)
	// 取前8字节转为 uint64，再取模得到6位数字
	num := binary.BigEndian.Uint64(sum[:8]) % 900000 + 100000
	return fmt.Sprintf("%d", num)
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "outview", "device.json")
}

func loadSaved() string {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return ""
	}
	var info deviceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ""
	}
	return info.Code
}

func save(code string) {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, _ := json.Marshal(deviceInfo{Code: code})
	_ = os.WriteFile(path, data, 0600)
}
