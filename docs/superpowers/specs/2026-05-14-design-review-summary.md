# WebRTC 设计评审总结报告

**评审日期**: 2026-05-14  
**评审团队**: 架构师、Go 专家、Java 专家、网络专家、QA 专家  
**原设计文档**: 2026-05-14-webrtc-p2p-design.md

## 评审结论

**总体评价**: 需要重大修改

设计框架合理，但存在 **12 个严重问题** 和 **15+ 个中等问题**，必须在实施前解决。

---

## 🔴 严重问题（必须修复）

### 1. 架构定位偏差 - "P2P"是误称
**发现者**: 架构师、Java 专家  
**问题**: 
- 当前设计中服务器仍在数据路径上（MSTSC → 服务器 → Go 客户端）
- 服务器进行 TCP ↔ DataChannel 桥接，带宽消耗不变
- 目标章节宣称"减少服务器带宽压力"与架构现实矛盾

**影响**: 交付产品与承诺目标不符，可能引发上层失望

**建议方案**:
1. **修正命名**（推荐）: 改为"WebRTC 传输优化"，删除"减少服务器带宽"目标
2. **真 P2P**: 增加用户侧本地代理组件，实现 MSTSC ↔ Go 客户端直连（服务器仅做信令）
3. **混合**: 提供本地代理作为可选组件，不安装则回退到服务器中继

---

### 2. webrtc-java 选型风险高
**发现者**: 架构师、Java 专家  
**问题**:
- 0.8.0 版本号说明仍是早期阶段，社区采用度低
- JNI 封装 libwebrtc 的运维痛点：
  - 需打包多平台 native 二进制（每平台 30-80MB）
  - 每个 PeerConnection 占用 ~10MB native 内存
  - JNI 崩溃直接拖垮 JVM 进程
  - Java GC 暂停可能影响 ICE keepalive

**影响**: 生产环境稳定性风险高，排障成本高

**建议方案**:
1. **Sidecar 架构**（强烈推荐）: Java 服务器旁起一个 Go 进程（用 pion）专做 WebRTC，Java 通过 Unix Socket / gRPC 通信
2. **POC 验证**: 若坚持 webrtc-java，必须先做 100 并发 24h 压测

---

### 3. TCP ↔ DataChannel 桥接层设计完全缺失
**发现者**: Java 专家、架构师  
**问题**: 
- 文档说"服务端将用户的 TCP 连接桥接到 DataChannel"，但这是核心代码路径，全文无任何细节
- 必须回答的问题：
  - TCP 字节流 vs DataChannel 消息流如何切片？切多大？
  - 反向流（DataChannel→TCP）如何回写到 Netty Channel？
  - 双向背压：TCP 写慢时如何暂停 DataChannel 读？
  - DataChannel 配置（ordered/reliable）？

**影响**: 进入实施会撞墙，无法编码

**建议方案**: 补充 `RtcTcpBridge` 组件章节，明确：
- 分片大小（建议 16KB）
- 双向 backpressure 触发器（DataChannel.bufferedAmount 阈值 1MB / TCP `Channel.isWritable()`）
- DataChannel 配置 `ordered=true, maxRetransmits=null`

---

### 4. PeerConnection 资源生命周期管理缺失
**发现者**: Java 专家  
**问题**:
- `WebRTCManager` 只有 create/handle/send，没有 close/remove/cleanup
- libwebrtc 的 PeerConnection 必须显式 `dispose()` 释放，否则 native 内存泄漏导致 JVM 崩溃
- 连接断开时由谁、何时、在哪个线程销毁 PeerConnection？
- `Map<String, PeerConnectionWrapper> connections` 的 remove 时机未定义

**影响**: 长期运行必然 native 内存泄漏

**建议方案**: 
- wrapper 实现 `AutoCloseable`
- 定义 5 种触发清理的事件（控制通道 channelInactive / ICE failed / DataChannel onClose / 业务超时 / 应用关闭 @PreDestroy）
- 统一走 `WebRTCManager.releaseConnection(connectionId)` 回收路径

---

### 5. Go 客户端并发安全缺失
**发现者**: Go 专家  
**问题**:
- `Manager` 结构体的 `state`、`peerConnection`、`dataChannel` 字段被多个 goroutine 访问
- pion 的回调（`OnICECandidate` / `OnConnectionStateChange` / `OnMessage`）在内部 goroutine 执行
- 无 `sync.RWMutex` 或 `atomic` 保护，生产环境必然数据竞争

