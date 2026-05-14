# WebRTC 传输优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 outView 的服务器到客户端传输层从 TCP 升级为 WebRTC，降低延迟 30-50%，改善弱网稳定性

**Architecture:** Sidecar 架构 - Java 服务器通过 IPC 与独立 Go 进程（WebRTC Sidecar）通信，Go 客户端使用 pion/webrtc v4，信令复用现有 Netty 控制通道

**Tech Stack:** 
- Go: pion/webrtc v4, pion/ice v3, pion/dtls v3
- Java: Netty, Spring Boot
- IPC: Unix Domain Socket / Named Pipe
- GUI: Fyne (Go)

---

## 文件结构规划

### Go 客户端新增文件

```
client/internal/webrtc/
├── manager.go          # WebRTC Manager 核心
├── manager_test.go
├── state.go            # 连接状态机
├── state_test.go
├── ice.go              # ICE 候选处理
├── backpressure.go     # DataChannel 背压控制
├── config.go           # WebRTC 配置
└── types.go            # 类型定义
```

### Java 服务端新增文件（Sidecar）

```
webrtc-sidecar/
├── cmd/sidecar/main.go              # Sidecar 主程序
├── internal/ipc/
│   ├── server.go                    # IPC 服务器
│   ├── protocol.go                  # IPC 协议
│   └── types.go
├── internal/webrtc/
│   ├── manager.go                   # WebRTC Manager（服务端版）
│   └── pool.go                      # PeerConnection 池
└── internal/bridge/
    └── tcp_ipc_bridge.go            # TCP ↔ IPC 桥接
```

### Java 服务端修改文件

```
src/main/java/com/outview/
├── webrtc/
│   ├── WebRTCProxyService.java      # IPC 客户端
│   ├── TcpIpcBridge.java            # TCP ↔ IPC 桥接
│   ├── SidecarManager.java          # Sidecar 进程管理
│   └── WebRTCConfig.java            # 配置类
├── protocol/MessageType.java        # 新增 WebRTC 消息类型
└── netty/handler/WebRTCHandler.java # WebRTC 信令处理器
```

---


## 阶段 0: POC 验证（3-5 天）

### Task 1: Sidecar 架构 POC

**Goal:** 验证 Java ↔ Go IPC 通信可行性

**Files:**
- Create: webrtc-sidecar/internal/ipc/server.go
- Create: src/main/java/com/outview/poc/IPCClient.java

- [ ] 创建 Go IPC 服务器（Unix Socket，消息格式：4字节长度 + JSON）
- [ ] 创建 Java IPC 客户端（使用 junixsocket 库）
- [ ] 测试双向通信（ping-pong 消息）
- [ ] 验证 100 并发连接稳定性
- [ ] 提交代码

**Verification:**
Run Go server: cd webrtc-sidecar && go run cmd/poc/main.go
Run Java client: cd out-view && mvn exec:java -Dexec.mainClass=com.outview.poc.IPCClient
Expected: 成功收发消息

---

### Task 2: pion/webrtc v4 基础验证

**Goal:** 验证 pion/webrtc v4 API 和功能

**Files:**
- Create: webrtc-sidecar/internal/webrtc/poc_manager.go
- Create: webrtc-sidecar/cmd/poc-webrtc/main.go

- [ ] 添加 pion/webrtc v4 依赖到 go.mod
- [ ] 创建 PeerConnection + DataChannel
- [ ] 实现 Offer/Answer 交换
- [ ] 测试 DataChannel 数据传输
- [ ] 验证 ICE 候选收集
- [ ] 提交代码

**Verification:**
Run: cd webrtc-sidecar && go run cmd/poc-webrtc/main.go
Expected: DataChannel 建立成功，数据传输正常

---

### Task 3: 100 并发稳定性测试

**Goal:** 验证 Sidecar 架构在高并发下的稳定性

**Files:**
- Create: webrtc-sidecar/cmd/stress-test/main.go

- [ ] 创建压力测试程序（100 并发 PeerConnection）
- [ ] 运行 24 小时稳定性测试
- [ ] 监控内存/CPU 使用率
- [ ] 记录性能数据（建立耗时、吞吐量）
- [ ] 生成 POC 报告

