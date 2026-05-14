# outView 1.2.0 - WebRTC P2P 直连设计文档

**版本**: 1.2.0  
**日期**: 2026-05-14  
**状态**: 设计阶段

## 目标

将 outView 从纯服务器中继架构升级为支持 P2P 直连的混合架构，实现：
- 更低延迟（P2P 直连时 < 50ms）
- 更高稳定性（自动降级保障）
- 减少服务器带宽压力
- 保持用户体验不变（MSTSC 直接连接）

## 整体架构

### 连接建立流程

```
MSTSC 用户                 Java 服务器              Go 客户端              本地 RDP
    |                          |                        |                      |
    |---(1) TCP 连接到数据端口-->|                        |                      |
    |                          |---(2) 信令: 请求 P2P--->|                      |
    |                          |<--(3) 信令: SDP Offer--|                      |
    |                          |---(4) 信令: SDP Answer->|                      |
    |                          |<--(5) 信令: ICE 候选---|                      |
    |                          |---(6) 信令: ICE 候选--->|                      |
    |                          |                        |                      |
    |                          |<===(7) WebRTC DataChannel 建立 ===>|          |
    |                          |                        |                      |
    |<--(8) 如果 P2P 成功，通过 DataChannel 传输 RDP 数据-->|---(9) 转发--->|
    |                          |                        |                      |
    |   (10) 如果 P2P 失败（15秒超时），降级到 TCP 中继   |                      |
    |<===================== TCP 中继 RDP 数据 =======================>|
```

### 关键决策

1. **信令通道**: 复用现有 Netty 控制通道（端口 7000）
2. **P2P 超时**: ICE 协商超时 15 秒，超时后自动降级
3. **TURN 服务器**: 初期使用公共 TURN（Google），后续可选自建
4. **传输协议**: WebRTC DataChannel（UDP + SCTP 可靠传输）
5. **降级策略**: P2P 失败时无缝切换到 TCP 中继

## Go 客户端设计

### 新增组件

**1. WebRTC Manager** (`internal/webrtc/manager.go`)

```go
type Manager struct {
    peerConnection *webrtc.PeerConnection
    dataChannel    *webrtc.DataChannel
    config         *webrtc.Configuration
    onDataReceived func([]byte)
    state          ConnectionState
}

// 核心方法
- CreateOffer() (SDP, error)           // 创建 SDP offer
- SetRemoteAnswer(SDP) error           // 设置远端 answer
- AddICECandidate(candidate) error     // 添加 ICE 候选
- SendData([]byte) error               // 通过 DataChannel 发送数据
- Close() error                        // 关闭连接
```

**2. ICE Candidate Handler** (`internal/webrtc/ice.go`)
- 监听本地 ICE 候选生成事件
- 通过信令通道发送给服务器
- 接收远端 ICE 候选并添加到 PeerConnection

**3. 连接状态机** (`internal/client/connection_state.go`)

```go
const (
    StateIdle              // 空闲
    StateNegotiating       // WebRTC 协商中
    StateP2PConnected      // P2P 已连接
    StateP2PFailed         // P2P 失败，降级中
    StateTCPRelay          // TCP 中继模式
)
```

**4. 修改现有 client.go**
- 收到数据端口分配通知后，启动 WebRTC 协商
- 15 秒超时计时器
- 根据状态选择发送路径（DataChannel 或 TCP）

### 依赖库

```go
require (
    github.com/pion/webrtc/v3 v3.2.24
    github.com/pion/ice/v2 v2.3.11
)
```

### 配置扩展

```go
type Config struct {
    // ... 现有字段 ...
    
    // WebRTC 配置
    EnableP2P         bool     // 是否启用 P2P（默认 true）
    STUNServers       []string // STUN 服务器列表
    TURNServers       []string // TURN 服务器列表（可选）
    P2PTimeout        int      // P2P 协商超时（秒，默认 15）
    PreferP2P         bool     // 优先使用 P2P（默认 true）
}
```

## Java 服务端设计

### 技术选型

使用 **webrtc-java** (dev.onvoid.webrtc:webrtc-java:0.8.0)
- 基于 Google WebRTC native 库的 JNI 封装
- 支持完整的 WebRTC API
- 纯 Java 接口，易于集成

### 新增组件

**1. WebRTC Manager** (`com.outview.webrtc.WebRTCManager`)

```java
@Component
public class WebRTCManager {
    private final Map<String, PeerConnectionWrapper> connections;
    private final PeerConnectionFactory factory;
    
    // 为每个用户连接创建 PeerConnection
    PeerConnectionWrapper createPeerConnection(String connectionId);
    
    // 处理来自客户端的 SDP offer
    void handleOffer(String connectionId, String sdp);
    
    // 处理 ICE candidate
    void handleIceCandidate(String connectionId, IceCandidate candidate);
    
    // 发送数据到客户端
    void sendData(String connectionId, byte[] data);
}
```

**2. PeerConnection 包装器** (`com.outview.webrtc.PeerConnectionWrapper`)

