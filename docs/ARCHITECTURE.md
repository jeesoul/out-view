# 实际架构与职责

## 稳定数据路径

```text
mstsc / 外部 TCP 客户端
  ↕ 原始 TCP（设备固定外网端口）
RawDataHandler ↔ ProxyHandler / Netty 控制连接
  ↕ OVWS v1，默认7000
Go Client 控制连接 ↔ 每 connectionId 独立的本地转发任务
  ↕ TCP
127.0.0.1:local-port（默认RDP 3389）
```

控制协议为 12 字节大端头：Magic、Version、Type、Length、Reserved。注册、心跳和查询使用 JSON；数据体携带连接 ID 长度、ID 和二进制负载。消息类型 7 双向通知单条转发关闭，1–15 的线格式保持兼容。

## Java

| 组件 | 职责 |
| --- | --- |
| NettyServer / Initializer | 监听、协议编解码、读空闲检测和处理器编排 |
| DeviceLifecycleService | 按设备串行协调注册、断开、封禁、预留和在线改端口 |
| PortMappingService | 固定端口分配、验证、持久化以及内存视图 |
| SessionStore | 当前控制连接所有权，替换时关闭旧连接并防止旧事件清新会话 |
| DataPortService | 数据监听及其设备归属，关闭监听与关联外部连接 |
| RawDataHandler / ProxyHandler | connectionId 路由、归属检查、背压、关闭通知 |
| Controller | HTTP 请求和结果，生命周期操作委托服务层 |

心跳、查询、数据和单连接关闭在 IO 线程处理，绕过注册队列；注册涉及的数据库和 bind 操作由独立业务线程池处理。IO读取已提交映射快照，不等待数据库写锁。固定映射与封禁是持久数据，会话与在线状态是运行时数据。离线不会释放端口预留。

一组设备与端口池由单个服务实例管理；本项目没有提供跨实例会话调度。

## Go

| 文件 | 职责 |
| --- | --- |
| client.go | 客户端状态、回调及协议消息分派 |
| connection.go | 首次连接、注册、读写期限、心跳 RTT、重连和停止 |
| forwarding.go | 本地连接、有界队列、关闭传播与资源回收 |
| signaling.go | 实验 WebRTC 信令，约束旧 manager / 旧控制连接的异步事件 |
| config.go | 默认值、文件和环境配置、参数验证 |

控制连接按代际顺序恢复。本地拨号/写入在单连接任务中执行，慢本地服务不会阻塞控制消息读取。旧任务不能写入新控制连接；Stop 取消等待并回收所有自有任务。

GUI 入口拆为 main、host、control、config 和 webrtc_tab；状态刷新任务捕获当前客户端并可取消。后台的 HTML、CSS、JavaScript 分开维护，动态数据使用 DOM 文本节点。

## 背压与资源边界

客户端每条本地连接默认最多排队 1 MiB / 64 帧，同时最多128条。单连接过载关闭该连接并通知服务端；全局关闭通知队列也饱和时使该代控制连接失效，统一回收后重连。服务端对慢外部接收者限制待写字节，不以暂停控制读来阻挡心跳。

## WebRTC 实验范围

Java IPC 代理、Go Sidecar 和 pion 组件保留在原模块中，但尚未构成已交付的数据路径。历史设计的目标是服务器到被控端的传输优化，服务器仍中转；终端到终端的 P2P 另需控制端代理。

详见 [实验模块说明](webrtc-user-guide.md)。
