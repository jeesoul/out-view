package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/outview/client/internal/client"
)

var (
	Version   = "1.0.2-SNAPSHOT"
	BuildDate = "unknown"
)

func main() {
	// Parse command line flags
	serverHost := flag.String("host", "", "Server host address")
	serverPort := flag.Int("port", 7000, "Server control port")
	deviceID := flag.String("device-id", "", "Device ID for registration")
	token := flag.String("token", "", "Authentication token")
	localPort := flag.Int("local-port", 3389, "Local service port (default: 3389 for RDP)")
	heartbeatInterval := flag.Int("heartbeat", 30, "Heartbeat interval in seconds")
	configFile := flag.String("config", "", "Config file path (default: auto-detect config.txt)")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "outView Client - Remote Desktop Tunnel Client\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfig file format (config.txt):\n")
		fmt.Fprintf(os.Stderr, "  host=192.168.1.100\n")
		fmt.Fprintf(os.Stderr, "  port=7000\n")
		fmt.Fprintf(os.Stderr, "  device-id=my-device\n")
		fmt.Fprintf(os.Stderr, "  token=secret-token\n")
		fmt.Fprintf(os.Stderr, "  local-port=3389\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -config config.txt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -host 192.168.1.100 -device-id my-device -token secret-token\n", os.Args[0])
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("outView Client v%s (built: %s)\n", Version, BuildDate)
		os.Exit(0)
	}

	// Create config - start with defaults
	config := client.DefaultConfig()

	// Try to load from config file
	configPath := *configFile
	if configPath == "" {
		configPath = client.FindConfigFile()
	}
	if configPath != "" {
		fileConfig, err := client.LoadFromFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[Warning] Failed to load config file: %v\n", err)
		} else {
			config = fileConfig
			fmt.Printf("[Info] Loaded config from: %s\n", configPath)
		}
	}

	// Override with command line flags (if provided)
	if *serverHost != "" {
		config.ServerHost = *serverHost
	}
	if *serverPort > 0 && *serverPort != 7000 {
		config.ServerPort = *serverPort
	}
	if *deviceID != "" {
		config.DeviceID = *deviceID
	}
	if *token != "" {
		config.Token = *token
	}
	if *localPort > 0 && *localPort != 3389 {
		config.LocalPort = *localPort
	}
	if *heartbeatInterval > 0 && *heartbeatInterval != 30 {
		config.HeartbeatInterval = *heartbeatInterval
	}

	// Load from environment variables (if not set via flags)
	envConfig := client.LoadFromEnv()
	if config.ServerHost == "" || config.ServerHost == "localhost" {
		if envConfig.ServerHost != "localhost" {
			config.ServerHost = envConfig.ServerHost
		}
	}
	if config.DeviceID == "" {
		config.DeviceID = envConfig.DeviceID
	}
	if config.Token == "" {
		config.Token = envConfig.Token
	}

	// Validate config
	if err := config.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n\n", err)
		fmt.Fprintf(os.Stderr, "Required parameters:\n")
		fmt.Fprintf(os.Stderr, "  -host        Server host address\n")
		fmt.Fprintf(os.Stderr, "  -device-id   Device ID\n")
		fmt.Fprintf(os.Stderr, "  -token       Authentication token\n")
		fmt.Fprintf(os.Stderr, "\nYou can also set environment variables:\n")
		fmt.Fprintf(os.Stderr, "  OUTVIEW_SERVER_HOST, OUTVIEW_SERVER_PORT\n")
		fmt.Fprintf(os.Stderr, "  OUTVIEW_DEVICE_ID, OUTVIEW_TOKEN, OUTVIEW_LOCAL_PORT\n")
		os.Exit(1)
	}

	// Print banner
	printBanner(config)

	// Create client
	c := client.NewClient(config)

	// Set callbacks
	c.OnStateChange = func(old, new client.State) {
		fmt.Printf("[State] %s -> %s\n", old, new)
	}

	c.OnRegisterResult = func(success bool, externalPort int, err error) {
		if success {
			fmt.Printf("[Register] Success! External port: %d\n", externalPort)
			fmt.Printf("[Info] You can now connect to %s:%d for RDP access\n", config.ServerHost, externalPort)
		} else {
			fmt.Printf("[Register] Failed: %v\n", err)
		}
	}

	c.OnError = func(err error) {
		fmt.Printf("[Error] %v\n", err)
	}

	c.OnDataReceived = func(data []byte) {
		fmt.Printf("[Data] Received %d bytes from tunnel\n", len(data))
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n[Info] Shutting down...")
		c.Stop()
		os.Exit(0)
	}()

	// Start client
	fmt.Println("[Info] Connecting to server...")
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] Failed to start client: %v\n", err)
		os.Exit(1)
	}

	// Wait for registration
	fmt.Println("[Info] Waiting for registration...")

	// Keep running
	select {}
}

func printBanner(config *client.Config) {
	fmt.Println()
	fmt.Println("====================================")
	fmt.Println("       outView Client v" + Version)
	fmt.Println("====================================")
	fmt.Printf("Server:      %s:%d\n", config.ServerHost, config.ServerPort)
	fmt.Printf("Device ID:   %s\n", config.DeviceID)
	fmt.Printf("Local Port:  %d\n", config.LocalPort)
	fmt.Printf("Heartbeat:   %ds\n", config.HeartbeatInterval)
	fmt.Println("====================================")
	fmt.Println()
}