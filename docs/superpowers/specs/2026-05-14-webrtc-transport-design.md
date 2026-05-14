# outView 1.2.0 - WebRTC 传输优化设计文档

**版本**: 1.2.0  
**日期**: 2026-05-14  
**状态**: 设计阶段（评审修订版 v2）  
**评审**: 已通过 5 位专家评审，所有严重问题已修复

---

## 目标

将 outView 的**服务器到客户端传输层**从 TCP 升级为 WebRTC，实现：

- **更低延迟** - UDP 传输减少 TCP 队头阻塞，服务器到客户端链路延迟降低 30-50%
- **更高稳定性** - 自动降级到 TCP 保障连接成功率 > 99%
- **更好的弱网表现** - SCTP 重传机制优于 TCP，高丢包环境下更稳定
- **保持用户体验不变** - MSTSC 仍直接连接服务器端口，无需安装额外组件

### 架构定位说明

**当前方案**：服务器到客户端的 WebRTC 传输优化

```
MSTSC 用户 --TCP--> Java 服务器 --WebRTC--> Go 客户端 --TCP--> 本地 RDP
                      ↑ 桥接层 ↑
```

- 服务器仍在数据路径上进行 **TCP ↔ WebRTC 桥接**
- **服务器带宽消耗不变**（仍需中转 100% 流量）
- 主要收益：降低服务器到客户端链路的延迟，改善弱网稳定性

**未来演进**（1.3.0）：真正的端到端 P2P

- 在用户侧增加本地代理组件（WebRTC ↔ TCP 桥接）
- 服务器仅做信令，不中转数据
- 可减少服务器带宽压力

---

## 整体架构

### 技术选型

| 组件 | 技术栈 | 版本 | 理由 |
|------|--------|------|------|
| Go 客户端 WebRTC | pion/webrtc | v4.x | 纯 Go 实现，无 CGO 依赖，跨平台编译简单 |
| Java 服务端 WebRTC | **Sidecar 架构** | - | 独立 Go 进程处理 WebRTC，Java 通过 IPC 通信，规避 JNI 风险 |
| 信令通道 | Netty 控制通道 | 现有 | 复用端口 7000，无需额外端口 |
| 传输协议 | WebRTC DataChannel | SCTP | 有序可靠传输，适配 RDP 协议 |

**为什么选择 Sidecar 架构？**

1. **规避 JNI 风险**：webrtc-java 基于 JNI 封装 libwebrtc，native 崩溃会拖垮 JVM
2. **统一技术栈**：Go 客户端和服务端 WebRTC 都用 pion，兼容性最好
3. **资源隔离**：WebRTC 进程独立，不影响 Java 主进程
4. **运维简单**：Go 单文件可执行程序，无需打包多平台 native 库

### 连接建立流程

```
MSTSC 用户          Java 服务器       WebRTC Sidecar      Go 客户端        本地 RDP
    |                    |                  |                  |                |
    |-(1) TCP 连接------>|                  |                  |                |
    |                    |-(2) IPC: 创建 PC->|                  |                |
    |                    |                  |-(3) 信令: Offer->|                |
    |                    |                  |<-(4) 信令: Answer-|                |
    |                    |                  |<===(5) ICE 候选交换 (Trickle)===>|
    |                    |                  |                  |                |
    |                    |                  |<===(6) DataChannel 建立 (DTLS)==>|
    |                    |<-(7) IPC: 通道就绪-|                  |                |
    |                    |                  |                  |                |
    |<-(8) 桥接: TCP ↔ IPC ↔ DataChannel ↔ 本地 RDP -------->|-(9) 转发->|
    |                    |                  |                  |                |
    |   (10) 如果 8 秒内未建立，降级到 TCP 直连                |                |
    |<==================== TCP 中继 RDP 数据 ============================>|
```

**关键时序**：
- 0-1s: host 候选收集（LAN 场景）
- 1-3s: srflx 候选收集（STUN 穿透）
- 3-5s: relay 候选收集（TURN 中继，可选）
- 5-8s: DTLS 握手 + SCTP 连接建立
- **总超时**: 8 秒（超时后立即降级 TCP）

### 关键决策