**Verification:**
Run: cd webrtc-sidecar && go run cmd/stress-test/main.go -connections=100 -duration=24h
Expected: 无崩溃，内存稳定，CPU < 50%

---

### Task 4: 跨平台验证

**Goal:** 验证 Linux x86_64 + Windows x86_64 平台兼容性

- [ ] Linux 平台编译测试
- [ ] Windows 平台编译测试（Named Pipe）
- [ ] 验证 IPC 通信在两平台正常工作
- [ ] 提交 POC 报告

**Verification:**
Linux: GOOS=linux GOARCH=amd64 go build -o sidecar-linux ./cmd/sidecar
Windows: GOOS=windows GOARCH=amd64 go build -o sidecar-windows.exe ./cmd/sidecar

---

## 阶段 1: 基础设施（1.5 周）

### Task 5: Go 客户端集成 pion/webrtc v4

**Files:**
- Create: client/internal/webrtc/manager.go
- Create: client/internal/webrtc/config.go
- Create: client/internal/webrtc/types.go
- Modify: client/go.mod

- [ ] 添加 pion/webrtc v4 依赖到 client/go.mod
- [ ] 创建 Manager 结构体（包含 sync.RWMutex, atomic.Int32）
- [ ] 实现 CreateOffer/SetRemoteDescription/AddICECandidate 方法
- [ ] 实现 SendData 方法（带背压检查）
- [ ] 实现 Close 方法（资源清理）
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -v

---

### Task 6: Sidecar IPC 服务器开发

**Files:**
- Create: webrtc-sidecar/internal/ipc/server.go
- Create: webrtc-sidecar/internal/ipc/protocol.go
- Create: webrtc-sidecar/internal/ipc/types.go

- [ ] 实现 IPC 服务器（Unix Socket + Named Pipe）
- [ ] 定义 IPC 消息类型（create_pc, set_remote_sdp, add_ice_candidate, send_data, close_pc, event）
- [ ] 实现消息路由和处理
- [ ] 实现连接管理（多客户端支持）
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd webrtc-sidecar && go test ./internal/ipc/... -v

---

### Task 7: Sidecar WebRTC Manager 开发

**Files:**
- Create: webrtc-sidecar/internal/webrtc/manager.go
- Create: webrtc-sidecar/internal/webrtc/pool.go

- [ ] 实现 WebRTC Manager（服务端版）
- [ ] 实现 PeerConnection 池管理
- [ ] 实现 Offer/Answer 处理
- [ ] 实现 ICE 候选处理
- [ ] 实现 DataChannel 事件回调
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd webrtc-sidecar && go test ./internal/webrtc/... -v

---

### Task 8: Java IPC 客户端开发

**Files:**
- Create: src/main/java/com/outview/webrtc/WebRTCProxyService.java
- Create: src/main/java/com/outview/webrtc/IPCConnection.java
- Create: src/main/java/com/outview/webrtc/WebRTCConfig.java

- [ ] 实现 IPC 客户端（连接到 Sidecar）
- [ ] 实现消息发送/接收
- [ ] 实现异步回调处理
- [ ] 实现连接池管理
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd out-view && mvn test -Dtest=WebRTCProxyServiceTest

---

### Task 9: 协议扩展

**Files:**
- Modify: client/internal/protocol/constants.go
- Modify: client/internal/protocol/message.go
- Modify: src/main/java/com/outview/protocol/MessageType.java

- [ ] 新增 6 个 WebRTC 消息类型常量
- [ ] 实现 WebRTC 消息编码/解码
- [ ] 更新协议文档
- [ ] 编写单元测试
- [ ] 提交代码

New Message Types:
  TYPE_WEBRTC_OFFER = 8
  TYPE_WEBRTC_ANSWER = 9
  TYPE_WEBRTC_ICE_CANDIDATE = 10
  TYPE_WEBRTC_ESTABLISHED = 11
  TYPE_WEBRTC_FAILED = 12
  TYPE_WEBRTC_ICE_COMPLETE = 13

---

### Task 10: 基础信令流程