**影响**: 并发 bug，难以复现和调试

**建议方案**:
```go
type Manager struct {
    mu             sync.RWMutex
    peerConnection *webrtc.PeerConnection
    dataChannel    *webrtc.DataChannel
    state          atomic.Int32
    // 或用 channel 串行化所有状态变更
    stateCh        chan stateTransition
}
```
推荐用 **单一 actor goroutine + channel** 模式串行化状态机变更

---

### 6. Go 客户端 API 与 pion/webrtc 不一致
**发现者**: Go 专家  
**问题**:
- `CreateOffer() (SDP, error)` — pion 实际返回 `webrtc.SessionDescription`
- `SetRemoteAnswer(SDP)` — pion 的方法叫 `SetRemoteDescription(SessionDescription)`
- `AddICECandidate(candidate)` — pion 接受 `webrtc.ICECandidateInit`，不是 string
- 完全没提到必须调用的 `SetLocalDescription(offer)`

**影响**: 初次实现者会踩坑，代码无法编译

**建议方案**: 直接复用 pion 的类型：
```go
func (m *Manager) CreateOffer(ctx context.Context) (webrtc.SessionDescription, error)
func (m *Manager) SetRemoteDescription(sd webrtc.SessionDescription) error
func (m *Manager) AddICECandidate(c webrtc.ICECandidateInit) error
```

---

### 7. DataChannel 背压设计缺失
**发现者**: Go 专家、架构师、Java 专家  
**问题**:
- RDP 流量可达数十 Mbps，DataChannel 内部 SCTP 缓冲区有限（默认 16MB）
- 无 `BufferedAmount` 监控、`OnBufferedAmountLow` 回调，无发送限流
- 高带宽场景下会导致内存暴涨或数据丢失

**影响**: 高带宽场景下 OOM 或数据丢失

**建议方案**:
```go
const bufferThreshold = 1 * 1024 * 1024 // 1MB
dc.SetBufferedAmountLowThreshold(bufferThreshold)
dc.OnBufferedAmountLow(func() { /* 唤醒发送方 */ })
// SendData 内部检查 dc.BufferedAmount() 决定是否阻塞
```

---

### 8. TURN 凭据配置缺失
**发现者**: Go 专家、网络专家  
**问题**:
- `TURNServers []string` 用 `[]string` 表达 TURN，无法承载 username/credential
- TURN 几乎都需要鉴权，当前配置结构无法支持

**影响**: 无法使用需要鉴权的 TURN 服务器，对称 NAT 场景必然失败

**建议方案**: 直接用 pion 的类型：
```go
type Config struct {
    ICEServers  []webrtc.ICEServer  // 包含 URLs, Username, Credential
    P2PTimeout  time.Duration
    EnableP2P   bool
}
```

---

### 9. 缺少 Trickle ICE 机制
**发现者**: 网络专家  
**问题**:
- 文档提到"双方交换 ICE_CANDIDATE（可能多个）"，但未明确是否支持 Trickle ICE
- 如果等待所有候选收集完成再发送 Offer/Answer，会显著增加建立时间（可能超过 5 秒）
- 对称 NAT 场景下，TURN 候选可能需要 3-5 秒才能收集到

**影响**: 连接建立时间过长，用户体验差

**建议方案**:
1. 在协议设计中明确支持 Trickle ICE
2. 修改信令流程：Offer/Answer 发送后立即开始发送 ICE 候选，无需等待收集完成
3. 在 `ICE_CANDIDATE` 消息中增加 `complete: true` 字段，标识候选收集结束

---

### 10. DTLS 握手超时和重传机制缺失
**发现者**: 网络专家  
**问题**:
- WebRTC DataChannel 依赖 DTLS 握手，但文档未提及 DTLS 层的超时和重传配置
- 在高丢包网络（如移动网络）中，DTLS 握手可能因丢包失败
- 默认超时可能过长（30 秒），导致用户等待时间超过预期

**影响**: 弱网环境下连接失败率高

**建议方案**:
1. 在 Go 客户端配置中增加 `DTLSHandshakeTimeout: 10s`
2. 在 Java 服务端配置中设置 `RTCConfiguration.iceCandidatePoolSize = 10`（预收集候选）
3. 监控 `oniceconnectionstatechange` 事件，在 `failed` 状态时立即触发降级