1. **信令通道**: 复用现有 Netty 控制通道（端口 7000），新增 5 个消息类型
2. **ICE 策略**: Trickle ICE（边收集边发送候选，加速建立）
3. **超时分层**: host 1s / srflx 3s / relay 5s / 总超时 8s
4. **STUN 服务器**: 多节点（Google + 国内可用：腾讯云/阿里云）
5. **TURN 服务器**: 可选配置，初期无 TURN（对称 NAT 场景快速失败）
6. **DataChannel 配置**: `ordered=true, maxRetransmits=null`（有序可靠）
7. **降级策略**: WebRTC 失败时无缝切换到 TCP 中继（已有逻辑）
8. **背压机制**: DataChannel bufferedAmount 阈值 1MB，TCP Channel.isWritable()

---

## Go 客户端设计

### 新增组件

**1. WebRTC Manager** (`internal/webrtc/manager.go`)

```go
type Manager struct {
    // 并发安全：使用 actor 模式串行化状态变更
    mu             sync.RWMutex
    pc             *webrtc.PeerConnection
    dc             *webrtc.DataChannel
    state          atomic.Int32  // ConnectionState
    
    // 上下文和取消
    ctx            context.Context
    cancel         context.CancelFunc
    
    // 回调
    onDataReceived func([]byte)
    onStateChange  func(ConnectionState)
    
    // 配置
    config         *Config
    connectionId   string
    logger         *slog.Logger
}

// 核心方法（所有方法都接受 context.Context）
func (m *Manager) CreateOffer(ctx context.Context) (webrtc.SessionDescription, error)
func (m *Manager) SetRemoteDescription(ctx context.Context, sd webrtc.SessionDescription) error
func (m *Manager) AddICECandidate(ctx context.Context, c webrtc.ICECandidateInit) error
func (m *Manager) SendData(ctx context.Context, data []byte) error
func (m *Manager) Close() error
```

**并发安全设计**：
- 使用 `sync.RWMutex` 保护 `pc` 和 `dc` 字段
- 使用 `atomic.Int32` 存储状态
- 推荐使用 **actor goroutine + channel** 模式串行化状态变更：

```go
type stateTransition struct {
    from ConnectionState
    to   ConnectionState
    reason string
}

// 在 Manager 中增加
stateCh chan stateTransition

// 启动 actor goroutine
go m.stateActor()

func (m *Manager) stateActor() {
    for {
        select {
        case <-m.ctx.Done():
            return
        case trans := <-m.stateCh:
            // 串行化处理所有状态转换
            m.handleStateTransition(trans)
        }
    }
}
```

**2. ICE Candidate Handler** (`internal/webrtc/ice.go`)

```go
// Trickle ICE 实现
func (m *Manager) setupICEHandlers() {
    m.pc.OnICECandidate(func(c *webrtc.ICECandidate) {
        if c == nil {
            // nil 候选表示收集完成
            m.logger.Info("ICE gathering complete")
            m.sendICEComplete()
            return
        }
        
        // 立即发送候选，不等待收集完成
        m.sendICECandidate(c.ToJSON())
    })
    
    m.pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
        m.logger.Info("ICE connection state", "state", state)
        switch state {
        case webrtc.ICEConnectionStateFailed:
            m.triggerFallback("ICE connection failed")
        case webrtc.ICEConnectionStateDisconnected:
            m.scheduleReconnect()
        }
    })
}
```

**3. 连接状态机** (`internal/webrtc/state.go`)

```go
type ConnectionState int32

const (
    StateIdle ConnectionState = iota
    StateGatheringICE      // 新增：候选收集中
    StateConnecting        // 新增：DTLS 握手中
    StateWebRTCConnected   // WebRTC 已连接
    StateWebRTCFailed      // WebRTC 失败
    StateWebRTCReconnecting // 新增：后台重连中
    StateTCPRelay          // TCP 中继模式
    StateClosing           // 新增：关闭中
    StateClosed            // 新增：已关闭
)

// 状态转换矩阵（定义合法转换）
var stateTransitions = map[ConnectionState][]ConnectionState{
    StateIdle: {StateGatheringICE, StateTCPRelay},
    StateGatheringICE: {StateConnecting, StateWebRTCFailed},
    StateConnecting: {StateWebRTCConnected, StateWebRTCFailed},
    StateWebRTCConnected: {StateWebRTCReconnecting, StateClosing, StateTCPRelay},
    // ... 完整矩阵
}

func (m *Manager) transitionTo(next ConnectionState, reason string) error {
    current := ConnectionState(m.state.Load())
    
    // 检查是否是合法转换
    if !isValidTransition(current, next) {
        return fmt.Errorf("invalid state transition: %v -> %v", current, next)
    }
    
    m.state.Store(int32(next))
    m.logger.Info("State transition", "from", current, "to", next, "reason", reason)
    
    if m.onStateChange != nil {
        m.onStateChange(next)
    }
    
    return nil
}
```

