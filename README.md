# outView

<p align="center">
  <strong>🚀 高性能远程桌面内网穿透解决方案</strong>
</p>

<p align="center">
  <a href="#功能特性">功能特性</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#配置说明">配置说明</a> •
  <a href="#编译构建">编译构建</a> •
  <a href="USER_MANUAL.md">用户手册</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/version-1.0.0-blue.svg" alt="Version">
  <img src="https://img.shields.io/badge/java-8%2B-orange.svg" alt="Java">
  <img src="https://img.shields.io/badge/go-1.21%2B-00ADD8.svg" alt="Go">
  <img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License">
</p>

---

## 简介

**outView** 是一款轻量级、高性能的远程桌面内网穿透系统。无需复杂的网络配置，即可让您从外部网络安全地访问内网电脑的远程桌面服务。

### 核心优势

- 🎯 **零配置启动** - 开箱即用，5分钟即可完成部署
- ⚡ **高性能转发** - 基于 Netty NIO 框架，支持高并发连接
- 🔐 **安全认证** - Token 机制 + 可选 SSL/TLS 加密
- 🌐 **跨平台支持** - 服务端 Java，客户端 Go，支持 Windows/Linux/macOS
- 📦 **轻量级** - 服务端 JAR 仅 29MB，客户端 exe 仅 2.4MB

---

## 架构图

```
┌─────────────────┐                    ┌─────────────────┐                    ┌─────────────────┐
│    外出电脑      │                    │    公网服务器    │                    │    家庭电脑      │
│    (电脑 A)      │                    │   outView 服务端 │                    │   outView 客户端 │
│                 │     RDP 连接        │                 │     隧道转发       │                 │
│   ┌─────────┐   │ ──────────────────> │   ┌─────────┐   │ <────────────────── │   ┌─────────┐   │
│   │  mstsc  │   │   :6001 (数据端口)   │   │  Netty  │   │   7000 (控制端口)   │   │  Go客户端│   │
│   └─────────┘   │                    │   └─────────┘   │                    │   └─────────┘   │
│                 │                    │                 │                    │        │        │
│                 │                    │                 │                    │        ▼        │
│                 │                    │                 │                    │   ┌─────────┐   │
│                 │                    │                 │                    │   │ RDP:3389│   │
│                 │                    │                 │                    │   └─────────┘   │
└─────────────────┘                    └─────────────────┘                    └─────────────────┘
```

---

## 功能特性

| 特性 | 描述 | 状态 |
|------|------|:----:|
| **高性能服务端** | Spring Boot 2.7 + Netty 4.1，支持高并发 | ✅ |
| **跨平台客户端** | Go 1.21+ 编写，支持 Windows/Linux/macOS | ✅ |
| **动态端口分配** | 自动分配 6000-6500 数据端口 | ✅ |
| **Token 认证** | 自动生成设备ID和密钥，支持有效期管理 | ✅ |
| **心跳保活** | 30秒间隔心跳，90秒超时自动断开 | ✅ |
| **双向数据转发** | 完整支持 RDP 协议，多连接ID管理 | ✅ |
| **配置文件支持** | 支持 config.txt，简化部署流程 | ✅ |
| **SSL/TLS 加密** | 支持自签名证书和 CA 证书 | ✅ |
| **管理后台** | Web UI 管理设备、生成 Token | ✅ |

---

## 快速开始

### 环境要求

| 组件 | 版本 | 用途 |
|------|------|------|
| JDK | 8+ | 运行服务端 |
| Maven | 3.6+ | 编译服务端（可选） |
| Go | 1.21+ | 编译客户端（可选） |

### 1. 部署服务端

```bash
# 方式一：直接运行 JAR
java -jar outview-server.jar

# 方式二：从源码编译
mvn package -DskipTests
java -jar target/outview-server.jar
```

### 2. 生成 Token

访问管理后台 `http://服务器IP:8080`，点击 **"生成新 Token"**

记录返回的 `deviceId` 和 `token`

### 3. 运行客户端

**命令行方式：**
```bash
outview-client.exe -host 服务器IP -port 7000 -device-id 设备ID -token 密钥
```

**配置文件方式：**

创建 `config.txt`（与 exe 同目录）：
```
host=your-server.com
port=7000
device-id=your-device-id
token=your-token
local-port=3389
```

双击 `outview-client.exe` 即可自动读取配置

### 4. 连接远程桌面

1. 打开 Windows 远程桌面连接 (`Win + R` → `mstsc`)
2. 输入 `服务器IP:分配端口`（如 `example.com:6001`）
3. 输入家庭电脑的 Windows 用户名和密码

---

## 端口说明

