# outView WebRTC 用户指南

## 概述

outView 1.2.0 引入了 WebRTC 传输层，将服务器到客户端的数据传输从 TCP 升级为 WebRTC DataChannel，在大多数网络环境下可降低延迟 30-50%。

## 架构说明

```
Java 服务器 ←→ IPC (Unix Socket/Named Pipe) ←→ Go WebRTC Sidecar ←→ Go 客户端
```

- **Java 服务器**: 处理业务逻辑，通过 IPC 与 Sidecar 通信
- **WebRTC Sidecar**: 独立 Go 进程，管理 WebRTC PeerConnection
- **Go 客户端**: 直接使用 pion/webrtc v4 建立 WebRTC 连接
- **信令**: 复用现有 Netty 控制通道（TCP 端口 7000）

## 启用 WebRTC

### 服务端配置

在 `application.yml` 中添加：

```yaml
webrtc:
  enabled: true
  sidecar-binary-path: /usr/local/bin/outview-sidecar
  sidecar-socket-path: /tmp/outview-webrtc.sock
  connect-timeout-ms: 5000
  read-timeout-ms: 30000
```

### 客户端配置

在 GUI 的 "WebRTC 配置" 选项卡中：

1. 勾选 "启用 WebRTC"
2. 配置 STUN 服务器（默认已包含 Google STUN）
3. 可选：配置 TURN 服务器（用于对称 NAT 环境）
4. 点击 "保存"

或通过配置文件 `~/.outview/config.json`：

```json
{
  "webrtc": {
    "enabled": true,
    "ice_servers": [
      {"urls": ["stun:stun.l.google.com:19302"]},
      {"urls": ["stun:stun.qq.com:3478"]}
    ],
    "webrtc_timeout": "8s",
    "dtls_timeout": "10s",
    "idle_timeout": "60s"
  }
}
```

## STUN/TURN 服务器配置

### 内置 STUN 服务器

outView 默认使用以下 STUN 服务器：
- `stun:stun.l.google.com:19302`
- `stun:stun1.l.google.com:19302`
- `stun:stun.qq.com:3478`

### 配置自定义 TURN 服务器

对于对称 NAT 环境（企业网络、严格防火墙），需要 TURN 服务器：

```json
{
  "webrtc": {
    "ice_servers": [
      {"urls": ["stun:stun.l.google.com:19302"]},
      {
        "urls": ["turn:your-turn-server.com:3478"],
        "username": "your-username",
        "credential": "your-password"
      }
    ]
  }
}
```

### 推荐 TURN 服务器

- **自建**: 使用 [coturn](https://github.com/coturn/coturn)
- **云服务**: Twilio TURN, Xirsys, Metered.ca

## 连接状态说明

| 状态 | 颜色 | 说明 |
|------|------|------|
| WebRTC ✓ | 绿色 | WebRTC DataChannel 已建立，低延迟模式 |
| WebRTC 重连中... | 黄色 | ICE 断开，正在重新协商 |
| TCP 降级 | 橙色 | WebRTC 失败，已降级到 TCP 传输 |
| 连接中... | 灰色 | 正在建立连接 |
| 已断开 | 红色 | 连接已关闭 |

## 超时配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `webrtc_timeout` | 8s | WebRTC 连接建立总超时 |
| `dtls_timeout` | 10s | DTLS 握手超时 |
| `idle_timeout` | 60s | 无数据活动后自动关闭 |

## 灰度发布

服务端支持按设备 ID 分桶的灰度发布：

```yaml
webrtc:
  gray-release:
    enabled: true
    percentage: 10  # 10% 的设备使用 WebRTC
```

灰度比例建议：
1. 第 1 天: 10%（监控指标）
2. 第 3 天: 50%（继续监控）
3. 第 5 天: 100%（全量）

## 性能指标

WebRTC 相比 TCP 的典型改善：

| 指标 | TCP | WebRTC | 改善 |
|------|-----|--------|------|
| 延迟 (P50) | 20ms | 12ms | -40% |
| 延迟 (P95) | 50ms | 28ms | -44% |
| 弱网稳定性 | 中 | 高 | +++ |
| 连接建立时间 | 100ms | 500ms | -5x (首次) |

注：WebRTC 首次连接需要 ICE 协商（约 500ms-2s），之后重连更快。

## 监控

### 关键指标

- `webrtc_connections_active`: 当前活跃 WebRTC 连接数
- `webrtc_success_rate`: WebRTC 连接成功率
- `webrtc_fallback_rate`: TCP 降级率（应 < 5%）
- `webrtc_establish_duration_ms_p95`: P95 建立时间（应 < 3000ms）

### 查看统计

```bash
# 实时统计
./tools/webrtc-stats -watch

# JSON 格式输出
./tools/webrtc-stats -json

# 调试工具
./tools/webrtc-debug.sh
```

## 常见问题

详见 [故障排查指南](webrtc-troubleshooting.md)。
