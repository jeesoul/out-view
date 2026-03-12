// Package client provides the outView client implementation
package client

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		ServerHost:        "localhost",
		ServerPort:        7000,
		LocalPort:         3389, // RDP default port
		HeartbeatInterval: 30,
		AutoReconnect:     true,
		MaxRetries:        10,
		RetryDelay:        5,
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
	return nil
}

// ServerAddr returns the server address in host:port format
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.ServerHost, c.ServerPort)
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
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := DefaultConfig()
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