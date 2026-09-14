package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/outview/client/internal/logger"
	"github.com/outview/client/internal/protocol"
)

// connectionConn 的队列和 socket 只属于创建它的控制连接。
type connectionConn struct {
	conn        net.Conn
	control     net.Conn
	closeCh     chan struct{}
	once        sync.Once
	mu          sync.Mutex
	queue       chan []byte
	queuedBytes int
	cancel      context.CancelFunc
	ctx         context.Context
}

type closeNotification struct {
	control net.Conn
	id      string
}

func (c *Client) forwardToLocal(id string, data []byte) {
	obj := c.getOrCreateConnection(id)
	if obj == nil {
		return
	}
	obj.mu.Lock()
	select {
	case <-obj.closeCh:
		obj.mu.Unlock()
		return
	default:
	}
	if len(data) > c.config.LocalQueueBytes-obj.queuedBytes {
		obj.mu.Unlock()
		c.finishLocal(id, obj, true)
		return
	}
	select {
	case obj.queue <- data:
		obj.queuedBytes += len(data)
		obj.mu.Unlock()
	default:
		obj.mu.Unlock()
		c.finishLocal(id, obj, true)
	}
}

// 控制读循环只建队列；本地 Dial 与 Write 始终在独立任务中执行。
func (c *Client) getOrCreateConnection(id string) *connectionConn {
	control := c.currentConn()
	if control == nil || c.ctx.Err() != nil || id == "" {
		return nil
	}
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.currentConn() != control || c.closedLocal[id] {
		return nil
	}
	if obj := c.localConnections[id]; obj != nil {
		return obj
	}
	if len(c.localConnections) >= c.config.MaxLocalConnections {
		c.rememberClosedLocal(id)
		c.notifyLocalClose(control, id)
		return nil
	}
	ctx, cancel := context.WithCancel(c.ctx)
	obj := &connectionConn{control: control, closeCh: make(chan struct{}), queue: make(chan []byte, c.config.LocalQueueSize), ctx: ctx, cancel: cancel}
	c.localConnections[id] = obj
	if !c.launch(func() { c.writeToLocal(id, obj) }) {
		delete(c.localConnections, id)
		cancel()
		return nil
	}
	return obj
}

func (c *Client) writeToLocal(id string, obj *connectionConn) {
	defer c.finishLocal(id, obj, true)
	dialer := net.Dialer{Timeout: c.config.LocalDialTimeout}
	conn, err := dialer.DialContext(obj.ctx, "tcp", c.config.LocalAddr())
	if err != nil {
		logger.Warn("Local service dial failed: %v", err)
		return
	}
	obj.mu.Lock()
	select {
	case <-obj.closeCh:
		obj.mu.Unlock()
		conn.Close()
		return
	default:
	}
	obj.conn = conn
	obj.mu.Unlock()
	if !c.launch(func() { c.readFromLocal(id, obj) }) {
		return
	}
	for {
		select {
		case <-obj.ctx.Done():
			return
		case data := <-obj.queue:
			if err = conn.SetWriteDeadline(time.Now().Add(c.config.LocalWriteTimeout)); err != nil {
				return
			}
			n, writeErr := conn.Write(data)
			c.bytesRecv.Add(int64(n))
			obj.mu.Lock()
			obj.queuedBytes -= len(data)
			obj.mu.Unlock()
			if writeErr != nil || n != len(data) {
				return
			}
		}
	}
}

func (c *Client) readFromLocal(id string, obj *connectionConn) {
	defer c.finishLocal(id, obj, true)
	obj.mu.Lock()
	conn := obj.conn
	obj.mu.Unlock()
	if conn == nil {
		return
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			if sendErr := c.sendRawFor(obj.control, protocol.NewDataMessageWithConnectionID(id, buf[:n])); sendErr != nil {
				return
			}
			c.bytesSent.Add(int64(n))
		}
		if err != nil {
			if err != io.EOF && obj.ctx.Err() == nil {
				logger.Debug("Local connection closed: %v", err)
			}
			return
		}
	}
}

// finishLocal 使用对象身份删除，旧 goroutine 不会删除相同 ID 的新 socket。
// once 也覆盖关闭通知，收到远端 type 7 时不会再向远端反射 type 7。
func (c *Client) finishLocal(id string, obj *connectionConn, notify bool) {
	if obj == nil {
		return
	}
	obj.once.Do(func() {
		c.connMu.Lock()
		if c.localConnections[id] == obj {
			delete(c.localConnections, id)
			c.rememberClosedLocal(id)
		}
		c.connMu.Unlock()
		obj.mu.Lock()
		close(obj.closeCh)
		if obj.cancel != nil {
			obj.cancel()
		}
		if obj.conn != nil {
			obj.conn.Close()
		}
		obj.mu.Unlock()
		if notify {
			c.notifyLocalClose(obj.control, id)
		}
	})
}

// 有界保留近期关闭 ID，丢弃已在控制流中排队的尾部数据，避免马上重建 socket。
func (c *Client) rememberClosedLocal(id string) {
	if c.closedLocal[id] {
		return
	}
	if len(c.closedOrder) >= 1024 {
		delete(c.closedLocal, c.closedOrder[0])
		c.closedOrder = c.closedOrder[1:]
	}
	c.closedLocal[id] = true
	c.closedOrder = append(c.closedOrder, id)
}

func (c *Client) closeConnection(id string) {
	c.connMu.Lock()
	obj := c.localConnections[id]
	c.rememberClosedLocal(id)
	c.connMu.Unlock()
	c.finishLocal(id, obj, false)
}

func (c *Client) closeAllLocalConns() {
	c.connMu.Lock()
	old := c.localConnections
	c.localConnections = make(map[string]*connectionConn)
	c.closedLocal = make(map[string]bool)
	c.closedOrder = nil
	c.connMu.Unlock()
	for id, obj := range old {
		c.finishLocal(id, obj, false)
	}
}

func (c *Client) notifyLocalClose(control net.Conn, id string) {
	if control == nil || c.ctx.Err() != nil {
		return
	}
	select {
	case c.closeNotifications <- closeNotification{control: control, id: id}:
	default:
		logger.Warn("Local close notification queue full: %s", id)
		// 不丢弃关闭后继续假装在线；由独立控制任务关闭该代连接，服务端统一回收。
		// 此处可能持 connMu，因此只发送有界信号，不直接清理或创建额外 goroutine。
		select {
		case c.controlOverload <- control:
		default:
		}
	}
}

func (c *Client) closeNotificationLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case item := <-c.closeNotifications:
			if err := c.sendRawFor(item.control, protocol.NewCloseConnectionMessage(item.id)); err != nil && c.ctx.Err() == nil {
				logger.Debug("Close notification not sent: %v", fmt.Errorf("connection %s: %w", item.id, err))
			}
		}
	}
}