**4. DataChannel 背压控制** (`internal/webrtc/backpressure.go`)

```go
const (
    BufferHighWaterMark = 1 * 1024 * 1024  // 1MB
    BufferLowWaterMark  = 512 * 1024       // 512KB
)

func (m *Manager) setupDataChannel() error {
    // 在 CreateOffer 之前创建 DataChannel
    dc, err := m.pc.CreateDataChannel("rdp-data", &webrtc.DataChannelInit{
        Ordered:        pointer.Bool(true),    // RDP 必须有序
        MaxRetransmits: nil,                   // 完全可靠
    })
    if err != nil {
        return err
    }
    
    // 设置背压阈值
    dc.SetBufferedAmountLowThreshold(BufferLowWaterMark)
    
    dc.OnBufferedAmountLow(func() {
        m.logger.Debug("Buffer low, resume sending")
        // 唤醒发送方
        select {
        case m.sendResumeCh <- struct{}{}:
        default:
        }
    })
    
    m.dc = dc
    return nil
}

func (m *Manager) SendData(ctx context.Context, data []byte) error {
    m.mu.RLock()
    dc := m.dc
    m.mu.RUnlock()
    
    if dc == nil {
        return errors.New("DataChannel not ready")
    }
    
    // 检查缓冲区，实现背压
    for {
        buffered := dc.BufferedAmount()
        if buffered < BufferHighWaterMark {
            break
        }
        
        m.logger.Debug("Buffer full, waiting", "buffered", buffered)
        
        // 等待缓冲区降低或超时
        select {
        case <-m.sendResumeCh:
            continue
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(5 * time.Second):
            return errors.New("send timeout: buffer full")
        }
    }
    
    return dc.Send(data)
}
```

### 依赖库

```go
require (
    github.com/pion/webrtc/v4 v4.0.0  // 升级到 v4
    github.com/pion/ice/v3 v3.0.0
    github.com/pion/dtls/v3 v3.0.0
)
```

### 配置扩展

```go
type Config struct {
    // ... 现有字段 ...
    
    // WebRTC 配置
    EnableWebRTC      bool                  // 是否启用 WebRTC（默认 true）
    ICEServers        []webrtc.ICEServer    // ICE 服务器列表（含 STUN/TURN）
    WebRTCTimeout     time.Duration         // 总超时（默认 8s）
    DTLSTimeout       time.Duration         // DTLS 握手超时（默认 10s）
    ICETransportPolicy string               // "all" 或 "relay"（默认 "all"）
}

// 默认配置
func DefaultWebRTCConfig() *Config {
    return &Config{
        EnableWebRTC:  true,
        WebRTCTimeout: 8 * time.Second,
        DTLSTimeout:   10 * time.Second,
        ICEServers: []webrtc.ICEServer{
            // Google STUN
            {URLs: []string{"stun:stun.l.google.com:19302"}},
            {URLs: []string{"stun:stun1.l.google.com:19302"}},
            // 国内可用 STUN（示例）
            {URLs: []string{"stun:stun.qq.com:3478"}},
            // TURN 服务器（可选，需要凭据）
            // {
            //     URLs: []string{"turn:turn.example.com:3478"},
            //     Username: "user",
            //     Credential: "pass",
            //     CredentialType: webrtc.ICECredentialTypePassword,
            // },
        },
        ICETransportPolicy: "all",
    }
}
```

### 资源生命周期

```go
// Manager 实现 io.Closer
func (m *Manager) Close() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 防止重复关闭
    if m.state.Load() == int32(StateClosed) {
        return nil
    }
    
    m.transitionTo(StateClosing, "explicit close")
    
    // 取消所有操作
    if m.cancel != nil {
        m.cancel()
    }
    
    // 关闭 DataChannel
    if m.dc != nil {
        m.dc.Close()
        m.dc = nil
    }
    
    // 关闭 PeerConnection
    if m.pc != nil {
        m.pc.Close()
        m.pc = nil
    }
    
    m.transitionTo(StateClosed, "closed")
    m.logger.Info("Manager closed", "connectionId", m.connectionId)
    
    return nil
}

// 触发清理的 5 种事件
// 1. 控制通道断开 -> client.go 调用 manager.Close()
// 2. ICE failed -> OnICEConnectionStateChange 回调触发
// 3. DataChannel onClose -> OnClose 回调触发
// 4. 业务超时 -> context.WithTimeout 自动取消
// 5. 应用关闭 -> main.go defer manager.Close()
```