```java
class PeerConnectionWrapper {
    private final PeerConnection pc;
    private final DataChannel dataChannel;
    private final String connectionId;
    private volatile boolean p2pEstablished = false;
    
    // 状态回调
    void onDataChannelOpen();
    void onDataChannelMessage(byte[] data);
    void onIceCandidate(IceCandidate candidate);
    void onConnectionStateChange(PeerConnectionState state);
}
```

**3. 信令处理器** (`com.outview.netty.handler.WebRTCSignalingHandler`)
- 处理 TYPE_WEBRTC_OFFER/ANSWER/ICE_CANDIDATE 消息
- 调用 WebRTCManager 进行 WebRTC 操作
- 将 ICE 候选和 SDP answer 发送回客户端

**4. 混合传输管理器** (`com.outview.service.HybridTransportService`)

```java
@Service
public class HybridTransportService {
    // 决定使用哪个传输通道
    TransportChannel selectChannel(String connectionId);
    
    // P2P 超时后切换到 TCP
    void fallbackToTCP(String connectionId);
    
    // 发送数据（自动选择通道）
    void sendData(String connectionId, byte[] data);
}
```

### Maven 依赖

```xml
<dependency>
    <groupId>dev.onvoid.webrtc</groupId>
    <artifactId>webrtc-java</artifactId>
    <version>0.8.0</version>
</dependency>
```

### 配置扩展

```yaml
outview:
  webrtc:
    enabled: true
    ice-servers:
      - urls: stun:stun.l.google.com:19302
      - urls: stun:stun1.l.google.com:19302
    p2p-timeout: 15000  # 毫秒
    prefer-p2p: true
```

## 协议扩展

### 新增消息类型

```java
// ProtocolConstants.java
public static final byte TYPE_WEBRTC_OFFER = 8;
public static final byte TYPE_WEBRTC_ANSWER = 9;
public static final byte TYPE_WEBRTC_ICE_CANDIDATE = 10;
public static final byte TYPE_P2P_ESTABLISHED = 11;
public static final byte TYPE_P2P_FAILED = 12;
```

### 消息体格式

**WEBRTC_OFFER / ANSWER**
```json
{
  "connectionId": "uuid-1234",
  "sdp": "v=0\r\no=- 123456 2 IN IP4...",
  "type": "offer"
}
```

**ICE_CANDIDATE**
```json
{
  "connectionId": "uuid-1234",
  "candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54321 typ host",
  "sdpMid": "0",
  "sdpMLineIndex": 0
}
```

**P2P_ESTABLISHED**
```json
{
  "connectionId": "uuid-1234",
  "latency": 25
}
```

**P2P_FAILED**
```json
{
  "connectionId": "uuid-1234",
  "reason": "ICE negotiation timeout",
  "fallbackToTCP": true
}
```

### 信令流程

1. 用户连接数据端口 → 服务端分配 connectionId
2. 服务端通过控制通道发送 WEBRTC_OFFER 给客户端
3. 客户端创建 PeerConnection，生成 SDP Answer
4. 客户端发送 WEBRTC_ANSWER 给服务端
5. 双方交换 ICE_CANDIDATE（可能多个）
6. ICE 协商成功 → DataChannel 打开
7. 客户端发送 P2P_ESTABLISHED 通知
8. 服务端将用户的 TCP 连接桥接到 DataChannel
9. RDP 数据通过 P2P 传输

如果 15 秒内未建立：
- 服务端发送 P2P_FAILED
- 双方降级到 TCP 中继（现有逻辑）

## GUI 客户端设计

### 界面布局（选项卡式）

**Tab 1: 连接配置**
- 服务器地址、端口
- 设备 ID、Token
- 本地端口
- P2P 开关
- 优先 P2P 选项
- P2P 超时设置

**Tab 2: 高级设置**
- STUN 服务器列表（可添加自定义）
- TURN 服务器配置（可选）
- 自动重连、开机自启、最小化到托盘

**Tab 3: 连接状态**
- 当前连接类型（P2P / TCP）
- 延迟、上传/下载速度
- ICE 状态、候选信息
- 连接时长、传输数据统计

**Tab 4: 日志**
- 实时日志输出
- 清空、导出、自动滚动功能

### 系统托盘

**图标状态**
- 🟢 绿色：P2P 直连
- 🟡 黄色：TCP 中继
- 🔴 红色：连接失败
- ⚪ 灰色：未连接

**右键菜单**
- 显示连接状态和延迟
- 显示/隐藏主窗口
- 断开/重新连接
- 退出

### 新增 UI 组件

1. **连接状态指示器** - 动画圆点显示状态
2. **实时流量图表** - 上传/下载速度曲线
3. **ICE 协商进度条** - 显示 P2P 建立进度

## 错误处理和降级策略

### P2P 失败场景

**场景 1: ICE 协商超时（15秒）**
- 触发：未收到 ICE 候选或连接测试全部失败
- 处理：发送 P2P_FAILED，切换 TCP，GUI 显示降级

**场景 2: DataChannel 建立后断开**
- 触发：P2P 连接中途断开（网络切换、NAT 重映射）
- 处理：立即切换 TCP，后台重试 P2P（最多 3 次）

