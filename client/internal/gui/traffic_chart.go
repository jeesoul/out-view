package gui

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const trafficChartSampleCap = 60

// TrafficStats tracks cumulative byte counts using atomics.
type TrafficStats struct {
	BytesSent     atomic.Uint64
	BytesReceived atomic.Uint64
}

// trafficSample represents a single point in the rolling traffic window.
type trafficSample struct {
	at       time.Time
	sent     uint64
	received uint64
}

// TrafficChart displays current send/receive rates and history.
type TrafficChart struct {
	stats TrafficStats

	mu      sync.Mutex
	samples []trafficSample

	sentLabel *canvas.Text
	recvLabel *canvas.Text
	totalLine *widget.Label
	root      fyne.CanvasObject

	stopCh chan struct{}
	once   sync.Once
}

// NewTrafficChart creates a new chart and starts the periodic refresher.
func NewTrafficChart() *TrafficChart {
	c := &TrafficChart{
		samples: make([]trafficSample, 0, trafficChartSampleCap),
		stopCh:  make(chan struct{}),
	}
	c.sentLabel = canvas.NewText("↑ 0 B/s", colorOrange())
	c.sentLabel.TextStyle = fyne.TextStyle{Bold: true}
	c.recvLabel = canvas.NewText("↓ 0 B/s", colorGreen())
	c.recvLabel.TextStyle = fyne.TextStyle{Bold: true}
	c.totalLine = widget.NewLabel("累计：发送 0 B / 接收 0 B")

	row := container.NewHBox(c.sentLabel, c.recvLabel)
	c.root = container.NewVBox(row, c.totalLine)

	go c.tickLoop()
	return c
}

// Widget returns the renderable canvas object.
func (c *TrafficChart) Widget() fyne.CanvasObject {
	return c.root
}

// Stop halts the background refresh goroutine. Safe to call multiple times.
func (c *TrafficChart) Stop() {
	c.once.Do(func() { close(c.stopCh) })
}

// AddSample records a snapshot of cumulative byte counters at the current time.
func (c *TrafficChart) AddSample(sent, received uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples = append(c.samples, trafficSample{at: time.Now(), sent: sent, received: received})
	if len(c.samples) > trafficChartSampleCap {
		c.samples = c.samples[len(c.samples)-trafficChartSampleCap:]
	}
}

// RecordSend increments the cumulative sent-bytes counter.
func (c *TrafficChart) RecordSend(n uint64) {
	c.stats.BytesSent.Add(n)
}

// RecordReceive increments the cumulative received-bytes counter.
func (c *TrafficChart) RecordReceive(n uint64) {
	c.stats.BytesReceived.Add(n)
}

func (c *TrafficChart) tickLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.refresh()
		}
	}
}

func (c *TrafficChart) refresh() {
	sent := c.stats.BytesSent.Load()
	recv := c.stats.BytesReceived.Load()
	c.AddSample(sent, recv)

	sentRate, recvRate := c.currentRates()
	sentText := fmt.Sprintf("↑ %s/s", humanBytes(sentRate))
	recvText := fmt.Sprintf("↓ %s/s", humanBytes(recvRate))
	totalText := fmt.Sprintf("累计：发送 %s / 接收 %s", humanBytes(sent), humanBytes(recv))

	c.sentLabel.Text = sentText
	c.recvLabel.Text = recvText
	canvas.Refresh(c.sentLabel)
	canvas.Refresh(c.recvLabel)
	c.totalLine.SetText(totalText)
}

func (c *TrafficChart) currentRates() (uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.samples) < 2 {
		return 0, 0
	}
	first := c.samples[0]
	last := c.samples[len(c.samples)-1]
	dur := last.at.Sub(first.at).Seconds()
	if dur <= 0 {
		return 0, 0
	}
	var sentDiff, recvDiff uint64
	if last.sent >= first.sent {
		sentDiff = last.sent - first.sent
	}
	if last.received >= first.received {
		recvDiff = last.received - first.received
	}
	return uint64(float64(sentDiff) / dur), uint64(float64(recvDiff) / dur)
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"KB", "MB", "GB", "TB"}[exp]
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), suffix)
}