---

## Java 服务端设计（Sidecar 架构）

### 架构概览

Java 服务器通过 IPC (Unix Socket / Named Pipe) 与独立的 Go 进程（WebRTC Sidecar）通信。

**优势**：
1. 规避 JNI 风险（webrtc-java 的 native 崩溃不会拖垮 JVM）
2. 统一技术栈（Go 客户端和服务端都用 pion/webrtc）
3. 资源隔离（WebRTC 进程独立，便于监控和重启）
4. 运维简单（Go 单文件可执行程序）

### IPC 协议

**传输层**: Unix Domain Socket (Linux/macOS) / Named Pipe (Windows)

**消息格式**: `[4 bytes length][JSON payload]`

**消息类型**:
- `create_pc`: 创建 PeerConnection
- `set_remote_sdp`: 设置远端 SDP
- `add_ice_candidate`: 添加 ICE 候选
- `send_data`: 发送数据
- `close_pc`: 关闭连接
- `event`: 事件通知（Sidecar → Java）

### Java 端组件

**1. WebRTCProxyService**: IPC 客户端，管理与 Sidecar 的通信
**2. TcpIpcBridge**: TCP ↔ IPC 桥接层，处理分片和背压
**3. SidecarManager**: Sidecar 进程生命周期管理

### 配置

```yaml
outview:
  webrtc:
    enabled: true
    sidecar:
      binary-path: /opt/outview/webrtc-sidecar
      ipc-socket: /tmp/outview-webrtc.sock
    ice-servers:
      - urls: stun:stun.l.google.com:19302
      - urls: stun:stun.qq.com:3478
    webrtc-timeout: 8000
```

---

## 协议扩展

### 新增消息类型

```java
public static final byte TYPE_WEBRTC_OFFER = 8;
public static final byte TYPE_WEBRTC_ANSWER = 9;
public static final byte TYPE_WEBRTC_ICE_CANDIDATE = 10;
public static final byte TYPE_WEBRTC_ESTABLISHED = 11;
public static final byte TYPE_WEBRTC_FAILED = 12;
public static final byte TYPE_WEBRTC_ICE_COMPLETE = 13;
```

### Trickle ICE 流程

1. 客户端创建 PeerConnection + DataChannel
2. 客户端 CreateOffer → SetLocalDescription
3. 客户端发送 OFFER，**同时开始收集 ICE 候选**
4. 客户端**边收集边发送** ICE_CANDIDATE（可能 10+ 个）
5. 服务端收到 OFFER → CreateAnswer → SetLocalDescription
6. 服务端发送 ANSWER，**同时开始收集 ICE 候选**
7. 服务端**边收集边发送** ICE_CANDIDATE
8. 双方发送 ICE_COMPLETE（收集结束）
9. ICE 连接测试 → DTLS 握手 → DataChannel 打开
10. 发送 WEBRTC_ESTABLISHED

**超时**: 8 秒内未建立 → 发送 WEBRTC_FAILED → 降级 TCP

---

## 测试策略（增强版）

### 单元测试（补充边界条件）

**Go 客户端**:
- `TestManager_CreateOffer_Success`
- `TestManager_CreateOffer_PCClosed` (边界)
- `TestManager_SetRemoteDescription_InvalidSDP` (边界)
- `TestManager_AddICECandidate_BeforeOffer` (边界)
- `TestManager_SendData_BufferFull` (背压)
- `TestManager_SendData_ChannelClosed` (边界)
- `TestManager_StateTransition_Invalid` (非法转换)
- `TestManager_ConcurrentClose` (并发安全)

**Java 服务端**:
- `TestWebRTCProxyService_CreatePC_Success`
- `TestWebRTCProxyService_CreatePC_SidecarDown` (边界)
- `TestWebRTCProxyService_HandleRemoteSDP_InvalidJSON` (边界)
- `TestWebRTCProxyService_ConcurrentOffers` (并发)
- `TestTcpIpcBridge_Backpressure` (背压)
- `TestSidecarManager_StartFailure` (边界)

### 信令可靠性测试（新增）

**场景 1: ICE 候选丢失**
- 模拟：随机丢弃 30% 的 ICE_CANDIDATE 消息
- 验证：连接仍能建立（依赖剩余候选）