| 端口 | 用途 | 说明 |
|------|------|------|
| **8080** | HTTP API | 管理后台、Token 生成、设备管理 |
| **7000** | 控制端口 | 客户端注册、心跳保活 |
| **6000-6500** | 数据端口 | RDP 数据转发，自动分配 |

---

## 配置说明

### 服务端配置 (`application.yml`)

```yaml
outview:
  control-port: 7000          # 客户端连接端口
  data-port-start: 6000       # 数据端口范围起始
  data-port-end: 6500         # 数据端口范围结束
  heartbeat-timeout: 90       # 心跳超时时间（秒）
  heartbeat-interval: 30      # 心跳间隔（秒）
  token-expire-days: 30       # Token 有效期（天）
  ssl:
    enabled: false            # 生产环境建议启用
    use-self-signed: true     # 开发环境可使用自签名证书
```

### 客户端参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `host` | 服务器地址 | - |
| `port` | 服务器控制端口 | `7000` |
| `device-id` | 设备ID（必需） | - |
| `token` | 认证密钥（必需） | - |
| `local-port` | 本地服务端口 | `3389` (RDP) |
| `heartbeat` | 心跳间隔（秒） | `30` |
| `config` | 指定配置文件路径 | 自动检测 |

---

## 编译构建

### 服务端

```bash
mvn clean package -DskipTests
```

### 客户端

```bash
cd client

# 安装依赖
go mod tidy

# 编译 CLI 版本（推荐，无需 CGO）
set CGO_ENABLED=0
go build -ldflags "-s -w" -o outview-client.exe ./cmd/outview-client

# 编译 GUI 版本（需要 CGO 和 C 编译器）
build-gui.bat
```

---

## 自定义协议

outView 使用自定义二进制协议进行通信，消息头固定 12 字节：

| 字段 | 长度 | 说明 |
|------|------|------|
| Magic | 4B | `0x4F565753` ("OVWS") |
| Version | 1B | 协议版本 (1) |
| Type | 1B | 消息类型 |
| Length | 4B | 消息体长度 |
| Reserved | 2B | 保留字段 |

**消息类型：**

| 值 | 类型 | 说明 |
|:--:|------|------|
| 1 | REGISTER | 注册请求 |
| 2 | HEARTBEAT | 心跳请求 |
| 3 | DATA | 数据转发 |
| 4 | ERROR | 错误消息 |
| 5 | REGISTER_ACK | 注册响应 |
| 6 | HEARTBEAT_ACK | 心跳响应 |

---

## 常见问题

<details>
<summary><b>客户端连接失败？</b></summary>

- 检查服务器是否正常运行
- 检查防火墙是否开放 7000 端口
- 确认 deviceId 和 token 是否正确
- 检查网络连通性：`telnet 服务器IP 7000`
</details>

<details>
<summary><b>远程桌面连接不上？</b></summary>

- 确认客户端显示"注册成功"和分配的端口
- 使用正确的数据端口（如 6001），不是控制端口 7000
- 检查防火墙是否开放 6000-6500 端口
- 确认家庭电脑开启了远程桌面功能
</details>

<details>
<summary><b>连接后黑屏或断开？</b></summary>

- 确认家庭电脑的 RDP 服务正常运行（端口 3389）
- 确认家庭电脑防火墙允许 3389 入站连接
- 检查网络稳定性
</details>

<details>
<summary><b>如何管理多台电脑？</b></summary>

- 每台电脑使用不同的 deviceId
- 每台电脑会自动分配不同的数据端口
- 通过不同端口区分连接哪台电脑
</details>

---

## 项目结构

```
out-view/
├── src/main/java/com/outview/       # 服务端源码
│   ├── config/                      # 配置类
│   ├── controller/                  # REST API
│   ├── netty/                       # Netty 核心
│   │   ├── handler/                 # 消息处理器
│   │   └── ssl/                     # SSL 支持
│   ├── protocol/                    # 协议实现
│   └── service/                     # 业务服务
├── client/                          # Go 客户端源码
│   ├── cmd/outview-client/          # CLI 入口
│   ├── cmd/outview-gui/             # GUI 入口
│   └── internal/                    # 内部模块
│       ├── protocol/                # 协议实现
│       └── client/                  # 客户端核心
├── pom.xml                          # Maven 配置
└── README.md                        # 本文件
```

---

## 技术栈

**服务端：**
- Spring Boot 2.7.18
- Netty 4.1.100
- Fastjson

**客户端：**
- Go 1.21+
- Fyne (GUI，可选)

---

## 贡献指南

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

---

## License

本项目基于 [MIT License](LICENSE) 开源。

---

<p align="center">
  Made with ❤️ by <a href="https://github.com/jeesoul">jeesoul</a>
</p>