---

### 11. 单元测试边界条件不足
**发现者**: QA 专家  
**问题**:
- 单元测试部分只列出了测试对象，但未明确测试用例和边界条件
- WebRTC 的 ICE 协商、SDP 交换、DataChannel 状态转换都有复杂的边界情况

**影响**: 关键 bug 遗漏，生产环境出现问题

**建议方案**: 补充测试用例：
- Go 客户端：ICE 候选收集失败、SDP 格式错误、DataChannel 异常关闭
- Java 服务端：PeerConnection 创建失败、并发 Offer 处理、ICE 候选乱序到达
- 状态机：非法状态转换、并发状态变更、超时边界条件

---

### 12. 信令通道可靠性测试缺失
**发现者**: QA 专家  
**问题**:
- 设计文档中信令复用 Netty 控制通道，但未测试信令消息丢失、乱序、延迟场景
- 信令消息丢失会导致 ICE 协商卡死，乱序会导致状态不一致

**影响**: 弱网环境下信令失败导致连接建立失败

**建议方案**: 模拟测试：
- 信令消息丢失（ICE_CANDIDATE 丢失、ANSWER 丢失）
- 信令消息乱序（ANSWER 先于 OFFER 到达）
- 信令延迟（超过超时阈值）
- 验证重传机制和超时处理

---

## 🟡 中等问题（强烈建议修复）

### 13. NAT 类型矩阵测试不完整
**发现者**: QA 专家、网络专家、架构师  
**问题**: 只列出了 4 种 NAT 组合，实际 NAT 类型有 7 种  
**建议**: 补充完整的 7×7 NAT 矩阵测试（至少覆盖常见的 20 种组合）

### 14. STUN/TURN 服务器配置不完整
**发现者**: 网络专家、架构师  
**问题**: 
- 只列出 Google 公共 STUN，未配置 TURN
- Google STUN 在中国大陆不可用
- 缺少 TURN 服务器的认证配置

**建议**: 
- 增加多个 STUN 服务器（含国内可用：腾讯云、阿里云）
- 明确 TURN 服务器格式（urls/username/credential）

### 15. 对称 NAT 检测逻辑缺失
**发现者**: 网络专家  
**问题**: 文档提到"5 秒快速失败"用于对称 NAT，但未说明如何检测  
**建议**: 在客户端启动时通过 STUN 请求检测 NAT 类型（RFC 5780）

### 16. P2P 重连场景测试缺失
**发现者**: QA 专家  
**问题**: 设计文档提到"运行中断开后台重试 3 次"，但测试策略未覆盖  
**建议**: 测试 P2P 断开后重连成功/失败、重连过程中 TCP 中继稳定性

### 17. TURN 服务器故障场景测试缺失
**发现者**: QA 专家  
**问题**: 未测试 TURN 不可用场景  
**建议**: 测试 TURN 服务器不可达、限流、故障转移

### 18. 性能测试缺少弱网场景
**发现者**: QA 专家、架构师  
**问题**: 只关注平均延迟和吞吐量，未测试延迟抖动和丢包率  
**建议**: 使用 tc 命令模拟延迟抖动（±50ms）、丢包（1%、5%、10%）

### 19. 15 秒超时过长
**发现者**: 架构师  
**问题**: 工业实践一般 3~5 秒就完成 ICE，15 秒对用户体验是巨大伤害  
**建议**: 拆分超时分层——LAN 候选 1s，srflx 候选 3s，relay 候选 5s；总超时压到 6~8 秒

### 20. pion/webrtc 版本过旧
**发现者**: 架构师、Go 专家  
**问题**: v3.2.24 是 2023 年版本，v4 已 GA（2024）  
**建议**: 升级到 v4 最新版

### 21. ICE 候选过滤策略未定义
**发现者**: 网络专家  
**问题**: 未说明是否过滤 ICE 候选类型（host/srflx/relay）  
**建议**: 增加 `iceTransportPolicy` 选项（all/relay）

### 22. DataChannel 参数未配置
**发现者**: 网络专家、Go 专家  
**问题**: 未说明 `ordered` 和 `maxRetransmits` 参数  
**建议**: RDP 使用 `ordered: true, maxRetransmits: null`（有序可靠传输）