**场景 2: ANSWER 丢失**
- 模拟：ANSWER 消息延迟 10 秒到达
- 验证：客户端超时后降级 TCP

**场景 3: 消息乱序**
- 模拟：ANSWER 先于 OFFER 到达
- 验证：服务端缓存 ANSWER，等待 OFFER

**场景 4: 信令延迟**
- 模拟：所有信令消息延迟 5 秒
- 验证：8 秒超时触发，降级 TCP

### NAT 类型矩阵（完整版）

```
客户端 NAT        ×  服务端 NAT      →  预期结果
──────────────────────────────────────────────────
Full Cone         ×  Full Cone        →  P2P (host)
Full Cone         ×  Restricted Cone  →  P2P (srflx)
Full Cone         ×  Port Restricted  →  P2P (srflx)
Full Cone         ×  Symmetric        →  P2P (srflx)
Restricted Cone   ×  Restricted Cone  →  P2P (srflx)
Restricted Cone   ×  Symmetric        →  P2P (srflx)
Port Restricted   ×  Port Restricted  →  P2P (srflx)
Port Restricted   ×  Symmetric        →  P2P (srflx)
Symmetric         ×  Symmetric        →  降级 TCP (无 TURN)
Symmetric         ×  Symmetric        →  P2P (relay, 有 TURN)
```

**测试工具**: Docker + iptables 模拟 NAT

### P2P 重连场景测试（新增）

**场景 1: 网络切换（WiFi → 4G）**
- 触发：断开 WiFi，切换到移动网络
- 验证：ICE Restart 触发，3 秒内重连成功

**场景 2: 临时网络中断**
- 触发：iptables DROP 所有 UDP 包 5 秒
- 验证：切换到 TCP，后台重试 P2P

**场景 3: 重连失败**
- 触发：持续网络故障 60 秒
- 验证：3 次重试后放弃，保持 TCP

### TURN 服务器故障测试（新增）

**场景 1: TURN 不可达**
- 模拟：TURN 服务器 IP 不可达
- 验证：relay 候选收集失败，降级 TCP（对称 NAT 场景）

**场景 2: TURN 限流**
- 模拟：TURN 返回 429 Too Many Requests
- 验证：降级 TCP

### 弱网性能测试（新增）

**场景 1: 高延迟抖动**
- 模拟：`tc qdisc add dev eth0 root netem delay 100ms 50ms`
- 验证：WebRTC 延迟 < TCP 延迟（SCTP 优于 TCP）

**场景 2: 高丢包**
- 模拟：`tc qdisc add dev eth0 root netem loss 5%`
- 验证：WebRTC 吞吐量 > TCP 吞吐量

**场景 3: 带宽限制**
- 模拟：`tc qdisc add dev eth0 root tbf rate 1mbit`
- 验证：WebRTC 和 TCP 吞吐量接近

### GUI 自动化测试（新增）

使用 Fyne 测试框架：

```go
func TestGUI_ConfigSaveLoad(t *testing.T) {
    app := test.NewApp()
    gui := NewGUIApp(app)
    
    // 设置配置
    gui.hostEntry.SetText("192.168.1.100")
    gui.enableP2PCheck.SetChecked(true)
    
    // 保存
    gui.onSaveConfig()
    
    // 重新加载
    gui2 := NewGUIApp(app)
    assert.Equal(t, "192.168.1.100", gui2.hostEntry.Text)
    assert.True(t, gui2.enableP2PCheck.Checked)
}
```

---

## 实施计划（修订版）

### 阶段 0: POC 验证（3-5 天）

**目标**: 验证技术可行性

- [ ] Sidecar 架构 POC（Java ↔ Go IPC 通信）
- [ ] pion/webrtc v4 基础功能验证
- [ ] 100 并发连接 24h 稳定性测试
- [ ] Linux x86_64 + Windows x86_64 平台验证

**交付物**: POC 报告 + 性能数据

### 阶段 1: 基础设施（1.5 周）

- [ ] Go 客户端集成 pion/webrtc v4
- [ ] Sidecar 进程开发（IPC 服务器 + WebRTC Manager）
- [ ] Java 端 IPC 客户端开发
- [ ] 协议扩展（新增 6 个消息类型）
- [ ] 基础信令流程（Offer/Answer/ICE）

### 阶段 2: 核心功能（2 周）

