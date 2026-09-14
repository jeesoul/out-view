package client

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/outview/client/internal/logger"
	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"
)

// launch 与 Stop 共用入口锁，防止 Wait 与从零开始的 Add 并发。
func (c *Client) launch(fn func()) bool {
	c.lifeMu.Lock()
	defer c.lifeMu.Unlock()
	if c.stopping {
		return false
	}
	c.wg.Add(1)
	go func() { defer c.wg.Done(); fn() }()
	return true
}

func (c *Client) connect() error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	c.setState(StateConnecting)
	dialer := net.Dialer{Timeout: c.config.DialTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(c.ctx, "tcp", c.config.ServerAddr())
	if err != nil {
		c.setState(StateDisconnected)
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	c.sessionMu.Lock()
	c.mu.Lock()
	if c.ctx.Err() != nil || c.conn != nil {
		c.mu.Unlock()
		c.sessionMu.Unlock()
		conn.Close()
		return fmt.Errorf("client stopped or already connected")
	}
	c.conn, c.reader, c.writer = conn, bufio.NewReader(conn), bufio.NewWriter(conn)
	c.registered = false
	c.heartbeatSent, c.latencyAt = time.Time{}, time.Time{}
	c.externalPort = 0
	c.mu.Unlock()
	c.sessionMu.Unlock()
	c.setState(StateConnected)
	return nil
}

// Connect 保留一次性查询客户端的拨号接口。
func (c *Client) Connect() error { return c.connect() }

func (c *Client) Register() error {
	msg, err := protocol.NewRegisterMessage(c.config.DeviceID, c.config.Token, c.config.LocalPort)
	if err != nil {
		return err
	}
	return c.sendRaw(msg)
}

func (c *Client) SendData(data []byte) error { return c.sendRaw(protocol.NewDataMessage(data)) }

func (c *Client) SendHeartbeat() error {
	c.mu.Lock()
	conn := c.conn
	// 同时最多一个未应答心跳，避免旧 ACK 被当作新心跳的低延迟结果。
	if conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	if !c.heartbeatSent.IsZero() {
		expired := time.Since(c.heartbeatSent) >= c.config.HeartbeatTimeout
		c.mu.Unlock()
		if expired {
			err := fmt.Errorf("heartbeat acknowledgement timeout")
			c.failConn(conn, err)
			return err
		}
		return nil
	}
	c.heartbeatSent = time.Now()
	c.mu.Unlock()
	msg, err := protocol.NewHeartbeatMessage()
	if err != nil {
		return err
	}
	return c.sendRawFor(conn, msg)
}

func (c *Client) handleHeartbeatAck() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.heartbeatSent.IsZero() {
		c.latency = time.Since(c.heartbeatSent)
		c.latencyAt = time.Now()
		c.heartbeatSent = time.Time{}
	}
}

// GetLatency 返回最近一次真实心跳 RTT；未收到应答、断线或结果过期时返回 false。
func (c *Client) GetLatency() (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latency, c.conn != nil && !c.latencyAt.IsZero() && time.Since(c.latencyAt) <= c.config.HeartbeatTimeout
}

// Start 在自动重连模式下立即返回，首次拨号和后续恢复均由后台执行。
// 关闭自动重连时保留同步拨号错误，兼容一次性 CLI 调用。
func (c *Client) Start() error {
	if err := c.config.Validate(); err != nil {
		return err
	}
	c.lifeMu.Lock()
	if c.started || c.stopping {
		c.lifeMu.Unlock()
		return fmt.Errorf("client already started or stopped")
	}
	c.started = true
	c.wg.Add(1)
	c.lifeMu.Unlock()
	defer c.wg.Done()
	if c.config.AutoReconnect {
		c.launch(c.heartbeatLoop)
		c.launch(c.closeNotificationLoop)
		c.launch(func() {
			err := c.connect()
			if err == nil {
				err = c.Register()
			}
			c.runConnections(err)
		})
		return nil
	}
	err := c.connect()
	if err == nil {
		err = c.Register()
	}
	if err != nil && !c.config.AutoReconnect {
		c.closeConn()
		return err
	}
	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}
	c.launch(c.heartbeatLoop)
	c.launch(c.closeNotificationLoop)
	c.launch(func() { c.runConnections(err) })
	return nil
}

// runConnections 唯一拥有重连顺序：旧读循环返回并清理后才拨号新连接。
func (c *Client) runConnections(initialError error) {
	err := initialError
	attempts := 0
	delay := time.Duration(c.config.RetryDelay) * time.Second
	for c.ctx.Err() == nil {
		if err == nil {
			conn := c.currentConn()
			registered, readErr := c.readConnection(conn)
			if registered {
				attempts = 0
				delay = time.Duration(c.config.RetryDelay) * time.Second
			}
			err = readErr
			c.failConn(conn, err)
		}
		if c.ctx.Err() != nil || !c.config.AutoReconnect {
			return
		}
		if c.config.MaxRetries > 0 && attempts >= c.config.MaxRetries {
			if c.OnError != nil {
				c.OnError(fmt.Errorf("max reconnect attempts reached: %w", err))
			}
			return
		}
		c.setState(StateReconnecting)
		wait := time.Duration(float64(delay) * (0.8 + 0.4*rand.Float64()))
		timer := time.NewTimer(wait)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		attempts++
		c.reconnectCount.Add(1)
		logger.Info("Reconnect attempt %d", attempts)
		err = c.connect()
		if err == nil {
			c.prepareWebRTCReconnect()
			err = c.Register()
		}
		if err != nil {
			c.closeConn()
		}
		delay *= 2
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
	}
}

