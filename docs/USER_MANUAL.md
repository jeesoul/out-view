# outView 系统使用手册

## 系统简介

outView 是一款轻量级、高性能的远程桌面内网穿透系统，支持 TCP 和 WebRTC 双传输模式。通过公网服务器中转，让您从外部网络安全访问内网电脑的远程桌面服务（RDP）。

**v1.2.0 新特性**: WebRTC P2P 传输，延迟降低 30-50%，弱网稳定性显著提升。

**v1.2.0 迭代新特性**: 固定外网映射端口（客户端重连/服务端重启端口不变）；断网重连稳定性修复；被控端默认无限重连。

## 固定端口映射

v1.2.0 起，设备首次上线时分配的外网端口会持久化到数据库，此后客户端重连或服务端重启，端口保持不变。

- **自动固定**：设备首次连接自动分配并固化端口，无需配置
- **管理员预设**：在管理后台「端口映射」卡片，输入设备ID和端口，点击「添加」即可为设备预设固定端口（设备未上线也可预设）
- **修改端口**：在端口映射列表点击「修改端口」可调整固定端口
- **删除映射**：点击「删除」释放端口，设备下次上线将重新分配

> 端口范围默认 6000-6500（可在 `application.yml` 的 `outview.data-port-start` / `data-port-end` 配置）。


## 快速开始

### 1. 服务端部署（公网服务器）

#### 环境要求
- 公网 IP 或域名
- JDK 8+
- 开放端口: 7000（控制）、6001-6100（数据）

#### 安装步骤

```bash
# 1. 下载服务端
wget https://github.com/outview/outview/releases/download/v1.2.0/outview-server-1.2.0.jar

# 2. 创建配置文件
cat > application.yml <<EOF
server:
  port: 7000

outview:
  data-port-range:
    start: 6001
    end: 6100
  
  # WebRTC 配置（可选）
  webrtc:
    enabled: true
    sidecar-binary-path: /usr/local/bin/outview-sidecar
    sidecar-socket-path: /tmp/outview-webrtc.sock
    
    # 灰度发布（可选）
    gray-release:
      enabled: false
      percentage: 0  # 0-100
EOF

# 3. 启动服务
java -jar outview-server-1.2.0.jar
```

#### 使用 systemd 管理（推荐）

```bash
# 创建 systemd 服务
sudo cat > /etc/systemd/system/outview.service <<EOF
[Unit]
Description=outView Server
After=network.target

[Service]
Type=simple
User=outview
WorkingDirectory=/opt/outview
ExecStart=/usr/bin/java -jar /opt/outview/outview-server-1.2.0.jar
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable outview
sudo systemctl start outview

# 查看状态
sudo systemctl status outview
```

### 2. 客户端安装（内网电脑）

#### Windows

1. 下载客户端: `outview-client.exe` 或 `outview-gui.exe`
2. 双击运行 GUI 版本，或命令行运行 CLI 版本

**GUI 版本**:
- 填写服务器地址、端口、设备 ID、Token
- 点击"连接"按钮
- 查看连接状态（绿色 = WebRTC，橙色 = TCP）

**CLI 版本**:
```cmd
outview-client.exe ^
  --server 203.0.113.1 ^
  --port 7000 ^
  --device-id my-pc-001 ^
  --token your-secret-token ^
  --local-port 3389
```

#### Linux

```bash
# 下载客户端
wget https://github.com/outview/outview/releases/download/v1.2.0/outview-client-linux-amd64

# 添加执行权限
chmod +x outview-client-linux-amd64

# 运行
./outview-client-linux-amd64 \
  --server 203.0.113.1 \
  --port 7000 \
  --device-id my-pc-001 \
  --token your-secret-token \
  --local-port 3389
```

#### macOS

```bash
# 下载客户端
curl -LO https://github.com/outview/outview/releases/download/v1.2.0/outview-client-darwin-amd64

# 添加执行权限
chmod +x outview-client-darwin-amd64

# 运行
./outview-client-darwin-amd64 \
  --server 203.0.113.1 \
  --port 7000 \
  --device-id my-pc-001 \
  --token your-secret-token \
  --local-port 22  # SSH
```

### 3. 远程连接（外出电脑）

#### Windows 远程桌面

1. 打开"远程桌面连接"（mstsc.exe）
2. 输入: `服务器地址:外部端口`
   - 例如: `203.0.113.1:6001`
3. 输入内网电脑的用户名和密码
4. 连接成功

#### Linux/macOS 使用 rdesktop

```bash
rdesktop 203.0.113.1:6001
```

