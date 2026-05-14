package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/outview/webrtc-sidecar/internal/webrtc"
)

// Metrics tracks performance metrics
type Metrics struct {
	totalConnections      int32
	activeConnections     int32
	failedConnections     int32
	totalDataSent         int64
	totalDataReceived     int64
	connectionEstablishMs []int64
	mu                    sync.Mutex
}

func (m *Metrics) recordConnectionTime(ms int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionEstablishMs = append(m.connectionEstablishMs, ms)
}

func (m *Metrics) getAverageConnectionTime() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.connectionEstablishMs) == 0 {
		return 0
	}
	var sum int64
	for _, t := range m.connectionEstablishMs {
		sum += t
	}
	return float64(sum) / float64(len(m.connectionEstablishMs))
}

func (m *Metrics) getMinMaxConnectionTime() (int64, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.connectionEstablishMs) == 0 {
		return 0, 0
	}
	min := m.connectionEstablishMs[0]
	max := m.connectionEstablishMs[0]
	for _, t := range m.connectionEstablishMs {
		if t < min {
			min = t
		}
		if t > max {
			max = t
		}
	}
	return min, max
}

// ConnectionPair represents a pair of connected peers
type ConnectionPair struct {
	id      int
	manager *webrtc.POCManager
	ctx     context.Context
	cancel  context.CancelFunc
}

func main() {
	// Command-line flags
	connections := flag.Int("connections", 100, "Number of concurrent PeerConnection pairs")
	duration := flag.Duration("duration", 24*time.Hour, "Test duration")
	dataInterval := flag.Duration("data-interval", 5*time.Second, "Interval for sending test data")
	metricsInterval := flag.Duration("metrics-interval", 10*time.Second, "Interval for logging metrics")

	flag.Parse()

	fmt.Println("=== WebRTC Sidecar Stress Test ===")
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Connections: %d pairs (%d total PeerConnections)\n", *connections, *connections*2)
	fmt.Printf("  Duration: %v\n", *duration)
	fmt.Printf("  Data interval: %v\n", *dataInterval)
	fmt.Printf("  Metrics interval: %v\n", *metricsInterval)
	fmt.Println()

	// Initialize metrics
	metrics := &Metrics{}

	// Context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start connections
	fmt.Printf("Creating %d connection pairs...\n", *connections)
	startTime := time.Now()

	pairs := make([]*ConnectionPair, 0, *connections)
	var wg sync.WaitGroup

	for i := 0; i < *connections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			connStart := time.Now()
			pair, err := createConnectionPair(ctx, id, metrics)
			connDuration := time.Since(connStart).Milliseconds()

			if err != nil {
				fmt.Printf("[Pair %d] Failed to create: %v\n", id, err)
				atomic.AddInt32(&metrics.failedConnections, 1)
				return
			}

			metrics.recordConnectionTime(connDuration)
			atomic.AddInt32(&metrics.activeConnections, 1)
			atomic.AddInt32(&metrics.totalConnections, 1)

			pairs = append(pairs, pair)
			fmt.Printf("[Pair %d] Established in %dms\n", id, connDuration)
		}(i)
	}

	wg.Wait()
	setupDuration := time.Since(startTime)

	fmt.Printf("\n✓ Setup completed in %v\n", setupDuration)
	fmt.Printf("  Active: %d, Failed: %d\n",
		atomic.LoadInt32(&metrics.activeConnections),
		atomic.LoadInt32(&metrics.failedConnections))
	fmt.Println()

	// Start metrics logger
	metricsTicker := time.NewTicker(*metricsInterval)
	defer metricsTicker.Stop()

	// Start data sender
	dataTicker := time.NewTicker(*dataInterval)
	defer dataTicker.Stop()

	fmt.Println("Starting stress test...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	testStart := time.Now()

	// Main loop
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTest duration reached")
			goto cleanup

		case <-sigChan:
			fmt.Println("\nReceived interrupt signal")
			goto cleanup

		case <-metricsTicker.C:
			logMetrics(metrics, time.Since(testStart))

		case <-dataTicker.C:
			sendDataToPairs(pairs, metrics)
		}
	}

cleanup:
	fmt.Println("\nShutting down...")

	// Close all connections
	for _, pair := range pairs {
		if pair != nil {
			pair.cancel()
			if err := pair.manager.Close(); err != nil {
				fmt.Printf("[Pair %d] Error closing: %v\n", pair.id, err)
			}
		}
	}

	// Final report
	testDuration := time.Since(testStart)
	generateReport(metrics, testDuration, setupDuration)
}

