package client

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"
)

func stabilityConfig(t *testing.T) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "client.conf")
	err := os.WriteFile(path, []byte("host=127.0.0.1\ndevice-id=test\ntoken=test\nheartbeat=1\nretry-delay=1\nread-timeout=200ms\nwrite-timeout=100ms\nlocal-write-timeout=200ms\nlocal-queue-bytes=10485760\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestAutoReconnectStartDoesNotWaitForDial(t *testing.T) {
	cfg := stabilityConfig(t)
	c := NewClient(cfg)
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	defer c.Stop()
	defer release()
	c.OnStateChange = func(_, next State) {
		if next == StateConnecting {
			<-gate
		}
	}
	done := make(chan error, 1)
	go func() { done <- c.Start() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(200 * time.Millisecond):
		release()
		<-done
		t.Fatal("Start waited for the initial connection attempt")
	}
}

func TestConfigRejectsNegativeConnectionLimits(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.ReadTimeout = -time.Second },
		func(c *Config) { c.LocalQueueBytes = -1 },
		func(c *Config) { c.LocalQueueSize = -1 },
		func(c *Config) { c.MaxLocalConnections = -1 },
		func(c *Config) { c.MaxRetries = -1 },
	} {
		cfg := stabilityConfig(t)
		change(cfg)
		if err := cfg.Validate(); err == nil {
			t.Error("invalid connection limit accepted")
		}
	}
}

func TestStartDoesNotNormalizeNegativeLimits(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.LocalQueueSize = -1
	c := NewClient(cfg)
	defer c.Stop()
	if err := c.Start(); err == nil {
		t.Fatal("Start silently replaced invalid negative queue limit")
	}
}

func TestConnectAcceptsIPv6Host(t *testing.T) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{IP: net.ParseIP("::1")})
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer l.Close()
	cfg := stabilityConfig(t)
	cfg.ServerHost = "::1"
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	c := NewClient(cfg)
	defer c.Stop()
	if err := c.Connect(); err != nil {
		t.Fatalf("IPv6 loopback dial failed: %v", err)
	}
}

// 重连后才调度到的旧 offer 任务不能把旧 manager 的信令写入新控制连接。
func TestDelayedOldWebRTCOfferCannotWriteNewControl(t *testing.T) {
	cfg := stabilityConfig(t)
	rtc := clientwebrtc.DefaultConfig()
	rtc.ICEServers = nil
	c := NewClientWithWebRTC(cfg, "old", rtc)
	old := c.webrtcManager
	defer old.Close()
	c.webrtcManager = clientwebrtc.NewManager("new", rtc, nil)
	local, peer := net.Pipe()
	defer peer.Close()
	defer c.Stop()
	c.conn, c.reader, c.writer = local, bufio.NewReader(local), bufio.NewWriter(local)
	done := make(chan struct{})
	go func() { c.initiateWebRTCOffer(old); close(done) }()
	peer.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	msg, err := protocol.NewDecoder(peer).Decode()
	if err == nil {
		t.Errorf("old manager emitted message type %d on new connection", msg.Header.Type)
	}
	<-done
}

func testListener(t *testing.T) *net.TCPListener {
	t.Helper()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

// 首次拨号失败不能终止自动恢复，否则服务端稍后启动时客户端永不注册。
func TestStartRetriesInitialConnectionFailure(t *testing.T) {
	l := testListener(t)
	cfg := stabilityConfig(t)
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	l.Close()
	c := NewClient(cfg)
	retrying := make(chan struct{}, 1)
	c.OnStateChange = func(_, next State) {
		if next == StateReconnecting {
			select {
			case retrying <- struct{}{}:
			default:
			}
		}
	}
	defer c.Stop()
	if err := c.Start(); err != nil {
		t.Fatalf("initial failure must schedule reconnect: %v", err)
	}
	select {
	case <-retrying:
	case <-time.After(time.Second):
		t.Fatal("initial failure did not enter retry state")
	}
	later, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: cfg.ServerPort})
	if err != nil {
		t.Fatal(err)
	}
	defer later.Close()
	later.SetDeadline(time.Now().Add(3 * time.Second))
	peer, err := later.Accept()
	if err != nil {
		t.Fatalf("client never retried: %v", err)
	}
	defer peer.Close()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	msg, err := protocol.NewDecoder(peer).Decode()
	if err != nil || msg.Header.Type != protocol.TypeRegister {
		t.Fatalf("expected registration after retry: %v", err)
	}
}