**Files:**
- Create: src/main/java/com/outview/netty/handler/WebRTCHandler.java
- Modify: client/internal/client/client.go

- [ ] 实现 Java 端 WebRTC 信令处理器
- [ ] 实现 Go 客户端信令处理
- [ ] 实现 Offer/Answer 交换流程
- [ ] 实现 ICE 候选交换流程
- [ ] 编写集成测试
- [ ] 提交代码

**Test:** 启动服务器 + 客户端，验证信令交换成功

---

## 阶段 2: 核心功能（2 周）

### Task 11: Trickle ICE 实现

**Files:**
- Create: client/internal/webrtc/ice.go
- Create: client/internal/webrtc/ice_test.go
- Modify: webrtc-sidecar/internal/webrtc/manager.go

- [ ] 实现 OnICECandidate 回调（立即发送候选）
- [ ] 实现 ICE 候选缓存（处理乱序）
- [ ] 实现 ICE 收集完成通知
- [ ] 实现 ICE 连接状态监控
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -run TestICE -v

---

### Task 12: DataChannel 背压控制

**Files:**
- Create: client/internal/webrtc/backpressure.go
- Create: client/internal/webrtc/backpressure_test.go

- [ ] 实现 BufferedAmount 监控
- [ ] 实现高/低水位阈值（1MB/512KB）
- [ ] 实现 OnBufferedAmountLow 回调
- [ ] 实现发送阻塞逻辑
- [ ] 编写单元测试（模拟缓冲区满）
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -run TestBackpressure -v

---

### Task 13: TCP ↔ IPC ↔ DataChannel 桥接层

**Files:**
- Create: src/main/java/com/outview/webrtc/TcpIpcBridge.java
- Create: webrtc-sidecar/internal/bridge/tcp_ipc_bridge.go

- [ ] 实现 Java 端 TCP → IPC 桥接（16KB 分片）
- [ ] 实现 Java 端 IPC → TCP 桥接
- [ ] 实现 Go 端 IPC → DataChannel 桥接
- [ ] 实现 Go 端 DataChannel → IPC 桥接
- [ ] 实现双向背压控制
- [ ] 编写集成测试
- [ ] 提交代码

**Test:** cd out-view && mvn test -Dtest=TcpIpcBridgeIntegrationTest

---

### Task 14: 超时和降级逻辑

**Files:**
- Modify: client/internal/webrtc/manager.go
- Modify: src/main/java/com/outview/netty/handler/WebRTCHandler.java

- [ ] 实现分层超时（host 1s / srflx 3s / relay 5s / 总 8s）
- [ ] 实现超时检测和降级触发
- [ ] 实现 TCP 降级流程
- [ ] 实现降级状态通知
- [ ] 编写单元测试
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -run TestTimeout -v

---

### Task 15: 连接状态机

**Files:**
- Create: client/internal/webrtc/state.go
- Create: client/internal/webrtc/state_test.go

- [ ] 定义 9 个状态常量
- [ ] 实现状态转换矩阵
- [ ] 实现 transitionTo 方法（带合法性检查）
- [ ] 实现状态变更回调
- [ ] 编写单元测试（测试非法转换）
- [ ] 提交代码

States: StateIdle, StateGatheringICE, StateConnecting, StateWebRTCConnected,
        StateWebRTCFailed, StateWebRTCReconnecting, StateTCPRelay, StateClosing, StateClosed

---

### Task 16: 资源生命周期管理

**Files:**
- Modify: client/internal/webrtc/manager.go
- Modify: webrtc-sidecar/internal/webrtc/manager.go

- [ ] 实现 Close 方法（防止重复关闭）
- [ ] 实现 5 种清理触发事件
- [ ] 实现资源清理顺序（DataChannel → PeerConnection）
- [ ] 实现清理日志记录
- [ ] 编写单元测试
- [ ] 提交代码

5 Cleanup Triggers:
  1. 控制通道断开
  2. ICE failed
  3. DataChannel onClose
  4. 业务超时
  5. 应用关闭

---

## 阶段 3: GUI 增强（1 周）

### Task 17: WebRTC 配置选项卡