// createConnectionPair creates and establishes a PeerConnection pair
func createConnectionPair(ctx context.Context, id int, metrics *Metrics) (*ConnectionPair, error) {
	pairCtx, cancel := context.WithCancel(ctx)

	manager, err := webrtc.NewPOCManager()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Setup offerer
	if err := manager.SetupOfferer(); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to setup offerer: %w", err)
	}

	// Setup answerer
	if err := manager.SetupAnswerer(); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to setup answerer: %w", err)
	}

	// Create offer
	offerSDP, err := manager.CreateOffer()
	if err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	// Set remote offer
	if err := manager.SetRemoteOffer(offerSDP); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to set remote offer: %w", err)
	}

	// Create answer
	answerSDP, err := manager.CreateAnswer()
	if err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	// Set remote answer
	if err := manager.SetRemoteAnswer(answerSDP); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to set remote answer: %w", err)
	}

	// Exchange ICE candidates
	if err := manager.ExchangeICECandidates(); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to exchange ICE candidates: %w", err)
	}

	// Wait for DataChannel
	if err := manager.WaitForDataChannel(10 * time.Second); err != nil {
		cancel()
		manager.Close()
		return nil, fmt.Errorf("failed to wait for data channel: %w", err)
	}

	return &ConnectionPair{
		id:      id,
		manager: manager,
		ctx:     pairCtx,
		cancel:  cancel,
	}, nil
}

// sendDataToPairs sends test data through all connection pairs
func sendDataToPairs(pairs []*ConnectionPair, metrics *Metrics) {
	testData := []byte(fmt.Sprintf("Test data at %s", time.Now().Format(time.RFC3339)))

	for _, pair := range pairs {
		if pair != nil && pair.manager != nil {
			if err := pair.manager.SendData(testData); err != nil {
				// Connection might be closed, skip
				continue
			}
			atomic.AddInt64(&metrics.totalDataSent, int64(len(testData)))
		}
	}
}

// logMetrics logs current performance metrics
func logMetrics(metrics *Metrics, elapsed time.Duration) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	active := atomic.LoadInt32(&metrics.activeConnections)
	failed := atomic.LoadInt32(&metrics.failedConnections)
	dataSent := atomic.LoadInt64(&metrics.totalDataSent)

	fmt.Printf("[%s] Active: %d, Failed: %d, Data sent: %.2f KB, Memory: %.2f MB, Goroutines: %d\n",
		elapsed.Round(time.Second),
		active,
		failed,
		float64(dataSent)/1024,
		float64(m.Alloc)/1024/1024,
		runtime.NumGoroutine())
}

// generateReport generates final test report
func generateReport(metrics *Metrics, testDuration, setupDuration time.Duration) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	avgConnTime := metrics.getAverageConnectionTime()
	minConnTime, maxConnTime := metrics.getMinMaxConnectionTime()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("STRESS TEST REPORT")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Println("\n[Test Configuration]")
	fmt.Printf("  Total connections: %d\n", atomic.LoadInt32(&metrics.totalConnections))
	fmt.Printf("  Test duration: %v\n", testDuration.Round(time.Second))

	fmt.Println("\n[Connection Statistics]")
	fmt.Printf("  Active connections: %d\n", atomic.LoadInt32(&metrics.activeConnections))
	fmt.Printf("  Failed connections: %d\n", atomic.LoadInt32(&metrics.failedConnections))
	fmt.Printf("  Success rate: %.2f%%\n",
		float64(atomic.LoadInt32(&metrics.activeConnections))/float64(atomic.LoadInt32(&metrics.totalConnections))*100)

	fmt.Println("\n[Performance Metrics]")
	fmt.Printf("  Setup time: %v\n", setupDuration.Round(time.Millisecond))
	fmt.Printf("  Avg connection time: %.2f ms\n", avgConnTime)
	fmt.Printf("  Min connection time: %d ms\n", minConnTime)
	fmt.Printf("  Max connection time: %d ms\n", maxConnTime)
	fmt.Printf("  Total data sent: %.2f KB\n", float64(atomic.LoadInt64(&metrics.totalDataSent))/1024)

	fmt.Println("\n[Resource Usage]")
	fmt.Printf("  Memory allocated: %.2f MB\n", float64(m.Alloc)/1024/1024)
	fmt.Printf("  Total memory allocated: %.2f MB\n", float64(m.TotalAlloc)/1024/1024)
	fmt.Printf("  System memory: %.2f MB\n", float64(m.Sys)/1024/1024)
	fmt.Printf("  GC runs: %d\n", m.NumGC)
	fmt.Printf("  Goroutines: %d\n", runtime.NumGoroutine())

	fmt.Println("\n[Verdict]")
	successRate := float64(atomic.LoadInt32(&metrics.activeConnections))/float64(atomic.LoadInt32(&metrics.totalConnections))*100
	memoryMB := float64(m.Alloc)/1024/1024

	if successRate >= 95 && memoryMB < 1000 {
		fmt.Println("  ✓ PASS - System is stable under load")
	} else {
		fmt.Println("  ✗ FAIL - System shows instability")
		if successRate < 95 {
			fmt.Printf("    - Low success rate: %.2f%% (expected >= 95%%)\n", successRate)
		}
		if memoryMB >= 1000 {
			fmt.Printf("    - High memory usage: %.2f MB (expected < 1000 MB)\n", memoryMB)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println()
}