### 23. 状态机覆盖不全
**发现者**: 架构师、Go 专家  
**问题**: 缺失 `Closing`、`P2PReconnecting`、`Hybrid` 等状态  
**建议**: 补全状态并画状态转换图

### 24. 服务器侧 PeerConnection 并发承载未量化
**发现者**: 架构师  
**问题**: 文档要求"100 并发连接"，但未论证 webrtc-java 是否扛得住  
**建议**: 补充资源模型（每连接 X MB 内存、Y 个 UDP 端口）

### 25. 安全相关章节缺失
**发现者**: 架构师  
**问题**: 
- 没有 WebRTC Offer/Answer 的鉴权机制
- ICE 候选会暴露内网拓扑
- 没有 DTLS 证书指纹校验策略

**建议**: 信令消息必须绑定 token，增加候选过滤策略

### 26. GUI 自动化测试缺失
**发现者**: QA 专家  
**问题**: 只提到功能测试和兼容性测试，未提及自动化测试  
**建议**: 使用 Fyne 的测试框架编写 GUI 自动化测试

### 27. 日志和监控验证缺失
**发现者**: QA 专家  
**问题**: 测试策略未提及日志完整性和监控指标验证  
**建议**: 验证关键事件日志和监控指标

---

## 📊 工期评估

**原计划**: 3 周  
**架构师评估**: 6-8 周

**理由**:
- 仅 WebRTC + JNI 集成调试就要 2 周
- 加上跨平台 GUI、NAT 矩阵测试、生产灰度
- 建议要么砍范围（先只做 Linux/Windows），要么调整工期预期

---

## 💡 关键建议

### 1. 澄清架构定位（必须）
- 修正命名为"WebRTC 传输优化"
- 删除"减少服务器带宽"目标
- 明确说明服务器仍在数据路径上

### 2. Java 侧改用 Sidecar 架构（强烈推荐）
- 独立 Go 进程处理 WebRTC
- Java 通过 Unix Socket / gRPC 通信
- 规避 JNI 风险、统一技术栈

### 3. 增加 POC 验证周（3-5 天）
- 验证 webrtc-java（或 sidecar 方案）100 并发 24h 稳定性
- 验证至少 Linux x86_64 + Windows x86_64 两个平台
- 列出回退方案

### 4. 补充桥接层详细设计
- 分片、背压、双向流、切换语义
- 资源生命周期时序图
- 线程模型（WebRTC 内部线程、Netty EventLoop、桥接 executor）

### 5. 修正 Go 客户端设计
- 并发安全（mutex 或 actor 模式）
- API 与 pion/webrtc 一致
- 背压设计
- TURN 凭据配置
- 升级到 pion/webrtc v4

### 6. 完善协议细节
- Trickle ICE 支持
- DTLS 超时配置
- 分层超时（1s/3s/5s）
- 协议版本字段

### 7. 增强测试策略
- 补充单元测试用例（边界条件）
- 信令可靠性测试（丢失/乱序/延迟）
- 完整 NAT 矩阵测试（7×7）
- 弱网性能测试（抖动/丢包）
- P2P 重连场景测试
- TURN 故障测试

### 8. 调整实施计划
- 阶段 0: POC 验证周（3-5 天）
- 阶段 1: 基础设施（1.5 周）
- 阶段 2: 核心功能（2 周）
- 阶段 3: GUI 增强（1 周）
- 阶段 4: 测试和优化（1.5 周）
- 阶段 5: 生产灰度（1 周）
- **总工期**: 7-8 周

---

## 下一步行动

1. **决策**: 选择架构方案（修正命名 vs 真 P2P vs Sidecar）
2. **POC**: 验证技术选型（webrtc-java vs Sidecar）
3. **修订设计文档**: 根据评审反馈重写关键章节
4. **再次评审**: 确认修订后的设计可行
5. **进入实施**: 按修订后的计划执行

---

## 评审团队签名

- ✅ 架构师 - 需要修改（中度修订）
- ✅ Go 专家 - 需要修改
- ✅ Java 专家 - 需要修改
- ✅ 网络专家 - 需要修改
- ✅ QA 专家 - 需要修改

**一致结论**: 设计方向正确，但细节缺失严重，必须修复严重问题后再进入实施。