**Files:**
- Modify: client/cmd/outview-gui/main.go
- Create: client/internal/gui/webrtc_config.go

- [ ] 新增 WebRTC 配置选项卡
- [ ] 添加启用/禁用 WebRTC 开关
- [ ] 添加 STUN 服务器配置
- [ ] 添加 TURN 服务器配置
- [ ] 添加超时配置
- [ ] 实现配置保存/加载
- [ ] 提交代码

---

### Task 18: 连接状态显示

**Files:**
- Modify: client/cmd/outview-gui/main.go

- [ ] 添加连接状态标签（WebRTC / TCP）
- [ ] 实现状态实时更新
- [ ] 添加状态颜色指示（绿色=WebRTC，黄色=TCP）
- [ ] 提交代码

---

### Task 19: 系统托盘功能

**Files:**
- Modify: client/cmd/outview-gui/main.go

- [ ] 实现系统托盘图标
- [ ] 实现托盘菜单（显示/隐藏、退出）
- [ ] 实现状态图标切换（WebRTC/TCP）
- [ ] 提交代码

---

### Task 20: 实时流量图表

**Files:**
- Create: client/internal/gui/traffic_chart.go

- [ ] 实现流量统计收集
- [ ] 实现实时图表绘制
- [ ] 添加上传/下载速率显示
- [ ] 提交代码

---

### Task 21: ICE 协商进度条

**Files:**
- Create: client/internal/gui/ice_progress.go

- [ ] 实现 ICE 协商进度条
- [ ] 显示当前阶段（收集候选/连接测试/DTLS握手）
- [ ] 显示预计剩余时间
- [ ] 提交代码

---

## 阶段 4: 测试和优化（1.5 周）

### Task 22: 单元测试 - 边界条件

