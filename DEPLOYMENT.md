# outView 部署与使用指南

## 一、环境要求

- **Java**: JDK 8+
- **Maven**: 3.6+ (用于编译打包)
- **操作系统**: Windows / Linux / macOS

## 二、服务端部署

### 2.1 编译打包

```bash
# 进入项目目录
cd out-view

# 编译打包
mvn clean package -DskipTests

# 生成的 jar 包位于
# target/outview-server.jar
```

### 2.2 配置文件

创建 `application.yml` 或使用命令行参数：

```yaml
server:
  port: 8080

outview:
  control-port: 7000      # 客户端注册端口
  data-port-start: 6000   # 数据端口范围起始
  data-port-end: 6500     # 数据端口范围结束
  heartbeat-timeout: 90   # 心跳超时(秒)
```

### 2.3 启动服务

**Windows:**
```cmd
java -jar outview-server.jar
```

**Linux (后台运行):**
```bash
nohup java -jar outview-server.jar > outview.log 2>&1 &
```

**带参数启动:**
```bash
java -jar outview-server.jar --outview.control-port=7000 --server.port=8080
```

### 2.4 防火墙配置

需要开放以下端口：
- **8080**: HTTP API / 管理后台
- **7000**: 客户端控制端口
- **6000-6500**: 数据转发端口

**Linux iptables:**
```bash
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
iptables -A INPUT -p tcp --dport 7000 -j ACCEPT
iptables -A INPUT -p tcp --dport 6000:6500 -j ACCEPT
```

**Windows 防火墙:**
```powershell
netsh advfirewall firewall add rule name="outView" dir=in action=allow protocol=TCP localport=8080,7000,6000-6500
```

## 三、客户端使用

### 3.1 生成 Token

访问管理后台 `http://服务器IP:8080/index.html`，点击"生成新 Token"。

或使用 API：
```bash
curl -X POST http://服务器IP:8080/api/tokens
```

返回示例：
```json
{
  "success": true,
  "deviceId": "a1b2c3d4e5f6g7h8",
  "token": "x9y8z7w6v5u4t3s2r1q0"
}
```

### 3.2 客户端配置

创建 `client-config.json`：
```json
{
  "serverHost": "你的服务器IP",
  "serverPort": 7000,
  "deviceId": "a1b2c3d4e5f6g7h8",
  "token": "x9y8z7w6v5u4t3s2r1q0",
  "localPort": 3389
}
```

### 3.3 运行客户端

**Java 客户端:**
```bash
java -jar outview-client.jar client-config.json
```

### 3.4 连接远程桌面

1. 在外出电脑上打开"远程桌面连接" (mstsc)
2. 输入地址：`服务器IP:分配的对外端口`（如 `192.168.1.100:6001`）
3. 输入家庭电脑的用户名和密码

## 四、系统架构图

```
┌──────────────────────────────────────────────────────────────────┐
│                         外出电脑 (用户)                           │
│  ┌─────────────────┐                                              │
│  │ 远程桌面连接     │ ──────┐                                      │
│  │ mstsc           │       │                                      │
│  └─────────────────┘       │                                      │
└────────────────────────────│─────────────────────────────────────┘
                             │ RDP 协议
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│                      outView Server (公网)                        │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐    │
│  │ 数据端口      │◄───│ 数据转发     │───►│ 控制端口 (7000)  │    │
│  │ 6000-6500    │    │ ProxyHandler │    │ AuthHandler     │    │
│  └──────────────┘    └──────────────┘    └──────────────────┘    │
│                                                ▲                 │
└────────────────────────────────────────────────│─────────────────┘
                                                 │ 自定义协议
                                                 │
┌────────────────────────────────────────────────│─────────────────┐
│                      家庭电脑 (内网)            │                 │
│  ┌──────────────┐    ┌──────────────┐          │                 │
│  │ RDP 服务     │◄───│ outView      │──────────┘                 │
│  │ :3389        │    │ Client       │                            │
│  └──────────────┘    └──────────────┘                            │
└──────────────────────────────────────────────────────────────────┘
```

## 五、常见问题

### Q1: 客户端连接失败？
- 检查服务器防火墙是否开放 7000 端口
- 检查 Token 是否正确
- 查看服务器日志

### Q2: 远程桌面无法连接？
- 确认客户端已成功注册（查看管理后台）
- 确认使用正确的对外端口
- 检查数据端口是否开放

### Q3: 连接后断开？
- 检查心跳超时配置
- 查看网络稳定性
- 检查服务器日志