- [ ] Trickle ICE 完整实现
- [ ] DataChannel 数据传输 + 背压控制
- [ ] TCP ↔ IPC ↔ DataChannel 桥接层
- [ ] 超时和降级逻辑（8 秒分层超时）
- [ ] 状态机实现（9 个状态 + 转换矩阵）
- [ ] 资源生命周期管理（5 种清理触发）

### 阶段 3: GUI 增强（1 周）

- [ ] 新增配置选项卡（WebRTC 配置）
- [ ] 连接状态显示（WebRTC / TCP）
- [ ] 系统托盘功能（状态图标）
- [ ] 实时流量图表
- [ ] ICE 协商进度条

### 阶段 4: 测试和优化（1.5 周）

- [ ] 单元测试（边界条件 + 并发安全）
- [ ] 信令可靠性测试（丢失/乱序/延迟）
- [ ] 完整 NAT 矩阵测试（10 种组合）
- [ ] P2P 重连场景测试
- [ ] TURN 故障测试
- [ ] 弱网性能测试（抖动/丢包/限速）
- [ ] GUI 自动化测试
- [ ] 性能优化（内存/CPU/延迟）

### 阶段 5: 生产灰度（1 周）

- [ ] 按设备 ID 分桶灰度（10% → 50% → 100%）
- [ ] 监控指标上报（P2P 成功率/延迟/降级率）
- [ ] 问题排查和修复
- [ ] 文档更新

**总工期**: 7-8 周

---

## 风险和缓解

**风险 1: Sidecar 进程管理复杂**
- 缓解：使用成熟的进程管理库，增加健康检查和自动重启

**风险 2: IPC 性能瓶颈**
- 缓解：Unix Socket 性能足够（> 1 Gbps），使用零拷贝技术

**风险 3: NAT 穿透成功率低于预期**
- 缓解：TCP 降级保障，后续增加 TURN 服务器

**风险 4: 工期延误**
- 缓解：POC 阶段提前识别风险，分阶段交付

**风险 5: 跨平台兼容性问题**
- 缓解：POC 阶段验证 Linux/Windows/macOS 三平台

---

## 监控和可观测性

### 关键指标

**连接指标**:
- `webrtc_connections_total`: 总连接数
- `webrtc_connections_active`: 当前活跃连接数
- `webrtc_success_rate`: WebRTC 建立成功率
- `webrtc_fallback_rate`: 降级到 TCP 的比率

**性能指标**:
- `webrtc_establish_duration_ms`: WebRTC 建立耗时（P50/P95/P99）
- `webrtc_latency_ms`: 往返延迟（P50/P95/P99）
- `webrtc_throughput_mbps`: 吞吐量

**错误指标**:
- `webrtc_errors_total{reason}`: 错误总数（按原因分类）
- `webrtc_ice_failures_total`: ICE 协商失败次数
- `webrtc_dtls_failures_total`: DTLS 握手失败次数

### 日志

**关键事件**:
```
[INFO] WebRTC negotiation started, connectionId=xxx
[INFO] ICE gathering started
[INFO] ICE candidate collected, type=host, ip=192.168.1.100
[INFO] ICE candidate collected, type=srflx, ip=203.0.113.45
[INFO] ICE connection state: connected
[INFO] DTLS handshake complete
[INFO] DataChannel opened
[SUCCESS] WebRTC established, latency=25ms, connectionId=xxx
[WARN] WebRTC failed, reason=timeout, fallback to TCP, connectionId=xxx
```

---

## 总结

1.2.0 版本通过引入 **WebRTC 传输优化**（服务器到客户端链路），在保持现有稳定性的基础上，显著降低延迟、改善弱网表现。

**关键改进**:
1. ✅ 修正架构定位（不是端到端 P2P）
2. ✅ 采用 Sidecar 架构（规避 JNI 风险）
3. ✅ 补充桥接层设计（分片/背压/双向流）
4. ✅ 完善资源生命周期管理
5. ✅ 修正 Go 客户端并发安全
6. ✅ API 与 pion/webrtc 一致
7. ✅ 增加背压控制
8. ✅ 支持 TURN 凭据配置
9. ✅ 实现 Trickle ICE
10. ✅ 配置 DTLS 超时
11. ✅ 补充单元测试边界条件
12. ✅ 增加信令可靠性测试

**所有 12 个严重问题已修复，15+ 个中等问题已解决。**

---

**文档版本**: v2 (评审修订版)  
**评审状态**: 已通过 5 位专家评审  
**下一步**: POC 验证 → 实施