**Files:**
- Create: client/internal/webrtc/*_test.go
- Create: webrtc-sidecar/internal/webrtc/*_test.go

- [ ] TestManager_CreateOffer_PCClosed
- [ ] TestManager_SetRemoteDescription_InvalidSDP
- [ ] TestManager_AddICECandidate_BeforeOffer
- [ ] TestManager_SendData_BufferFull
- [ ] TestManager_SendData_ChannelClosed
- [ ] TestManager_StateTransition_Invalid
- [ ] TestManager_ConcurrentClose
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -v -cover

---

### Task 23: 信令可靠性测试

**Files:**
- Create: client/internal/webrtc/signaling_test.go

- [ ] 场景1: ICE 候选丢失 30%
- [ ] 场景2: ANSWER 延迟 10 秒
- [ ] 场景3: 消息乱序（ANSWER 先于 OFFER）
- [ ] 场景4: 信令延迟 5 秒
- [ ] 提交代码

**Test:** cd client && go test ./internal/webrtc/... -run TestSignaling -v

---

### Task 24: NAT 矩阵测试

**Files:**
- Create: test/nat/docker-compose.yml
- Create: test/nat/run_matrix_test.sh

- [ ] Full Cone × Full Cone
- [ ] Full Cone × Restricted Cone
- [ ] Full Cone × Port Restricted
- [ ] Full Cone × Symmetric
- [ ] Restricted Cone × Restricted Cone
- [ ] Restricted Cone × Symmetric
- [ ] Port Restricted × Port Restricted
- [ ] Port Restricted × Symmetric
- [ ] Symmetric × Symmetric (无 TURN)
- [ ] Symmetric × Symmetric (有 TURN)
- [ ] 提交代码

**Test:** cd test/nat && ./run_matrix_test.sh

---

### Task 25: P2P 重连场景测试

**Files:**
- Create: test/reconnect/reconnect_test.go

- [ ] 场景1: 网络切换（WiFi → 4G）
- [ ] 场景2: 临时网络中断 5 秒
- [ ] 场景3: 持续网络故障 60 秒
- [ ] 提交代码

---

### Task 26: TURN 服务器故障测试

**Files:**
- Create: test/turn/turn_failure_test.go

- [ ] 场景1: TURN 不可达
- [ ] 场景2: TURN 限流（429）
- [ ] 提交代码

---

### Task 27: 弱网性能测试

**Files:**
- Create: test/performance/weak_network_test.sh

- [ ] 场景1: 高延迟抖动（100ms ± 50ms）
- [ ] 场景2: 高丢包（5%）
- [ ] 场景3: 带宽限制（1 Mbps）
- [ ] 提交代码

---

### Task 28: GUI 自动化测试

**Files:**
- Create: client/cmd/outview-gui/gui_test.go

- [ ] TestGUI_ConfigSaveLoad
- [ ] TestGUI_ConnectionStateDisplay
- [ ] TestGUI_TrayIcon
- [ ] 提交代码

---

### Task 29: 性能优化

- [ ] 内存优化（减少分配，使用对象池）
- [ ] CPU 优化（减少锁竞争，使用 channel）
- [ ] 延迟优化（减少拷贝，零拷贝技术）
- [ ] 吞吐量优化（批量发送，调整缓冲区）
- [ ] 性能测试和基准测试
- [ ] 提交代码

**Benchmark:** cd client && go test ./internal/webrtc/... -bench=. -benchmem

---

## 阶段 5: 生产灰度（1 周）

### Task 30: 灰度发布配置

**Files:**
- Modify: src/main/java/com/outview/config/OutViewProperties.java
- Create: src/main/java/com/outview/webrtc/GrayReleaseManager.java

- [ ] 实现设备 ID 分桶算法
- [ ] 实现灰度比例配置（10% / 50% / 100%）
- [ ] 实现灰度开关（动态配置）
- [ ] 编写单元测试
- [ ] 提交代码

---

### Task 31: 监控指标上报

**Files:**
- Create: src/main/java/com/outview/webrtc/WebRTCMetrics.java
- Create: client/internal/webrtc/metrics.go

- [ ] webrtc_connections_total
- [ ] webrtc_connections_active
- [ ] webrtc_success_rate
- [ ] webrtc_fallback_rate
- [ ] webrtc_establish_duration_ms (P50/P95/P99)
- [ ] webrtc_latency_ms (P50/P95/P99)
- [ ] webrtc_throughput_mbps
- [ ] webrtc_errors_total{reason}
- [ ] 提交代码

---

### Task 32: 日志增强

**Files:**
- Modify: client/internal/logger/logger.go

- [ ] 添加 WebRTC 关键事件日志
- [ ] 添加结构化日志字段（connectionId, state, reason）
- [ ] 添加日志级别控制
- [ ] 提交代码

---

### Task 33: 问题排查工具

**Files:**
- Create: tools/webrtc-debug.sh
- Create: tools/webrtc-stats.go

- [ ] 创建 WebRTC 调试脚本（检查 STUN 可达性、NAT 类型）
- [ ] 创建 WebRTC 统计工具（连接统计、性能分析）
- [ ] 编写使用文档
- [ ] 提交代码

---

### Task 34: 文档更新

**Files:**
- Create: docs/webrtc-user-guide.md
- Create: docs/webrtc-troubleshooting.md
- Modify: README.md

- [ ] 编写用户指南（如何启用 WebRTC、配置 STUN/TURN）
- [ ] 编写故障排查指南（常见问题和解决方案）
- [ ] 更新 README（新增 WebRTC 功能说明）
- [ ] 提交代码

---

### Task 35: 灰度发布和监控

- [ ] 第 1 天: 10% 灰度（监控指标、收集反馈）
- [ ] 第 3 天: 50% 灰度（继续监控）
- [ ] 第 5 天: 100% 全量（持续监控 1 周）
- [ ] 生成灰度报告

---

## 总结

**总工期**: 7-8 周

**关键里程碑**:
- 第 1 周: POC 验证完成
- 第 2.5 周: 基础设施完成
- 第 4.5 周: 核心功能完成
- 第 5.5 周: GUI 增强完成
- 第 7 周: 测试和优化完成
- 第 8 周: 生产灰度完成

**风险缓解**:
- POC 阶段提前识别风险
- 分阶段交付，每阶段可独立验证
- TCP 降级保障连接成功率
- 灰度发布降低生产风险

**下一步**: 开始阶段 0 POC 验证
