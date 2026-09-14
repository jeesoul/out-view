// Package client provides the outView client implementation
package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds the client configuration
type Config struct {
	// Server address
	ServerHost string
	ServerPort int

	// Authentication
	DeviceID string
	Token    string

	// Local service to forward
	LocalPort int

	// Heartbeat interval in seconds
	HeartbeatInterval int

	// Reconnect settings
	AutoReconnect bool
	MaxRetries    int
	RetryDelay    int // seconds

	// 控制连接拨号、读空闲、单次写入及心跳应答超时；零值使用默认值。
	DialTimeout      time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	HeartbeatTimeout time.Duration
	RegisterTimeout  time.Duration // 注册必须在此期限内完成，其他消息不会延长期限。
	// 本地服务拨号和写入超时；每条连接独立排队，超过字节/消息上限只关闭该连接。
	LocalDialTimeout    time.Duration
	LocalWriteTimeout   time.Duration
	LocalQueueBytes     int
	LocalQueueSize      int
	MaxLocalConnections int // 同时转发连接数量上限，防止无界创建 goroutine。
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		ServerHost:          "localhost",
		ServerPort:          7000,
		LocalPort:           3389, // RDP default port
		HeartbeatInterval:   30,
		AutoReconnect:       true,
		MaxRetries:          0, // 0 = 无限重连（被控端需常驻在线）
		RetryDelay:          5,
		DialTimeout:         10 * time.Second,
		ReadTimeout:         90 * time.Second,
		WriteTimeout:        10 * time.Second,
		HeartbeatTimeout:    90 * time.Second,
		RegisterTimeout:     15 * time.Second,
		LocalDialTimeout:    5 * time.Second,
		LocalWriteTimeout:   10 * time.Second,
		LocalQueueBytes:     1024 * 1024,
		LocalQueueSize:      64,
		MaxLocalConnections: 128,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.ServerHost == "" {
		return fmt.Errorf("server host is required")
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("invalid server port: %d", c.ServerPort)
	}
	if c.DeviceID == "" {
		return fmt.Errorf("device ID is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	if c.LocalPort <= 0 || c.LocalPort > 65535 {
		return fmt.Errorf("invalid local port: %d", c.LocalPort)
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("invalid heartbeat interval: %d", c.HeartbeatInterval)
	}
	return c.validateConnectionSettings()
}

// 零值允许旧配置沿用默认值，负值和会造成溢出/过量分配的值明确报错。
func (c *Config) validateConnectionSettings() error {
	for name, d := range map[string]time.Duration{"dial-timeout": c.DialTimeout, "read-timeout": c.ReadTimeout,
		"write-timeout": c.WriteTimeout, "heartbeat-timeout": c.HeartbeatTimeout, "register-timeout": c.RegisterTimeout,
		"local-dial-timeout": c.LocalDialTimeout, "local-write-timeout": c.LocalWriteTimeout} {
		if d < 0 || d > 24*time.Hour {
			return fmt.Errorf("invalid %s: must be between 0 and 24h", name)
		}
	}
	if c.LocalQueueBytes < 0 || c.LocalQueueBytes > 64*1024*1024 {
		return fmt.Errorf("invalid local-queue-bytes: maximum 64 MiB")
	}
	if c.LocalQueueSize < 0 || c.LocalQueueSize > 4096 {
		return fmt.Errorf("invalid local-queue-size: maximum 4096")
	}
	if c.MaxLocalConnections < 0 || c.MaxLocalConnections > 4096 {
		return fmt.Errorf("invalid max-local-connections: maximum 4096")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("invalid max-retries: must be non-negative")
	}
	if c.RetryDelay < 0 || c.RetryDelay > 3600 {
		return fmt.Errorf("invalid retry-delay: maximum 3600 seconds")
	}
	if c.HeartbeatInterval > 86400 {
		return fmt.Errorf("invalid heartbeat: maximum 86400 seconds")
	}
	return nil
}

// ServerAddr returns the server address in host:port format
func (c *Config) ServerAddr() string {
	return net.JoinHostPort(c.ServerHost, strconv.Itoa(c.ServerPort))
}

// LocalAddr returns the local service address in host:port format
func (c *Config) LocalAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", c.LocalPort)
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	if host := os.Getenv("OUTVIEW_SERVER_HOST"); host != "" {
		cfg.ServerHost = host
	}
	if port := os.Getenv("OUTVIEW_SERVER_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.ServerPort)
	}
	if deviceID := os.Getenv("OUTVIEW_DEVICE_ID"); deviceID != "" {
		cfg.DeviceID = deviceID
	}
	if token := os.Getenv("OUTVIEW_TOKEN"); token != "" {
		cfg.Token = token
	}
	if localPort := os.Getenv("OUTVIEW_LOCAL_PORT"); localPort != "" {
		fmt.Sscanf(localPort, "%d", &cfg.LocalPort)
	}

	return cfg
}

