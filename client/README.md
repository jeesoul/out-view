# outView Go Client

Go 客户端用于连接 outView 服务器，实现内网穿透。

## 功能特性

- TCP 长连接管理
- 自定义二进制协议 (Magic: 0x4F565753)
- 自动注册和心跳保活
- 数据转发到本地服务 (如 RDP)
- 断线重连
- 跨平台支持
- **图形界面 (GUI)** - 简单易用

## 版本说明

### GUI 版本 (推荐)
提供图形界面，用户只需输入服务器地址和密钥即可连接。

### CLI 版本
命令行版本，适合脚本和自动化场景。

## 编译

### 前提条件
- Go 1.21+
- GUI 版本需要 CGO 支持 (Windows 需要 MinGW 或 MSVC)

### Windows
```batch
REM 编译 GUI 和 CLI 版本
build-gui.bat

REM 仅编译 CLI 版本
build.bat
```

### Linux/macOS
```bash
chmod +x build.sh
./build.sh
```

### 手动编译
```bash
go build -o outview-client ./cmd/outview-client
```

## 使用方法

### 配置文件方式（推荐）
创建 `config.txt`（与exe同目录）：
```
host=your-server.com
port=7000
device-id=your-device-id
token=your-token
local-port=3389
```

双击运行 `outview-client.exe` 即可自动读取配置。

### 命令行参数
```bash
./outview-client -host <服务器地址> -port <控制端口> -device-id <设备ID> -token <密钥> -local-port <本地端口>
```

### 参数说明
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-host` | 服务器地址 | localhost |
| `-port` | 服务器控制端口 | 7000 |
| `-device-id` | 设备 ID (必需) | - |
| `-token` | 认证密钥 (必需) | - |
| `-local-port` | 本地服务端口 | 3389 (RDP) |
| `-heartbeat` | 心跳间隔 (秒) | 30 |
| `-config` | 指定配置文件路径 | 自动检测 |
| `-version` | 显示版本信息 | - |

### 环境变量
也可以通过环境变量配置:
- `OUTVIEW_SERVER_HOST` - 服务器地址
- `OUTVIEW_SERVER_PORT` - 服务器端口
- `OUTVIEW_DEVICE_ID` - 设备 ID
- `OUTVIEW_TOKEN` - 认证密钥
- `OUTVIEW_LOCAL_PORT` - 本地端口

## 示例

### 连接远程服务器
```bash
./outview-client -host 192.168.1.100 -device-id my-device-001 -token secret-token-123
```

### 转发其他服务
```bash
# 转发 SSH (端口 22)
./outview-client -host server.com -device-id my-device -token secret -local-port 22

# 转发 HTTP (端口 8080)
./outview-client -host server.com -device-id my-device -token secret -local-port 8080
```

## 协议格式

### 消息头 (12 字节)
| 字段 | 长度 | 说明 |
|------|------|------|
| Magic | 4B | 0x4F565753 ("OVWS") |
| Version | 1B | 协议版本 (1) |
| Type | 1B | 消息类型 |
| Length | 4B | 消息体长度 |
| Reserved | 2B | 保留字段 |

### 消息类型
| 值 | 类型 | 说明 |
|----|------|------|
| 1 | REGISTER | 注册请求 |
| 2 | HEARTBEAT | 心跳请求 |
| 3 | DATA | 数据转发 |
| 4 | ERROR | 错误消息 |
| 5 | REGISTER_ACK | 注册响应 |
| 6 | HEARTBEAT_ACK | 心跳响应 |

## 项目结构

```
client/
├── cmd/
│   └── outview-client/
│       └── main.go          # 主入口
├── internal/
│   ├── protocol/
│   │   ├── constants.go     # 协议常量
│   │   ├── message.go       # 消息结构
│   │   ├── encoder.go       # 编码器
│   │   └── decoder.go       # 解码器
│   └── client/
│       ├── client.go        # 客户端核心
│       ├── config.go        # 配置管理
│       └── proxy.go         # 本地代理
├── go.mod
├── build.bat                # Windows 构建脚本
└── build.sh                 # Linux/macOS 构建脚本
```

## 工作流程

1. 客户端连接服务器控制端口 (默认 7000)
2. 发送注册消息 (包含 deviceId, token, localPort)
3. 服务器分配外部访问端口并返回
4. 客户端开始心跳保活 (默认 30 秒)
5. 外部用户通过数据端口访问时，服务器转发数据到客户端
6. 客户端将数据转发到本地服务 (如 RDP 3389)
7. 客户端将本地服务响应发回服务器