func (c *Client) currentConn() net.Conn { c.mu.Lock(); defer c.mu.Unlock(); return c.conn }

func (c *Client) readConnection(conn net.Conn) (bool, error) {
	if conn == nil {
		return false, fmt.Errorf("not connected")
	}
	decoder := protocol.NewDecoder(bufio.NewReader(conn))
	registrationDeadline := time.Now().Add(c.config.RegisterTimeout)
	registered := false
	for {
		deadline := time.Now().Add(c.config.ReadTimeout)
		if !registered && registrationDeadline.Before(deadline) {
			deadline = registrationDeadline
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return registered, err
		}
		msg, err := decoder.Decode()
		if err != nil {
			return registered, err
		}
		if c.ctx.Err() != nil || c.currentConn() != conn {
			return registered, fmt.Errorf("connection superseded or stopped")
		}
		c.handleMessage(msg)
		c.mu.Lock()
		registered = c.registered
		c.mu.Unlock()
	}
}

func (c *Client) StartReadLoop() error {
	conn := c.currentConn()
	_, err := c.readConnection(conn)
	c.failConn(conn, err)
	return err
}

// sendRawFor 的连接快照在等待写锁之前取得；旧转发任务不会把数据写到新会话。
func (c *Client) sendRaw(msg *protocol.Message) error { return c.sendRawFor(c.currentConn(), msg) }
func (c *Client) sendRawFor(conn net.Conn, msg *protocol.Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	writer := c.writer
	valid := conn != nil && conn == c.conn && c.ctx.Err() == nil
	c.mu.Unlock()
	if !valid || writer == nil {
		return fmt.Errorf("connection unavailable or superseded")
	}
	err := conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
	if err == nil {
		err = protocol.NewEncoder(writer).Encode(msg)
	}
	if err == nil {
		err = writer.Flush()
	}
	if err != nil {
		c.failConn(conn, err)
	}
	return err
}

func (c *Client) failConn(conn net.Conn, err error) {
	if conn == nil {
		return
	}
	c.sessionMu.Lock()
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		c.sessionMu.Unlock()
		return
	}
	c.conn, c.reader, c.writer = nil, nil, nil
	c.latencyAt, c.heartbeatSent = time.Time{}, time.Time{}
	c.externalPort = 0
	c.mu.Unlock()
	conn.Close()
	c.closeAllLocalConns()
	c.webrtcMu.Lock()
	c.usingWebRTC = false
	c.webrtcMu.Unlock()
	c.sessionMu.Unlock()
	c.setState(StateDisconnected)
	if err != nil && c.ctx.Err() == nil {
		logger.Warn("Control connection closed: %v", err)
	}
}

func (c *Client) closeConn() { c.failConn(c.currentConn(), nil) }

func (c *Client) heartbeatLoop() {
	interval := time.Duration(c.config.HeartbeatInterval) * time.Second
	// 额外监测心跳期限，不让较长发送间隔拖延故障检测。
	check := interval
	if c.config.HeartbeatTimeout/2 < check {
		check = c.config.HeartbeatTimeout / 2
	}
	if check < time.Millisecond {
		check = time.Millisecond
	}
	ticker := time.NewTicker(check)
	defer ticker.Stop()
	next := time.Now()
	for {
		select {
		case <-c.ctx.Done():
			return
		case conn := <-c.controlOverload:
			c.failConn(conn, fmt.Errorf("close notification queue overloaded"))
		case now := <-ticker.C:
			c.mu.Lock()
			pending := c.heartbeatSent
			conn := c.conn
			c.mu.Unlock()
			if !pending.IsZero() && now.Sub(pending) >= c.config.HeartbeatTimeout {
				c.failConn(conn, fmt.Errorf("heartbeat acknowledgement timeout"))
				continue
			}
			state := c.GetState()
			if !now.Before(next) && (state == StateConnected || state == StateRegistered) {
				_ = c.SendHeartbeat()
				next = now.Add(interval)
			}
		}
	}
}

// Stop 可并发重复调用；取消拨号、读写和定时器后等待所有自有任务退出。
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		c.lifeMu.Lock()
		c.stopping = true
		c.cancel()
		c.lifeMu.Unlock()
		c.closeConn()
		c.closeAllLocalConns()
		c.proxyManager.CloseAll()
		// 必须先取得初始化锁再选择当前 manager，避免关闭快照后遗漏并发替换的新对象。
		c.signalingMu.Lock()
		c.webrtcMu.RLock()
		mgr := c.webrtcManager
		c.webrtcMu.RUnlock()
		if mgr != nil {
			mgr.Close()
		}
		c.signalingMu.Unlock()
		c.wg.Wait()
		c.setState(StateDisconnected)
		close(c.stopped)
	})
	<-c.stopped
}

func (c *Client) prepareWebRTCReconnect() {
	c.signalingMu.Lock()
	defer c.signalingMu.Unlock()
	if c.ctx.Err() != nil || c.currentConn() == nil {
		return
	}
	c.webrtcMu.Lock()
	old := c.webrtcManager
	if old == nil || c.webrtcCfg == nil {
		c.webrtcMu.Unlock()
		return
	}
	id := old.ConnectionID()
	c.webrtcManager = clientwebrtc.NewManager(id, c.webrtcCfg, nil)
	c.webrtcRecoveryCount.Add(1)
	c.webrtcMu.Unlock()
	old.Close()
}
