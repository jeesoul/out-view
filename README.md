# outView - 远程桌面内网穿透系统

让外部电脑A通过公网服务器远程连接内网电脑B的RDP服务。

## 使用场景

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│   电脑 A        │         │  公网服务器      │         │   电脑 B        │
│   (外出电脑)     │  RDP    │  your-server    │  隧道   │   (家庭电脑)     │
│                 │ ──────> │                 │ <────── │                 │
│  运行 mstsc     │         │  运行服务端      │         │  运行客户端      │
└─────────────────┘         └─────────────────┘         └─────────────────┘
```

## 项目状态

| 模块 | 状态 | 说明 |
|------|------|------|
| 服务端核心 | ✅ 完成 | Spring Boot 2.7 + Netty 4.1 |
| 数据端口监听 | ✅ 完成 | 动态分配 6000-6500 |
| 数据转发 | ✅ 完成 | 双向转发 + 连接ID管理 |
| Token认证 | ✅ 完成 | 自动生成 + 有效期管理 |
| 心跳保活 | ✅ 完成 | 30秒间隔 / 90秒超时 |
| SSL/TLS | ✅ 完成 | 自签名证书 + CA证书支持 |
| Go客户端 | ✅ 完成 | CLI版本已编译 |
| 配置文件支持 | ✅ 完成 | 支持config.txt |

## 快速开始

### 1. 部署服务端

```bash
# 编译
mvn package -DskipTests

# 运行
java -jar target/outview-server.jar
```

### 2. 生成Token

访问 `http://服务器IP:8080/index.html`，点击"生成新 Token"

记录返回的 `deviceId` 和 `token`

### 3. 运行客户端

**方式一：命令行参数**
```bash
outview-client.exe -host 服务器IP -port 7000 -device-id 设备ID -token 密钥
```

**方式二：配置文件**

创建 `config.txt`（与exe同目录）：
```
host=your-server.com
port=7000
device-id=your-device-id
token=your-token
local-port=3389
```

双击运行 `outview-client.exe` 即可

### 4. 连接远程桌面

1. 打开 Windows 远程桌面连接 (Win+R → mstsc)
2. 输入 `服务器IP:分配端口`（如 `120.27.214.55:6001`）
3. 输入电脑B的Windows用户名和密码

## 端口说明

| 端口 | 用途 | 说明 |
|------|------|------|
| 8080 | HTTP API | 管理后台、Token生成 |
| 7000 | 控制端口 | 客户端注册、心跳 |
| 6000-6500 | 数据端口 | RDP转发，自动分配 |

## 配置说明

### 服务端配置 (application.yml)

```yaml
outview:
  control-port: 7000        # 客户端连接端口
  data-port-start: 6000     # 数据端口范围
  data-port-end: 6500
  heartbeat-timeout: 90     # 心跳超时(秒)
  heartbeat-interval: 30    # 心跳间隔(秒)
  token-expire-days: 30     # Token有效期(天)
  ssl:
    enabled: false          # 生产环境建议启用
```

### 客户端配置

| 参数 | 说明 | 默认值 |
|------|------|--------|
| host | 服务器地址 | - |
| port | 服务器端口 | 7000 |
| device-id | 设备ID | - |
| token | 认证密钥 | - |
| local-port | 本地服务端口 | 3389 (RDP) |
| heartbeat | 心跳间隔(秒) | 30 |

## 目录结构

```
out-view/
├── src/main/java/com/outview/    # 服务端源码
│   ├── config/                   # 配置类
│   ├── controller/               # REST API
│   ├── netty/                    # Netty核心
│   │   ├── handler/              # 消息处理器
│   │   └── ssl/                  # SSL支持
│   ├── protocol/                 # 协议实现
│   └── service/                  # 业务服务
├── client/                       # Go客户端源码
│   ├── cmd/outview-client/       # CLI入口
│   ├── internal/                 # 内部模块
│   │   ├── protocol/             # 协议实现
│   │   └── client/               # 客户端核心
│   └── go.mod                    # Go依赖
└── pom.xml                       # Maven配置
```

## 依赖要求

| 组件 | 版本 | 说明 |
|------|------|------|
| JDK | 8+ | 服务端运行 |
| Maven | 3.6+ | 服务端编译 |
| Go | 1.21+ | 客户端编译（可选） |

## 编译客户端

```bash
cd client

# 安装依赖
go mod tidy

# 编译 CLI 版本
set CGO_ENABLED=0
go build -ldflags "-s -w" -o outview-client.exe ./cmd/outview-client
```

## 常见问题

### Q: 客户端显示连接失败？
- 检查服务器是否启动
- 检查防火墙是否开放7000端口
- 检查deviceId和token是否正确

### Q: 远程桌面连接不上？
- 确认客户端显示"注册成功"和分配的端口
- 确认使用正确的端口（如6001，不是7000）
- 检查防火墙是否开放6000-6500端口

### Q: 连接后黑屏或断开？
- 确认电脑B开启了远程桌面（系统属性→远程）
- 确认电脑B的RDP服务正常运行（默认端口3389）
- 确认电脑B防火墙允许3389入站

### Q: 多台电脑怎么办？
- 每台电脑使用不同的deviceId
- 每台电脑会分配不同的数据端口
- 通过不同端口区分连接哪台电脑

## 协议格式

自定义二进制协议，12字节消息头：

| 字段 | 长度 | 说明 |
|------|------|------|
| Magic | 4B | 0x4F565753 ("OVWS") |
| Version | 1B | 协议版本(1) |
| Type | 1B | 消息类型 |
| Length | 4B | 消息体长度 |
| Reserved | 2B | 保留字段 |

消息类型：REGISTER(1), HEARTBEAT(2), DATA(3), ERROR(4), REGISTER_ACK(5), HEARTBEAT_ACK(6)

## License

MIT License