**场景 3: 对称 NAT 无法打洞**
- 触发：双方都在对称 NAT 后，且无 TURN
- 处理：5 秒快速失败，直接 TCP 中继

### 降级决策树

```
开始连接
    ↓
启用 P2P？
    ├─ 否 → 直接 TCP 中继
    └─ 是 → 启动 ICE 协商
            ↓
        5 秒内有候选？
            ├─ 否 → 快速失败 → TCP 中继
            └─ 是 → 继续协商
                    ↓
                15 秒内建立？
                    ├─ 否 → 超时 → TCP 中继
                    └─ 是 → P2P 成功
                            ↓
                        运行中断开？
                            ├─ 是 → 切换 TCP + 后台重试
                            └─ 否 → 保持 P2P
```

### 错误码

```go
const (
    ErrP2PTimeout           = "P2P_TIMEOUT"
    ErrP2PNoCandidate       = "P2P_NO_CANDIDATE"
    ErrP2PSymmetricNAT      = "P2P_SYMMETRIC_NAT"
    ErrP2PDataChannelFailed = "P2P_DATACHANNEL_FAILED"
    ErrP2PConnectionLost    = "P2P_CONNECTION_LOST"
)
```

### 重试策略

**P2P 重试**
- 初次失败：立即降级，不重试
- 运行中断开：后台重试 3 次（间隔 10s、30s、60s）
- 重试成功：无缝切回 P2P

**TCP 中继保障**
- P2P 失败时，TCP 通道必须已就绪
- 使用现有自动重连机制

## 测试策略

### 单元测试

**Go 客户端**
- WebRTC Manager: CreateOffer, SetRemoteAnswer, AddICECandidate
- 连接状态机: 状态转换、降级、升级

**Java 服务端**
- WebRTCManager: PeerConnection 创建、Offer 处理、ICE 处理
- HybridTransportService: 通道选择、降级、切换

### 集成测试

**场景 1: 正常 P2P 建立**
- 验证 15 秒内 DataChannel 建立
- 验证 RDP 流量走 P2P
- 验证延迟 < 100ms

**场景 2: P2P 失败降级**
- 模拟对称 NAT
- 验证 5 秒快速失败
- 验证自动降级到 TCP

**场景 3: P2P 中途断开**
- 模拟网络中断
- 验证无缝切换到 TCP
- 验证 RDP 会话不中断

**场景 4: 并发连接**
- 10 个客户端同时连接
- 验证混合模式（部分 P2P，部分 TCP）

### 网络环境测试

**NAT 类型矩阵**
```
客户端 NAT  ×  用户 NAT  →  预期结果
─────────────────────────────────────
Full Cone   ×  Full Cone  →  P2P 成功
Full Cone   ×  Symmetric  →  P2P 成功
Symmetric   ×  Symmetric  →  降级 TCP
Port Res.   ×  Port Res.  →  P2P 成功
```

**测试工具**
- Docker 模拟不同 NAT 类型
- iptables 规则模拟网络限制
- tc 命令模拟延迟和丢包

### 性能测试

**目标指标**
- P2P 建立时间: < 5 秒
- 降级切换时间: < 1 秒
- RDP 延迟: P2P < 50ms, TCP < 150ms
- 吞吐量: P2P > 50 Mbps

**压力测试**
- 100 并发连接
- 持续运行 24 小时
- 内存泄漏检测

### GUI 测试

**功能测试**
- 配置保存/加载
- 状态实时更新
- 托盘图标状态
- 日志输出

**兼容性测试**
- Windows 10/11
- macOS 12+
- Linux (Ubuntu 20.04+)

## 实施计划

### 阶段 1: 基础设施（1 周）
- Go 客户端集成 pion/webrtc
- Java 服务端集成 webrtc-java
- 协议扩展（新增消息类型）
- 基础信令流程

### 阶段 2: P2P 核心功能（1 周）
- ICE 协商完整流程
- DataChannel 数据传输
- 超时和降级逻辑
- 状态机实现

### 阶段 3: GUI 增强（3-4 天）
- 新增配置选项卡
- 连接状态显示
- 系统托盘功能
- 实时图表

### 阶段 4: 测试和优化（3-4 天）
- 单元测试
- 集成测试
- 网络环境测试
- 性能优化

**总工期**: 约 3 周

## 风险和缓解

**风险 1: webrtc-java 兼容性问题**
- 缓解：提前验证 POC，准备 JNI 方案

**风险 2: NAT 穿透成功率低**
- 缓解：TCP 降级保障，后续可自建 TURN

**风险 3: GUI 跨平台问题**
- 缓解：Fyne 框架成熟，提前测试三平台

**风险 4: 工期延误**
- 缓解：分阶段交付，核心功能优先

## 总结

1.2.0 版本通过引入 WebRTC P2P 直连，在保持现有稳定性的基础上，显著降低延迟、减少服务器压力。混合架构设计确保了向后兼容和平滑降级，GUI 增强提升了用户体验。