#### 使用 FreeRDP

```bash
xfreerdp /v:203.0.113.1:6001 /u:username /p:password
```

## WebRTC 配置

### 启用 WebRTC（客户端）

#### GUI 配置

1. 打开"WebRTC 配置"选项卡
2. 勾选"启用 WebRTC"
3. 配置 STUN 服务器（默认已包含 Google STUN）
4. 可选：配置 TURN 服务器（用于对称 NAT 环境）
5. 点击"保存"

#### CLI 配置文件

创建 `~/.outview/config.json`:

```json
{
  "server_host": "203.0.113.1",
  "server_port": 7000,
  "device_id": "my-pc-001",
  "token": "your-secret-token",
  "local_port": 3389,
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

### STUN/TURN 服务器配置

#### 使用公共 STUN 服务器

```json
{
  "ice_servers": [
    {"urls": ["stun:stun.l.google.com:19302"]},
    {"urls": ["stun:stun1.l.google.com:19302"]},
    {"urls": ["stun:stun.qq.com:3478"]}
  ]
}
```

#### 配置 TURN 服务器（企业网络）

```json
{
  "ice_servers": [
    {"urls": ["stun:stun.l.google.com:19302"]},
    {
      "urls": ["turn:your-turn-server.com:3478"],
      "username": "your-username",
      "credential": "your-password"
    }
  ]
}
```

#### 自建 TURN 服务器（coturn）

```bash
# 安装 coturn
sudo apt-get install coturn

# 配置 /etc/turnserver.conf
listening-port=3478
fingerprint
lt-cred-mech
user=testuser:testpass
realm=yourdomain.com
external-ip=YOUR_PUBLIC_IP

# 启动
sudo systemctl start coturn
```

## 连接状态说明

### GUI 状态指示

| 状态 | 颜色 | 说明 |
|------|------|------|
| WebRTC ✓ | 绿色 | WebRTC P2P 连接已建立，低延迟模式 |
| WebRTC 重连中... | 黄色 | ICE 连接断开，正在重新协商 |
| TCP 降级 | 橙色 | WebRTC 失败，已降级到 TCP 中转 |
| 连接中... | 灰色 | 正在建立连接 |
| 已断开 | 红色 | 连接已关闭 |

### CLI 日志输出

```
[INFO] State transition: Idle -> GatheringICE (offer created)
[INFO] ICE candidate type=host address=192.168.1.100
[INFO] ICE candidate type=srflx address=203.0.113.1
[INFO] ICE gathering complete
[INFO] State transition: GatheringICE -> Connecting (remote description set)
[INFO] ICE connection state: connected
[INFO] DataChannel opened
[INFO] State transition: Connecting -> WebRTCConnected (data channel open)
```

## 性能监控

### 查看实时统计

```bash
# 下载统计工具
wget https://github.com/outview/outview/releases/download/v1.2.0/webrtc-stats

# 实时监控
./webrtc-stats -watch

# JSON 格式输出
./webrtc-stats -json
```

### 输出示例

```
=== outView WebRTC Statistics ===
Time: 2026-05-17 10:30:00

Summary:
  Active connections:  5 / 10 total
  Success rate:        95.0%
  Fallback rate:       5.0%
  Avg establish time:  450ms
  P95 establish time:  800ms
  Total sent:          1.2 GB
  Total received:      2.4 GB

Connections:
  ID                   State        Uptime     Sent         Received     Fallbacks
  ─────────────────────────────────────────────────────────────────────────────────
  conn-abc-123         connected    5.2h       245.3 MB     512.1 MB     0
  conn-def-456         connected    2.1h       102.5 MB     198.7 MB     0
  conn-ghi-789         tcp-relay    1.5h       89.2 MB      156.3 MB     1
```

### Grafana 监控（服务端）

配置 Prometheus 抓取指标：

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'outview'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/actuator/prometheus'
```

导入 Grafana Dashboard（见 `docs/grafana-dashboard.json`）。

## 常见使用场景

### 场景 1: 家庭远程办公

**需求**: 从公司访问家里的 Windows 电脑

**配置**:
1. 家里电脑运行 outview-client
2. 公司电脑使用 mstsc 连接
3. 启用 WebRTC 降低延迟

**预期效果**:
- 延迟: 20-50ms（WebRTC）vs 50-100ms（TCP）
- 带宽: 1-5 Mbps（取决于屏幕分辨率）

### 场景 2: 企业内网穿透

**需求**: 外出员工访问公司内网服务器

