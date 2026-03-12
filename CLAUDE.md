# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**outView** - 远程桌面内网穿透系统，基于 Spring Boot 2.7 + Netty 构建高性能 TCP 隧道代理服务。

### 核心功能
- 客户端通过控制端口注册并建立长连接隧道
- 外部用户通过数据端口访问内网远程桌面 (RDP)
- 服务端实现双向数据转发（零拷贝优化）
- 支持 TLS 加密传输、Token 鉴权、心跳保活

## Tech Stack

- **Java**: JDK 8
- **Framework**: Spring Boot 2.7.x
- **Network**: Netty 4.1.x (NIO/Reactor 模式)
- **Database**: MyBatis-Plus + MySQL/H2
- **Cache**: Redis (可选)
- **Build**: Maven

## Build Commands

### Server (Java/Spring Boot)
```bash
# Set JAVA_HOME (required for compilation)
export JAVA_HOME="/c/Program Files/Java/jdk-1.8"

# Build JAR
cd D:/claudeCodeSpace/java/out-view
D:/java/maven/apache-maven-3.8.8/bin/mvn.cmd package -Dmaven.test.skip=true -s D:/java/maven/my-settings.xml

# Run server
java -jar target/outview-server.jar
```

### Client (Go)
```bash
cd D:/claudeCodeSpace/java/out-view/client

# Install dependencies
go mod tidy

# Build CLI version (no CGO required)
set CGO_ENABLED=0
go build -ldflags "-s -w" -o outview-client.exe ./cmd/outview-client

# Build GUI version (requires CGO and C compiler)
build-gui.bat
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    outView Server                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Spring Boot │  │   Netty     │  │   REST API          │  │
│  │ Container   │──│   Server    │──│   (Management)      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         │               │                    │               │
│         ▼               ▼                    ▼               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Auth/Config │  │ Protocol    │  │ Session Store       │  │
│  │ Services    │  │ Codec       │  │ (Redis/Memory)      │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Key Components

| Package | Description |
|---------|-------------|
| `com.outview.netty` | Netty 服务端、Handler 处理器 |
| `com.outview.protocol` | 自定义二进制协议、编解码器 |
| `com.outview.service` | 业务服务（鉴权、会话、端口映射） |
| `com.outview.controller` | REST API 控制器 |

### Protocol Specification

**消息头 (12 Bytes):**
| Field | Length | Description |
|-------|--------|-------------|
| Magic | 4B | 0x4F565753 ("OVWS") |
| Version | 1B | 协议版本 |
| Type | 1B | 1=注册, 2=心跳, 3=数据, 4=错误 |
| Length | 4B | Body 长度 |
| Reserved | 2B | 保留 |

### Port Configuration

- **8080**: HTTP REST API
- **7000**: Netty 控制端口 (客户端注册/心跳)
- **6000-6500**: 数据端口池 (RDP 转发)

## Development Notes

### Netty Handler Order
```
FrameDecoder → AuthHandler → HeartbeatHandler → ProxyHandler
```

### Memory Safety
- 所有 ByteBuf 使用后必须 `ReferenceCountUtil.release()`
- 使用 `PooledByteBufAllocator` 提升性能
- Handler 需标记 `@Sharable` 或确保线程安全

### Testing
- 单元测试使用 JUnit 5 + Mockito
- 集成测试需启动 Netty Server
- 压力测试模拟 1000+ 并发连接

## Client (Go)

Located in `client/` directory:
- `cmd/outview-client/` - CLI entry point
- `cmd/outview-gui/` - GUI entry point (fyne framework)
- `internal/protocol/` - Protocol implementation
- `internal/client/` - Core client logic

Go 1.21+ installer available at `client/go1.21.6.windows-amd64.msi`

## Circular Dependency Resolution

`RawDataHandler` uses `@Lazy` annotation on `DataPortService` dependency to break the cycle:
```
DataPortService -> DataChannelInitializer -> RawDataHandler -> DataPortService
```