func wireMessage(kind byte, body string) *protocol.Message {
	return &protocol.Message{Header: &protocol.MessageHeader{Magic: protocol.MagicNumber, Version: 1, Type: kind, Length: int32(len(body))}, Body: []byte(body)}
}

func registeredFixture(t *testing.T, cfg *Config) (*Client, net.Conn) {
	t.Helper()
	l := testListener(t)
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	cfg.AutoReconnect = false
	c := NewClient(cfg)
	registered := make(chan struct{}, 4)
	c.OnRegisterResult = func(success bool, _ int, _ error) {
		if success {
			registered <- struct{}{}
		}
	}
	t.Cleanup(c.Stop)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	peer, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })
	peer.SetReadDeadline(time.Now().Add(time.Second))
	msg, err := protocol.NewDecoder(peer).Decode()
	if err != nil || msg.Header.Type != protocol.TypeRegister {
		t.Fatalf("registration: %v", err)
	}
	if err := protocol.NewEncoder(peer).Encode(wireMessage(protocol.TypeRegisterAck, `{"success":true,"externalPort":9001}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("registration not processed")
	}
	peer.SetReadDeadline(time.Time{})
	return c, peer
}

func TestRegistrationDeadlineIsNotExtendedByOtherMessages(t *testing.T) {
	l := testListener(t)
	cfg := stabilityConfig(t)
	cfg.AutoReconnect = false
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	cfg.RegisterTimeout = 150 * time.Millisecond
	cfg.ReadTimeout = time.Second
	c := NewClient(cfg)
	defer c.Stop()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	peer, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		enc := protocol.NewEncoder(peer)
		for range tick.C {
			if err := enc.Encode(wireMessage(protocol.TypeHeartbeatAck, `{}`)); err != nil {
				return
			}
		}
	}()
	peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	dec := protocol.NewDecoder(peer)
	if _, err := dec.Decode(); err != nil {
		t.Fatal(err)
	} // 初始注册
	_, err = dec.Decode()
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("registration stayed open despite missing ACK")
	}
	if err == nil {
		t.Fatal("unexpected frame before registration timeout")
	}
	peer.Close()
	<-sent
	if c.GetExternalPort() != 0 {
		t.Fatal("unregistered connection published a port")
	}
}

func TestRemoteCloseDoesNotEchoAndLocalEOFSendsOneClose(t *testing.T) {
	for _, remote := range []bool{true, false} {
		t.Run(map[bool]string{true: "remote", false: "local"}[remote], func(t *testing.T) {
			local := testListener(t)
			cfg := stabilityConfig(t)
			cfg.LocalPort = local.Addr().(*net.TCPAddr).Port
			cfg.ReadTimeout = 3 * time.Second
			cfg.HeartbeatInterval = 30
			c, peer := registeredFixture(t, cfg)
			enc := protocol.NewEncoder(peer)
			if err := enc.Encode(protocol.NewDataMessageWithConnectionID("one", []byte("x"))); err != nil {
				t.Fatal(err)
			}
			local.SetDeadline(time.Now().Add(time.Second))
			service, err := local.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			service.SetReadDeadline(time.Now().Add(time.Second))
			if _, err = io.ReadFull(service, make([]byte, 1)); err != nil {
				t.Fatal(err)
			}
			if remote {
				if err = enc.Encode(protocol.NewCloseConnectionMessage("one")); err != nil {
					t.Fatal(err)
				}
				if _, err = service.Read(make([]byte, 1)); err != io.EOF {
					t.Fatalf("remote close did not produce EOF: %v", err)
				}
			} else {
				service.Close()
				peer.SetReadDeadline(time.Now().Add(time.Second))
				msg, err := protocol.NewDecoder(peer).Decode()
				if err != nil || msg.Header.Type != protocol.TypeCloseConnection {
					t.Fatalf("local EOF did not notify peer: %v", err)
				}
				packet, err := protocol.ParseDataPacket(msg.Body)
				if err != nil || packet.ConnectionID != "one" {
					t.Fatalf("wrong close target: %v", err)
				}
			}
			peer.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			if _, err = protocol.NewDecoder(peer).Decode(); err == nil {
				t.Fatal("duplicate or reflected close notification")
			}
			if c.GetState() != StateRegistered {
				t.Fatal("one local close broke control connection")
			}
		})
	}
}

func TestStopCancelsBlockedWriteAndConcurrentStops(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.WriteTimeout = time.Minute
	c := NewClient(cfg)
	local, peer := net.Pipe()
	defer peer.Close()
	c.conn, c.reader, c.writer = local, bufio.NewReader(local), bufio.NewWriter(local)
	entered := make(chan struct{})
	c.launch(func() { close(entered); _ = c.SendData(make([]byte, 8192)) })
	<-entered
	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func() { c.Stop(); done <- struct{}{} }()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Stop did not cancel blocked write")
		}
	}
	if c.GetState() != StateDisconnected {
		t.Fatal("Stop left client active")
	}
	if err := c.Start(); err == nil {
		t.Fatal("stopped instance restarted")
	}
}

// 已排队的重连任务在 Stop 后恢复执行时，不能再创建一个未被 Stop 关闭的 manager。
func TestStoppedClientCannotRecreateWebRTCManager(t *testing.T) {
	rtc := clientwebrtc.DefaultConfig()
	rtc.ICEServers = nil
	c := NewClientWithWebRTC(stabilityConfig(t), "stop-reconnect", rtc)
	original := c.webrtcManager
	c.Stop()
	c.prepareWebRTCReconnect()
	current := c.webrtcManager
	// 失败时也关闭新建的真实 Manager，避免回归测试自身泄漏资源。
	defer current.Close()
	if current != original || current.State() != clientwebrtc.StateClosed {
		t.Fatal("reconnect work created a live WebRTC manager after Stop")
	}
}

// 心跳线程断开连接后，已经解码但尚未处理的注册 ACK 不能恢复离线端口和状态。
func TestLateRegisterAckCannotRestoreDisconnectedState(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.ReadTimeout = 3 * time.Second
	cfg.HeartbeatInterval = 30
	c, _ := registeredFixture(t, cfg)
	c.closeConn()
	c.handleRegisterAck(wireMessage(protocol.TypeRegisterAck, `{"success":true,"externalPort":9002}`))
	if c.GetExternalPort() != 0 || c.GetState() != StateDisconnected {
		t.Fatal("late registration ACK restored state after disconnect")
	}
	// ACK 更新后断线、然后才发布 Registered 状态也是同一个竞态窗口。
	c.setState(StateRegistered)
	if c.GetState() != StateDisconnected {
		t.Fatal("stale state update marked closed control registered")
	}
}

// 丢弃全局关闭通知会让服务端永久残留隧道；过载必须显式中断控制连接统一回收。
func TestCloseNotificationOverloadClosesControl(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.ReadTimeout = 3 * time.Second
	cfg.HeartbeatInterval = 30
	c, peer := registeredFixture(t, cfg)
	control := c.currentConn()
	// 写锁模拟其他有效发送占用控制写入；仍调用真实关闭通知和故障循环。
	c.writeMu.Lock()
	for i := 0; i < 130; i++ {
		c.notifyLocalClose(control, "overload")
	}
	peer.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, err := peer.Read(make([]byte, 1))
	c.writeMu.Unlock()
	if err != io.EOF {
		t.Fatalf("overloaded close queue did not invalidate control: %v", err)
	}
}

func waitCondition(t *testing.T, description string, predicate func() bool) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for !predicate() {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal(description)
		}
	}
}

func TestLatencyRequiresRealHeartbeatAckAndClearsOnStop(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.ReadTimeout = 3 * time.Second
	cfg.HeartbeatInterval = 30
	c, peer := registeredFixture(t, cfg)
	if _, ok := c.GetLatency(); ok {
		t.Fatal("latency was invented before any heartbeat")
	}
	if err := c.SendHeartbeat(); err != nil {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	msg, err := protocol.NewDecoder(peer).Decode()
	if err != nil || msg.Header.Type != protocol.TypeHeartbeat {
		t.Fatalf("heartbeat not sent: %v", err)
	}
	// 模拟真实应答处理耗时，断言 RTT 确实包含往返等待。
	timer := time.NewTimer(40 * time.Millisecond)
	<-timer.C
	if err = protocol.NewEncoder(peer).Encode(wireMessage(protocol.TypeHeartbeatAck, `{}`)); err != nil {
		t.Fatal(err)
	}
	waitCondition(t, "heartbeat ACK did not update latency", func() bool { _, ok := c.GetLatency(); return ok })
	elapsed, _ := c.GetLatency()
	if elapsed < 35*time.Millisecond {
		t.Fatalf("heartbeat RTT ignored response delay: %v", elapsed)
	}
	c.Stop()
	if _, ok := c.GetLatency(); ok {
		t.Fatal("disconnected client retained valid latency")
	}
}

func TestHeartbeatAckDeadlineSurvivesOtherControlTraffic(t *testing.T) {
	cfg := stabilityConfig(t)
	cfg.ReadTimeout = time.Second
	cfg.HeartbeatInterval = 30
	cfg.HeartbeatTimeout = 150 * time.Millisecond
	c, peer := registeredFixture(t, cfg)
	if err := c.SendHeartbeat(); err != nil {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	dec := protocol.NewDecoder(peer)
	msg, err := dec.Decode()
	if err != nil || msg.Header.Type != protocol.TypeHeartbeat {
		t.Fatalf("heartbeat not sent: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		enc := protocol.NewEncoder(peer)
		for range tick.C {
			if enc.Encode(wireMessage(protocol.TypeDeviceQueryAck, `{"found":false}`)) != nil {
				return
			}
		}
	}()
	_, err = dec.Decode()
	if err == nil {
		t.Fatal("unexpected outgoing control frame")
	}
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("other traffic masked missing heartbeat ACK")
	}
	peer.Close()
	<-done
}

func TestLocalQueueByteLimitClosesOnlyAffectedConnection(t *testing.T) {
	local := testListener(t)
	cfg := stabilityConfig(t)
	cfg.LocalPort = local.Addr().(*net.TCPAddr).Port
	cfg.ReadTimeout = 3 * time.Second
	cfg.HeartbeatInterval = 30
	cfg.LocalQueueBytes = 4
	c, peer := registeredFixture(t, cfg)
	enc := protocol.NewEncoder(peer)
	services := make([]net.Conn, 0, 2)
	for _, id := range []string{"slow", "healthy"} {
		if err := enc.Encode(protocol.NewDataMessageWithConnectionID(id, []byte("x"))); err != nil {
			t.Fatal(err)
		}
		local.SetDeadline(time.Now().Add(time.Second))
		service, err := local.Accept()
		if err != nil {
			t.Fatal(err)
		}
		defer service.Close()
		service.SetReadDeadline(time.Now().Add(time.Second))
		if _, err = io.ReadFull(service, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
		services = append(services, service)
	}
	if err := enc.Encode(protocol.NewDataMessageWithConnectionID("slow", []byte("12345"))); err != nil {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	msg, err := protocol.NewDecoder(peer).Decode()
	if err != nil || msg.Header.Type != protocol.TypeCloseConnection {
		t.Fatalf("overflow did not notify close: %v", err)
	}
	packet, err := protocol.ParseDataPacket(msg.Body)
	if err != nil || packet.ConnectionID != "slow" {
		t.Fatalf("closed wrong connection: %v", err)
	}
	if _, err = services[0].Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("overflow socket was left open: %v", err)
	}
	if err = enc.Encode(protocol.NewDataMessageWithConnectionID("healthy", []byte("ok"))); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 2)
	if _, err = io.ReadFull(services[1], data); err != nil || string(data) != "ok" {
		t.Fatalf("healthy connection interrupted: %q %v", data, err)
	}
	if c.GetState() != StateRegistered {
		t.Fatal("local queue overflow closed control connection")
	}
}

func TestRepeatedRegisterAckUpdatesPortWithoutAnotherOffer(t *testing.T) {
	l := testListener(t)
	cfg := stabilityConfig(t)
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	cfg.AutoReconnect = false
	cfg.ReadTimeout = 3 * time.Second
	cfg.HeartbeatInterval = 30
	rtc := clientwebrtc.DefaultConfig()
	rtc.ICEServers = nil
	c := NewClientWithWebRTC(cfg, "repeat-ack", rtc)
	ports := make(chan int, 4)
	c.OnRegisterResult = func(success bool, port int, _ error) {
		if success {
			ports <- port
		}
	}
	defer c.Stop()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	peer, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	dec := protocol.NewDecoder(peer)
	if _, err = dec.Decode(); err != nil {
		t.Fatal(err)
	}
	enc := protocol.NewEncoder(peer)
	if err = enc.Encode(wireMessage(protocol.TypeRegisterAck, `{"success":true,"externalPort":9001}`)); err != nil {
		t.Fatal(err)
	}
	for {
		msg, err := dec.Decode()
		if err != nil {
			t.Fatalf("first offer missing: %v", err)
		}
		if msg.Header.Type == protocol.TypeWebRTCOffer {
			break
		}
	}
	select {
	case <-ports:
	case <-time.After(time.Second):
		t.Fatal("first register callback missing")
	}
	if err = enc.Encode(wireMessage(protocol.TypeRegisterAck, `{"success":true,"externalPort":9002}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case port := <-ports:
		if port != 9002 {
			t.Fatal("port update callback was stale")
		}
	case <-time.After(time.Second):
		t.Fatal("port update callback missing")
	}
	peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for {
		msg, err := dec.Decode()
		if err != nil {
			break
		}
		if msg.Header.Type == protocol.TypeWebRTCOffer {
			t.Fatal("port update initiated another WebRTC offer")
		}
	}
	if c.GetExternalPort() != 9002 || c.GetState() != StateRegistered {
		t.Fatal("port update did not preserve registered state")
	}
}

