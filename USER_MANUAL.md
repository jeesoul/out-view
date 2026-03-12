# outView 用户使用手册

## 目录

1. [系统概述](#1-系统概述)
2. [快速开始](#2-快速开始)
3. [服务端部署](#3-服务端部署)
4. [客户端配置](#4-客户端配置)
5. [远程桌面连接](#5-远程桌面连接)
6. [管理后台](#6-管理后台)
7. [API 接口文档](#7-api-接口文档)
8. [故障排除](#8-故障排除)
9. [安全建议](#9-安全建议)
10. [常见问题](#10-常见问题)

---

## 1. 系统概述

### 1.1 产品简介

outView 是一款远程桌面内网穿透系统，允许您从外部网络访问家庭或办公室内的电脑。通过该系统，您可以：

- 在外出时远程访问家庭电脑
- 访问公司内网的远程桌面服务
- 无需公网 IP 即可实现远程连接

### 1.2 系统架构

```
┌──────────────────────────────────────────────────────────────────┐
│                         外出电脑 (用户)                           │
│  ┌─────────────────┐                                              │
│  │ 远程桌面连接     │                                              │
│  │ mstsc           │                                              │
│  └─────────────────┘                                              │
└──────────────────────────────────────────────────────────────────┘
                             │
                             │ RDP 协议
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│                      outView Server (公网服务器)                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐    │
│  │ 数据端口      │    │ 数据转发     │    │ 控制端口 (7000)  │    │
│  │ 6000-6500    │    │ ProxyHandler │    │ AuthHandler     │    │
│  └──────────────┘    └──────────────┘    └──────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
                                                 │
                                                 │ 自定义协议
                                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│                      家庭电脑 (内网)                              │
│  ┌──────────────┐    ┌──────────────┐                            │
│  │ RDP 服务     │◄───│ outView      │                            │
│  │ :3389        │    │ Client       │                            │
│  └──────────────┘    └──────────────┘                            │
└──────────────────────────────────────────────────────────────────┘
```

### 1.3 技术规格

| 项目 | 规格 |
|------|------|
| 协议 | 自定义二进制协议 + RDP |
| 控制端口 | 7000 |
| 数据端口范围 | 6000-6500 |
| 心跳间隔 | 30 秒 |
| 心跳超时 | 90 秒 |
| 支持客户端数 | 最多 500 个并发 |

---

## 2. 快速开始

### 2.1 环境要求

| 组件 | 要求 |
|------|------|
| Java | JDK 8 或更高版本 |
| 操作系统 | Windows / Linux / macOS |
| 网络 | 需要一台具有公网 IP 的服务器 |

### 2.2 五分钟快速部署

**步骤 1: 编译打包**

```bash
# Windows
set JAVA_HOME=C:\Program Files\Java\jdk-1.8
mvn clean package -DskipTests

# Linux/Mac
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk
mvn clean package -DskipTests
```

**步骤 2: 启动服务端**

```bash
java -jar target/outview-server.jar
```

**步骤 3: 生成 Token**

访问 `http://服务器IP:8080/index.html`，点击"生成新 Token"。

**步骤 4: 启动客户端**

```bash
java -cp target/outview-server.jar com.outview.client.OutViewClientTest <服务器IP> 7000 <设备ID> <Token> 3389
```

**步骤 5: 连接远程桌面**

打开远程桌面连接，输入 `服务器IP:分配的端口`（如 `192.168.1.100:6001`）。

---

## 3. 服务端部署

### 3.1 配置文件说明

配置文件位置: `src/main/resources/application.yml`

```yaml
server:
  port: 8080                    # HTTP 服务端口

outview:
  control-port: 7000            # 客户端注册端口
  data-port-start: 6000         # 数据端口范围起始
  data-port-end: 6500           # 数据端口范围结束
  heartbeat-timeout: 90         # 心跳超时时间（秒）
  heartbeat-interval: 30        # 心跳间隔（秒）
  token-expire-days: 30         # Token 有效期（天）

logging:
  level:
    com.outview: DEBUG          # 日志级别
```

### 3.2 启动方式

**前台启动（调试用）:**

```bash
java -jar outview-server.jar
```

**后台启动（生产环境）:**

```bash
# Linux
nohup java -jar outview-server.jar > outview.log 2>&1 &

# 查看日志
tail -f outview.log
```

**使用 systemd 服务（推荐）:**

创建服务文件 `/etc/systemd/system/outview.service`:

```ini
[Unit]
Description=outView Server
After=network.target

[Service]
Type=simple
User=outview
WorkingDirectory=/opt/outview
ExecStart=/usr/bin/java -jar /opt/outview/outview-server.jar
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务:

```bash
sudo systemctl daemon-reload
sudo systemctl enable outview
sudo systemctl start outview
```

### 3.3 防火墙配置

**Linux (iptables):**

```bash
# 开放必要端口
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT   # HTTP API
iptables -A INPUT -p tcp --dport 7000 -j ACCEPT   # 控制端口
iptables -A INPUT -p tcp --dport 6000:6500 -j ACCEPT  # 数据端口

# 保存规则
service iptables save
```

**Linux (firewalld):**

```bash
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --permanent --add-port=7000/tcp
firewall-cmd --permanent --add-port=6000-6500/tcp
firewall-cmd --reload
```

**Windows:**

```powershell
netsh advfirewall firewall add rule name="outView-HTTP" dir=in action=allow protocol=TCP localport=8080
netsh advfirewall firewall add rule name="outView-Control" dir=in action=allow protocol=TCP localport=7000
netsh advfirewall firewall add rule name="outView-Data" dir=in action=allow protocol=TCP localport=6000-6500
```

---

## 4. 客户端配置

### 4.1 获取 Token

**方式一: 管理后台**

1. 打开浏览器访问 `http://服务器IP:8080/index.html`
2. 点击"生成新 Token"按钮
3. 记录显示的 **设备ID** 和 **Token**

**方式二: API 接口**

```bash
curl -X POST http://服务器IP:8080/api/tokens

# 响应示例
{
  "success": true,
  "deviceId": "a1b2c3d4e5f6g7h8",
  "token": "x9y8z7w6v5u4t3s2r1q0"
}
```

### 4.2 运行客户端

**交互式客户端:**

```bash
java -cp outview-server.jar com.outview.client.OutViewClient
```

按提示输入:
- 服务器地址
- 服务器端口 (默认 7000)
- 设备ID
- Token
- 本地 RDP 端口 (默认 3389)

**测试客户端（自动化）:**

```bash
java -cp outview-server.jar com.outview.client.OutViewClientTest <服务器IP> 7000 <设备ID> <Token> 3389
```

### 4.3 客户端日志说明

正常连接日志示例:

```
====================================
outView Client Test
====================================
Server: 192.168.1.100:7000
DeviceId: a1b2c3d4e5f6g7h8
Token: x9y8z7w6v5u4t3s2r1q0
LocalPort: 3389
====================================

Connected to server: 192.168.1.100:7000
Sent register request
Register response: {"success":true,"deviceId":"a1b2c3d4e5f6g7h8","externalPort":6001}
Heartbeat #1 sent
Heartbeat #2 sent
Test completed successfully!
```

### 4.4 Windows 客户端安装为服务（可选）

使用 WinSW 将客户端安装为 Windows 服务:

1. 下载 WinSW: https://github.com/winsw/winsw/releases
2. 创建配置文件 `outview-client.xml`:

```xml
<service>
  <id>outview-client</id>
  <name>outView Client</name>
  <description>Remote Desktop Tunneling Client</description>
  <executable>java</executable>
  <arguments>-cp outview-server.jar com.outview.client.OutViewClient</arguments>
  <workingdirectory>C:\outview</workingdirectory>
  <logpath>C:\outview\logs</logpath>
  <log mode="roll-by-size">
    <sizeThreshold>10240</sizeThreshold>
    <keepFiles>8</keepFiles>
  </log>
</service>
```

3. 安装服务:

```cmd
WinSW.exe install outview-client.xml
WinSW.exe start outview-client.xml
```

---

## 5. 远程桌面连接

### 5.1 确认客户端状态

在连接前，请确认:

1. 客户端已成功连接服务器
2. 管理后台显示设备在线
3. 已记录分配的对外端口

### 5.2 使用 Windows 远程桌面

1. 按 `Win + R` 打开运行对话框
2. 输入 `mstsc` 并回车
3. 在"计算机"字段输入: `服务器IP:对外端口`
   - 例如: `192.168.1.100:6001`
4. 点击"连接"
5. 输入家庭电脑的用户名和密码

### 5.3 使用其他 RDP 客户端

**Mac (Microsoft Remote Desktop):**

1. 从 App Store 安装 Microsoft Remote Desktop
2. 添加远程资源:
   - PC 名称: `服务器IP:对外端口`
   - 用户名: 家庭电脑用户名
   - 密码: 家庭电脑密码

**Linux (Remmina):**

1. 安装 Remmina: `sudo apt install remmina`
2. 创建新连接:
   - 协议: RDP
   - 服务器: `服务器IP:对外端口`
   - 用户名/密码: 家庭电脑凭据

### 5.4 连接优化设置

在远程桌面连接中，点击"显示选项"进行优化:

**体验选项卡:**
- 连接速度: 选择"LAN (10 Mbps 或更高)"
- 取消勾选"字体平滑"可提高性能

**显示选项卡:**
- 分辨率: 根据需要调整
- 全屏时显示连接栏: 建议勾选

---

## 6. 管理后台

### 6.1 访问管理后台

打开浏览器访问: `http://服务器IP:8080/index.html`

### 6.2 功能说明

**统计面板:**
- 在线设备数
- 总设备数
- 端口映射数

**Token 管理:**
- 生成新 Token
- 查看已生成的 Token

**设备管理:**
- 查看在线设备列表
- 查看设备状态
- 强制断开设备

**端口映射:**
- 查看所有端口映射
- 对外端口与设备的对应关系

### 6.3 自动刷新

管理后台每 10 秒自动刷新数据。

---

## 7. API 接口文档

### 7.1 健康检查

```
GET /health
```

**响应:**

```json
{
  "status": "UP",
  "timestamp": 1704067200000
}
```

### 7.2 设备管理

**获取设备列表:**

```
GET /api/devices
```

**响应:**

```json
{
  "total": 2,
  "online": 2,
  "devices": [
    {
      "deviceId": "device-001",
      "externalPort": 6001,
      "localPort": 3389,
      "status": "ONLINE",
      "lastHeartbeat": "2024-01-01T12:00:00",
      "createTime": "2024-01-01T11:00:00"
    }
  ]
}
```

**获取单个设备:**

```
GET /api/devices/{deviceId}
```

**强制断开设备:**

```
DELETE /api/devices/{deviceId}
```

**获取端口映射:**

```
GET /api/devices/mappings
```

### 7.3 Token 管理

**生成 Token:**

```
POST /api/tokens
Content-Type: application/json
```

**响应:**

```json
{
  "success": true,
  "deviceId": "a1b2c3d4e5f6g7h8",
  "token": "x9y8z7w6v5u4t3s2r1q0",
  "createTime": "2024-01-01T12:00:00.000+00:00"
}
```

**查询 Token 状态:**

```
GET /api/tokens/{deviceId}
```

---

## 8. 故障排除

### 8.1 客户端无法连接服务器

**检查步骤:**

1. **验证服务器运行状态:**
   ```bash
   curl http://服务器IP:8080/health
   ```
   应返回 `{"status":"UP",...}`

2. **检查防火墙:**
   ```bash
   # Linux
   iptables -L -n | grep 7000

   # Windows
   netsh advfirewall firewall show rule name="outView-Control"
   ```

3. **检查网络连通性:**
   ```bash
   telnet 服务器IP 7000
   ```

4. **检查 Token 是否正确:**
   - 确认设备ID和Token完全匹配
   - 注意大小写

### 8.2 远程桌面连接失败

**检查步骤:**

1. **确认客户端在线:**
   - 检查管理后台设备状态
   - 确认状态为 ONLINE

2. **确认端口正确:**
   - 使用分配的对外端口（如 6001）
   - 不是控制端口 7000

3. **确认本地 RDP 服务运行:**
   - Windows: 服务管理器检查 "Remote Desktop Services"
   - 确认 RDP 端口为 3389（或自定义端口）

4. **检查数据端口防火墙:**
   ```bash
   # 测试数据端口
   telnet 服务器IP 6001
   ```

### 8.3 连接频繁断开

**可能原因:**

1. **心跳超时:**
   - 检查网络稳定性
   - 确认心跳配置正确

2. **服务器资源不足:**
   - 检查服务器 CPU、内存使用率
   - 查看服务器日志

3. **网络问题:**
   - 使用更稳定的网络连接
   - 考虑使用有线网络

### 8.4 查看日志

**服务端日志:**

```bash
# 查看实时日志
tail -f outview.log

# 搜索错误
grep -i error outview.log
```

**客户端日志:**

客户端输出直接显示在控制台。

---

## 9. 安全建议

### 9.1 网络安全

1. **使用 TLS 加密:**
   - 配置 HTTPS
   - 使用 SSL 证书

2. **限制访问 IP:**
   ```bash
   # iptables 限制特定 IP
   iptables -A INPUT -p tcp --dport 7000 -s 允许的IP -j ACCEPT
   iptables -A INPUT -p tcp --dport 7000 -j DROP
   ```

3. **使用强 Token:**
   - Token 应足够长且随机
   - 定期更换 Token

### 9.2 服务器安全

1. **使用非 root 用户运行:**
   ```bash
   useradd -r -s /bin/false outview
   sudo -u outview java -jar outview-server.jar
   ```

2. **定期更新系统:**
   ```bash
   # Ubuntu/Debian
   apt update && apt upgrade

   # CentOS/RHEL
   yum update
   ```

3. **配置日志轮转:**
   ```bash
   # /etc/logrotate.d/outview
   /opt/outview/outview.log {
       daily
       rotate 7
       compress
       missingok
       notifempty
   }
   ```

### 9.3 客户端安全

1. **保护 Token:**
   - 不要在公共场所分享 Token
   - 定期更换 Token

2. **启用 Windows 远程桌面安全:**
   - 设置强密码
   - 启用网络级别身份验证 (NLA)

---

## 10. 常见问题

### Q1: 服务端启动失败，端口被占用？

**解决:**
```bash
# 查找占用端口的进程
netstat -tlnp | grep 7000

# 或更改配置文件中的端口
```

### Q2: 客户端显示注册成功，但远程桌面无法连接？

**检查:**
1. 确认使用正确的对外端口（不是 7000）
2. 确认本地 RDP 服务正在运行
3. 确认 Windows 防火墙允许 RDP

### Q3: 连接后画面卡顿？

**优化:**
1. 在远程桌面设置中降低分辨率
2. 取消"字体平滑"选项
3. 检查网络带宽

### Q4: 如何查看当前有多少客户端连接？

访问管理后台 `http://服务器IP:8080/index.html` 或调用 API:
```bash
curl http://服务器IP:8080/api/devices
```

### Q5: 如何强制断开某个客户端？

```bash
curl -X DELETE http://服务器IP:8080/api/devices/设备ID
```

或在管理后台点击"断开"按钮。

### Q6: 服务端支持多少并发连接？

理论上支持 500+ 并发连接，实际性能取决于服务器硬件配置。

### Q7: Token 过期后怎么办？

重新生成新的 Token，并在客户端更新配置。

---

## 附录

### A. 错误代码表

| 代码 | 说明 |
|------|------|
| TYPE_ERROR (4) | 协议错误 |
| Invalid magic number | 无效的魔数 |
| Invalid register parameters | 注册参数无效 |
| No available port | 无可用端口 |
| Heartbeat timeout | 心跳超时 |

### B. 协议格式

**消息头 (12 字节):**

| 字段 | 长度 | 说明 |
|------|------|------|
| Magic | 4B | 0x4F565753 ("OVWS") |
| Version | 1B | 协议版本 (1) |
| Type | 1B | 消息类型 |
| Length | 4B | Body 长度 |
| Reserved | 2B | 保留 |

**消息类型:**

| 类型值 | 名称 | 说明 |
|--------|------|------|
| 1 | REGISTER | 注册请求 |
| 5 | REGISTER_ACK | 注册响应 |
| 2 | HEARTBEAT | 心跳请求 |
| 6 | HEARTBEAT_ACK | 心跳响应 |
| 3 | DATA | 数据转发 |
| 4 | ERROR | 错误消息 |

### C. 联系支持

如有问题，请通过以下方式获取支持:
- 项目地址: [GitHub Repository]
- 问题反馈: [Issues]

---

*文档版本: 1.0.0*
*最后更新: 2024-01-01*