**配置**:
1. 内网服务器运行 outview-client
2. 配置 TURN 服务器（企业防火墙严格）
3. 启用灰度发布（10% → 50% → 100%）

**安全建议**:
- 使用强 Token（32+ 字符随机字符串）
- 启用 SSL/TLS 加密
- 配置防火墙白名单

### 场景 3: 多台设备管理

**需求**: 管理多台内网设备（服务器、NAS、路由器）

**配置**:
1. 每台设备运行 outview-client，使用不同 device-id
2. 服务端自动分配不同外部端口
3. 使用 GUI 客户端管理多个连接

**示例**:
```
设备 1 (my-pc-001)    → 203.0.113.1:6001
设备 2 (my-nas-002)   → 203.0.113.1:6002
设备 3 (my-router-003) → 203.0.113.1:6003
```

## 故障排查

### 连接失败

**症状**: 客户端显示"连接失败"

**排查步骤**:
1. 检查服务器地址和端口是否正确
2. 检查 Token 是否匹配
3. 检查服务器防火墙是否开放 7000 端口
4. 查看服务器日志: `journalctl -u outview -f`

### WebRTC 无法建立

**症状**: 状态显示"TCP 降级"

**排查步骤**:
1. 运行调试工具: `./tools/webrtc-debug.sh`
2. 检查 STUN 服务器是否可达
3. 检查 NAT 类型（对称 NAT 需要 TURN）
4. 查看客户端日志: `OUTVIEW_LOG_LEVEL=debug ./outview-client`

详见 [故障排查指南](webrtc-troubleshooting.md)。

### 性能问题

**症状**: 延迟高、卡顿

**优化建议**:
1. 启用 WebRTC（延迟降低 30-50%）
2. 降低 RDP 分辨率和色深
3. 关闭 RDP 桌面背景和动画
4. 检查网络带宽（建议 > 2 Mbps）

## 安全建议

### Token 管理

1. **使用强 Token**
   ```bash
   # 生成 32 字符随机 Token
   openssl rand -base64 32
   ```

2. **定期轮换 Token**
   - 建议每 90 天更换一次
   - 离职员工立即撤销 Token

3. **不要在日志中输出 Token**
   - 客户端日志会自动脱敏
   - 服务端日志仅记录 Token 哈希

### 网络安全

1. **启用 SSL/TLS**
   ```yaml
   server:
     ssl:
       enabled: true
       key-store: classpath:keystore.p12
       key-store-password: changeit
       key-store-type: PKCS12
   ```

2. **配置防火墙**
   ```bash
   # 只允许特定 IP 访问
   sudo ufw allow from 203.0.113.0/24 to any port 7000
   sudo ufw allow from 203.0.113.0/24 to any port 6001:6100
   ```

3. **使用 VPN 叠加**
   - outView + WireGuard/OpenVPN
   - 双重加密保护

### 审计日志

启用详细审计日志：

```yaml
logging:
  level:
    com.outview: INFO
  file:
    name: /var/log/outview/audit.log
    max-size: 100MB
    max-history: 30
```

## 高级配置

### 自定义端口范围

```yaml
outview:
  data-port-range:
    start: 10000
    end: 10100
```

### 连接超时配置

```yaml
outview:
  connection-timeout: 30s
  idle-timeout: 300s
```

### WebRTC 高级配置

```yaml
webrtc:
  enabled: true
  ice-transport-policy: all  # all | relay
  webrtc-timeout: 8s
  dtls-timeout: 10s
  idle-timeout: 60s
```

### 灰度发布配置

```yaml
webrtc:
  gray-release:
    enabled: true
    percentage: 50  # 50% 设备使用 WebRTC
```

## 升级指南

### 从 v1.0.0 升级到 v1.2.0

1. **备份配置文件**
   ```bash
   cp application.yml application.yml.bak
   ```

2. **停止服务**
   ```bash
   sudo systemctl stop outview
   ```

3. **替换 JAR 文件**
   ```bash
   mv outview-server-1.2.0.jar /opt/outview/
   ```

4. **更新配置文件**（添加 WebRTC 配置）

5. **启动服务**
   ```bash
   sudo systemctl start outview
   ```

6. **验证升级**
   ```bash
   curl http://localhost:7000/actuator/health
   ```

### 客户端升级

直接替换可执行文件，配置文件向后兼容。

## 技术支持

- **文档**: https://github.com/outview/outview/tree/main/docs
- **Issues**: https://github.com/outview/outview/issues
- **讨论**: https://github.com/outview/outview/discussions

## 许可证

MIT License - 详见 LICENSE 文件