// String returns a string representation of the config (without sensitive data)
func (c *Config) String() string {
	return fmt.Sprintf("Server=%s:%d, DeviceID=%s, LocalPort=%d",
		c.ServerHost, c.ServerPort, c.DeviceID, c.LocalPort)
}

// LoadFromFile loads configuration from a config file
// File format: key=value (one per line)
// Supports: host, port, device-id, token, local-port, heartbeat
func LoadFromFile(filename string) (*Config, error) {
	return LoadFromFileWithDefaults(filename, DefaultConfig())
}

// 零值保持旧版 Config 字面量兼容；复制配置后补齐，不修改调用方对象。
func (c *Config) applyTimeoutDefaults() {
	d := DefaultConfig()
	pairs := [][2]*time.Duration{{&c.DialTimeout, &d.DialTimeout}, {&c.ReadTimeout, &d.ReadTimeout},
		{&c.WriteTimeout, &d.WriteTimeout}, {&c.HeartbeatTimeout, &d.HeartbeatTimeout}, {&c.RegisterTimeout, &d.RegisterTimeout},
		{&c.LocalDialTimeout, &d.LocalDialTimeout}, {&c.LocalWriteTimeout, &d.LocalWriteTimeout}}
	for _, pair := range pairs {
		if *pair[0] == 0 {
			*pair[0] = *pair[1]
		}
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = d.HeartbeatInterval
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = d.RetryDelay
	}
	if c.LocalQueueBytes == 0 {
		c.LocalQueueBytes = d.LocalQueueBytes
	}
	if c.LocalQueueSize == 0 {
		c.LocalQueueSize = d.LocalQueueSize
	}
	if c.MaxLocalConnections == 0 {
		c.MaxLocalConnections = d.MaxLocalConnections
	}
}

// LoadFromFileWithDefaults 复制默认配置后应用文件配置，不修改调用方的默认值。
func LoadFromFileWithDefaults(filename string, defaults *Config) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if defaults == nil {
		defaults = DefaultConfig()
	}
	copyConfig := *defaults
	cfg := &copyConfig
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])
		// 时间支持 500ms、10s、2m；纯数字沿用秒单位。
		durations := map[string]*time.Duration{
			"dial-timeout": &cfg.DialTimeout, "read-timeout": &cfg.ReadTimeout,
			"write-timeout": &cfg.WriteTimeout, "heartbeat-timeout": &cfg.HeartbeatTimeout,
			"register-timeout":   &cfg.RegisterTimeout,
			"local-dial-timeout": &cfg.LocalDialTimeout, "local-write-timeout": &cfg.LocalWriteTimeout,
		}
		if target, ok := durations[key]; ok {
			if _, err := strconv.Atoi(value); err == nil {
				value += "s"
			}
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return nil, fmt.Errorf("invalid %s: %s", key, value)
			}
			*target = duration
			continue
		}
		limits := map[string]*int{"retry-delay": &cfg.RetryDelay, "local-queue-bytes": &cfg.LocalQueueBytes,
			"local-queue-size": &cfg.LocalQueueSize, "max-local-connections": &cfg.MaxLocalConnections}
		if target, ok := limits[key]; ok {
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid %s: %s", key, value)
			}
			*target = n
			continue
		}

		switch key {
		case "host":
			cfg.ServerHost = value
		case "port":
			fmt.Sscanf(value, "%d", &cfg.ServerPort)
		case "device-id", "deviceid", "device_id":
			cfg.DeviceID = value
		case "token":
			cfg.Token = value
		case "local-port", "localport", "local_port":
			fmt.Sscanf(value, "%d", &cfg.LocalPort)
		case "heartbeat":
			fmt.Sscanf(value, "%d", &cfg.HeartbeatInterval)
		case "max-retries", "maxretries", "max_retries":
			fmt.Sscanf(value, "%d", &cfg.MaxRetries)
		case "auto-reconnect", "autoreconnect", "auto_reconnect":
			b := strings.ToLower(value)
			cfg.AutoReconnect = b == "true" || b == "1" || b == "yes"
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// FindConfigFile searches for config file in common locations
// Returns the filename if found, empty string otherwise
func FindConfigFile() string {
	// Check common config file names
	configNames := []string{
		"config.txt",
		"config.ini",
		"outview.conf",
		"outview.ini",
	}

	// Get executable directory
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		for _, name := range configNames {
			configPath := filepath.Join(exeDir, name)
			if _, err := os.Stat(configPath); err == nil {
				return configPath
			}
		}
	}

	// Check current directory
	for _, name := range configNames {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}

	return ""
}
