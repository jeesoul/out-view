# outView WebRTC 故障排查指南

## 快速诊断

运行调试工具：
```bash
./tools/webrtc-debug.sh
```

## 常见问题

### 1. WebRTC 连接失败，自动降级到 TCP

**症状**: 状态显示 "TCP 降级"，日志中有 "ICE connection failed" 或 "peer connection failed"

**原因和解决方案**:

#### 1.1 STUN 服务器不可达
```
检查: curl -v stun:stun.l.google.com:19302
```
- 确认 UDP 端口 3478 未被防火墙阻断
- 尝试添加国内 STUN 服务器: `stun:stun.qq.com:3478`

#### 1.2 对称 NAT 环境（企业网络）
- 症状: ICE 候选只有 host 类型，无 srflx/relay
- 解决: 配置 TURN 服务器（见用户指南）

#### 1.3 ICE 超时
- 症状: 日志中 "WebRTC timeout after 8s"
- 解决: 增加 `webrtc_timeout` 到 15s，或检查网络延迟

### 2. WebRTC 连接建立很慢（> 3 秒）

**原因**:
- STUN 服务器响应慢
- 网络延迟高
- 候选对太多（多网卡环境）

**解决**:
```yaml
webrtc:
  ice-transport-policy: relay  # 只使用 TURN relay，跳过 ICE 协商
```

或减少 STUN 服务器数量（只保留 1-2 个响应快的）。

### 3. 连接频繁断开重连

**症状**: 状态在 "WebRTC ✓" 和 "WebRTC 重连中..." 之间切换

**原因**:
- 网络不稳定（WiFi 信号弱）
- ICE 连接超时（idle timeout）

**解决**:
```yaml
webrtc:
  idle-timeout: 120s  # 增加空闲超时
```

### 4. Sidecar 进程启动失败

**症状**: 日志中 "Failed to start sidecar process"

**检查**:
```bash
# 检查二进制文件
ls -la /usr/local/bin/outview-sidecar

# 手动启动测试
/usr/local/bin/outview-sidecar --socket /tmp/test.sock

# 检查权限
chmod +x /usr/local/bin/outview-sidecar
```

**Windows 特殊处理**:
- Sidecar 使用 Named Pipe: `\\.\pipe\outview-webrtc`
- 确认 Windows Defender 未阻止 sidecar.exe

### 5. IPC 通信失败

**症状**: 日志中 "IPC connection failed" 或 "dial unix /tmp/outview-webrtc.sock: no such file"

**检查**:
```bash
# 检查 socket 文件
ls -la /tmp/outview-webrtc.sock

# 检查 sidecar 是否运行
ps aux | grep sidecar

# 检查 socket 权限
stat /tmp/outview-webrtc.sock
```

### 6. 数据传输卡顿（背压问题）

**症状**: 日志中频繁出现 "Buffer full, waiting"

**原因**: DataChannel 缓冲区满（> 1MB）

**解决**:
- 检查网络带宽是否足够
- 减少单次发送数据量
- 检查接收端处理速度

### 7. DTLS 握手失败

**症状**: 日志中 "DTLS handshake failed" 或 "DTLS timeout"

**原因**:
- 网络丢包严重（> 10%）
- DTLS 超时设置过短

**解决**:
```yaml
webrtc:
  dtls-timeout: 20s  # 增加 DTLS 超时
```

## 日志分析

### 启用详细日志

```yaml
logging:
  level:
    com.outview.webrtc: DEBUG
```

Go 客户端：
```bash
OUTVIEW_LOG_LEVEL=debug ./outview-client
```

### 关键日志事件

| 日志消息 | 含义 |
|---------|------|
| `ICE gathering complete` | ICE 候选收集完成 |
| `DataChannel opened` | WebRTC 连接建立成功 |
| `ICE connection failed` | ICE 协商失败，触发降级 |
| `data channel closed` | DataChannel 意外关闭 |
| `idle timeout` | 空闲超时，连接关闭 |
| `control channel disconnect` | 控制通道断开 |
| `State transition: X -> Y` | 状态机转换 |

### 日志示例（正常连接）

```
INFO  State transition: Idle -> GatheringICE (offer created)
INFO  ICE candidate type=host address=192.168.1.100
INFO  ICE candidate type=srflx address=203.0.113.1
INFO  ICE gathering complete
INFO  State transition: GatheringICE -> Connecting (remote description set)
INFO  ICE connection state: connected
INFO  DataChannel opened
INFO  State transition: Connecting -> WebRTCConnected (data channel open)
```

### 日志示例（降级到 TCP）

```
INFO  State transition: Idle -> GatheringICE (offer created)
INFO  ICE gathering complete
INFO  State transition: GatheringICE -> Connecting (remote description set)
WARN  ICE connection state: failed
INFO  Closing manager trigger=ICE connection failed
INFO  State transition: Connecting -> WebRTCFailed (ICE connection failed)
INFO  Fallback triggered: ICE connection failed
```

## 网络要求

### 防火墙规则

| 协议 | 端口 | 方向 | 用途 |
|------|------|------|------|
| UDP | 3478 | 出站 | STUN |
| UDP | 3478 | 出站 | TURN |
| TCP | 3478 | 出站 | TURN over TCP |
| UDP | 49152-65535 | 双向 | WebRTC 媒体 |
| TCP | 7000 | 出站 | 信令（控制通道） |

### NAT 类型兼容性

| 客户端 NAT | 服务端 NAT | 是否需要 TURN |
|-----------|-----------|--------------|
| Full Cone | 任意 | 否 |
| Restricted | Restricted | 否 |
| Restricted | Symmetric | 否 |
| Symmetric | Symmetric | **是** |

## 性能调优

### 低延迟优先

```yaml
webrtc:
  ice-transport-policy: all  # 使用所有候选类型
  webrtc-timeout: 5s         # 快速失败
```

### 高可靠性优先

```yaml
webrtc:
  ice-transport-policy: relay  # 强制使用 TURN
  webrtc-timeout: 15s
  dtls-timeout: 20s
```

## 获取支持

1. 运行 `./tools/webrtc-debug.sh` 并保存输出
2. 收集日志（最近 5 分钟）
3. 提交 Issue: https://github.com/outview/outview/issues