// 对端接受注册但既不响应也不关闭时，读期限必须启动真正的新 TCP 连接。
func TestSilentControlConnectionReconnects(t *testing.T) {
	l := testListener(t)
	cfg := stabilityConfig(t)
	cfg.ServerPort = l.Addr().(*net.TCPAddr).Port
	c := NewClient(cfg)
	defer c.Stop()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	peer, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	l.SetDeadline(time.Now().Add(3 * time.Second))
	replacement, err := l.Accept()
	if err != nil {
		t.Fatalf("silent control connection was never replaced: %v", err)
	}
	replacement.Close()
}

// net.Pipe 的对端不读取；真实心跳 Flush 必须在写期限内失败且使控制连接失效。
func TestHeartbeatWriteHasDeadline(t *testing.T) {
	c := NewClient(stabilityConfig(t))
	local, peer := net.Pipe()
	c.conn, c.reader, c.writer = local, bufio.NewReader(local), bufio.NewWriter(local)
	defer c.Stop()
	defer peer.Close()
	done := make(chan error, 1)
	go func() { done <- c.SendHeartbeat() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked heartbeat unexpectedly succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		peer.Close()
		<-done
		t.Fatal("heartbeat Flush ignored configured write deadline")
	}
	c.mu.Lock()
	active := c.conn
	c.mu.Unlock()
	if active != nil {
		t.Fatal("failed heartbeat left connection active")
	}
}

