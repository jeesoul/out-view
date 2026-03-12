package client

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Proxy represents a local proxy connection
type Proxy struct {
	localAddr string
	conn      net.Conn
	mu        sync.Mutex
}

// NewProxy creates a new proxy to local service
func NewProxy(localPort int) *Proxy {
	return &Proxy{
		localAddr: fmt.Sprintf("127.0.0.1:%d", localPort),
	}
}

// Connect connects to the local service
func (p *Proxy) Connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		return nil
	}

	conn, err := net.DialTimeout("tcp", p.localAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to local service at %s: %w", p.localAddr, err)
	}

	p.conn = conn
	return nil
}

// Close closes the proxy connection
func (p *Proxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}
	return nil
}

// Read reads data from the local service
func (p *Proxy) Read(buf []byte) (int, error) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return 0, fmt.Errorf("proxy not connected")
	}

	return conn.Read(buf)
}

// Write writes data to the local service
func (p *Proxy) Write(data []byte) (int, error) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return 0, fmt.Errorf("proxy not connected")
	}

	return conn.Write(data)
}

// IsConnected returns whether the proxy is connected
func (p *Proxy) IsConnected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn != nil
}

// ProxyManager manages multiple proxy connections
type ProxyManager struct {
	proxies map[string]*Proxy // session ID -> proxy
	mu      sync.RWMutex
}

// NewProxyManager creates a new proxy manager
func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		proxies: make(map[string]*Proxy),
	}
}

// Create creates a new proxy for a session
func (pm *ProxyManager) Create(sessionID string, localPort int) (*Proxy, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.proxies[sessionID]; exists {
		return nil, fmt.Errorf("proxy already exists for session: %s", sessionID)
	}

	proxy := NewProxy(localPort)
	if err := proxy.Connect(); err != nil {
		return nil, err
	}

	pm.proxies[sessionID] = proxy
	return proxy, nil
}

// Get gets a proxy by session ID
func (pm *ProxyManager) Get(sessionID string) (*Proxy, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	proxy, exists := pm.proxies[sessionID]
	return proxy, exists
}

// Remove removes and closes a proxy
func (pm *ProxyManager) Remove(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if proxy, exists := pm.proxies[sessionID]; exists {
		proxy.Close()
		delete(pm.proxies, sessionID)
	}
}

// CloseAll closes all proxies
func (pm *ProxyManager) CloseAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, proxy := range pm.proxies {
		proxy.Close()
	}
	pm.proxies = make(map[string]*Proxy)
}

// Pipe pipes data between two connections
func Pipe(dst io.Writer, src io.Reader, done chan<- error) {
	_, err := io.Copy(dst, src)
	done <- err
}