// 单条本地连接写阻塞时，控制循环仍应处理紧随其后的查询响应。
func TestSlowLocalServiceDoesNotBlockControlMessages(t *testing.T) {
	control := testListener(t)
	local := testListener(t)
	cfg := stabilityConfig(t)
	cfg.AutoReconnect = false
	cfg.ServerPort = control.Addr().(*net.TCPAddr).Port
	cfg.LocalPort = local.Addr().(*net.TCPAddr).Port
	c := NewClient(cfg)
	received := make(chan struct{}, 1)
	c.OnDeviceQueryResult = func(*protocol.DeviceQueryResponse) { received <- struct{}{} }
	defer c.Stop()
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	peer, err := control.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	enc := protocol.NewEncoder(peer)
	if err := enc.Encode(protocol.NewDataMessageWithConnectionID("slow", []byte("x"))); err != nil {
		t.Fatal(err)
	}
	local.SetDeadline(time.Now().Add(time.Second))
	service, err := local.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.SetReadBuffer(1024)
	go func() {
		for i := 0; i < 4; i++ {
			if err := enc.Encode(protocol.NewDataMessageWithConnectionID("slow", make([]byte, 8*1024*1024))); err != nil {
				return
			}
		}
		_ = enc.Encode(&protocol.Message{Header: &protocol.MessageHeader{Magic: protocol.MagicNumber, Version: 1, Type: protocol.TypeDeviceQueryAck, Length: 15}, Body: []byte(`{"found":false}`)})
	}()
	select {
	case <-received:
	case <-time.After(800 * time.Millisecond):
		service.Close()
		t.Fatal("local Write blocked the control reader")
	}
}

// 远端 type 7 应立即关闭本地 socket，不能当作未知消息忽略。
func TestRemoteCloseClosesLocalSocket(t *testing.T) {
	c := NewClient(stabilityConfig(t))
	local, peer := net.Pipe()
	c.localConnections["closed"] = &connectionConn{conn: local, closeCh: make(chan struct{})}
	defer c.Stop()
	defer peer.Close()
	c.handleMessage(protocol.NewCloseConnectionMessage("closed"))
	peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err := peer.Read(make([]byte, 1))
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("remote close did not close local socket")
	}
}
