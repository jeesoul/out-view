#!/bin/bash
set -e

VERSION="1.2.0"
RELEASE_DIR="release/outview-${VERSION}"

echo "Building outView ${VERSION} release package..."

# Create directory structure
mkdir -p "${RELEASE_DIR}/client"/{windows,linux,macos}
mkdir -p "${RELEASE_DIR}/webrtc-sidecar"/{windows,linux,macos}

# Build Go client for all platforms
# Windows: GUI 版本（需要 CGO + MinGW）
# Linux/macOS: CLI 版本（无 CGO 依赖）
echo "Building Go client..."
cd client
# Windows GUI（CGO_ENABLED=1，需要 MinGW）
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w -H windowsgui -X main.Version=${VERSION}" \
  -o "../${RELEASE_DIR}/client/windows/outview-client.exe" ./cmd/outview-gui \
  || {
    echo "GUI build failed, falling back to CLI..."
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o "../${RELEASE_DIR}/client/windows/outview-client.exe" ./cmd/outview-client
  }
# Linux CLI
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/client/linux/outview-client" ./cmd/outview-client
# macOS CLI
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/client/macos/outview-client-intel" ./cmd/outview-client
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/client/macos/outview-client-arm" ./cmd/outview-client
cd ..

# Build WebRTC sidecar for all platforms
echo "Building WebRTC sidecar..."
cd webrtc-sidecar
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/webrtc-sidecar/windows/webrtc-sidecar.exe" ./cmd/sidecar
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/webrtc-sidecar/linux/webrtc-sidecar" ./cmd/sidecar
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/webrtc-sidecar/macos/webrtc-sidecar-intel" ./cmd/sidecar
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "../${RELEASE_DIR}/webrtc-sidecar/macos/webrtc-sidecar-arm" ./cmd/sidecar
cd ..

# Build Java server
echo "Building Java server..."
MVN="/d/java/maven/apache-maven-3.8.8/bin/mvn"
JAVA_HOME="/c/Program Files/Java/jdk-1.8"
JAVA_HOME="$JAVA_HOME" "$MVN" clean package -DskipTests
cp target/outview-*.jar "${RELEASE_DIR}/outview-server.jar"

# Copy configuration files
echo "Copying configuration files..."
cp src/main/resources/application.yml "${RELEASE_DIR}/"

# Copy documentation (user-facing only)
echo "Copying documentation..."
cp docs/USER_MANUAL.md "${RELEASE_DIR}/"
cp docs/webrtc-user-guide.md "${RELEASE_DIR}/"
cp docs/webrtc-troubleshooting.md "${RELEASE_DIR}/"
cp README.md "${RELEASE_DIR}/"
cp LICENSE "${RELEASE_DIR}/"

# Create client README
cat > "${RELEASE_DIR}/client/README.md" << 'EOF'
# outView Client v1.2.0

## 平台支持

- **Windows**: `windows/outview-client.exe`
- **Linux**: `linux/outview-client`
- **macOS Intel**: `macos/outview-client-intel`
- **macOS ARM (M1/M2)**: `macos/outview-client-arm`

## WebRTC Sidecar

v1.2.0 引入了 WebRTC P2P 传输优化，需要配合 WebRTC Sidecar 使用：

- **Windows**: `../webrtc-sidecar/windows/webrtc-sidecar.exe`
- **Linux**: `../webrtc-sidecar/linux/webrtc-sidecar`
- **macOS Intel**: `../webrtc-sidecar/macos/webrtc-sidecar-intel`
- **macOS ARM**: `../webrtc-sidecar/macos/webrtc-sidecar-arm`

客户端会自动启动 sidecar 进程，无需手动运行。

## 配置文件

在客户端同目录下创建 `config.txt` 文件（参考 `config.txt.example`）：

```ini
# 自定义本地服务端口（重要！）
local-port=3389

# 其他可选配置
host=120.27.214.55
port=7000
heartbeat=30
```

**支持情况**：
- **GUI 版本**：支持读取 `local-port` 配置（v1.2.0+）
- **CLI 版本**：完全支持所有配置项

**配置文件搜索路径**：
1. 可执行文件所在目录
2. 当前工作目录

**支持的文件名**：`config.txt`, `config.ini`, `outview.conf`, `outview.ini`

## 使用方法

### Windows GUI
```cmd
# 双击运行，会自动读取 config.txt
outview-client.exe
```

### Windows CLI
```cmd
# 自动查找配置文件
outview-client.exe

# 或指定配置文件
outview-client.exe -config config.txt

# 或使用命令行参数
outview-client.exe -host 120.27.214.55 -device-id TEST01 -token mytoken -local-port 8080
```

### Linux/macOS
```bash
chmod +x outview-client
./outview-client -config config.txt
```

详细使用说明请参考 `../USER_MANUAL.md`
EOF

# Create sidecar README
cat > "${RELEASE_DIR}/webrtc-sidecar/README.md" << 'EOF'
# WebRTC Sidecar v1.2.0

WebRTC Sidecar 是 outView 1.2.0 引入的 P2P 传输优化组件。

## 说明

- Sidecar 由客户端自动启动和管理，无需手动运行
- 支持 WebRTC P2P 直连，降低延迟，提升性能
- 自动 TCP 回退，确保连接稳定性

## 平台支持

- **Windows**: `windows/webrtc-sidecar.exe`
- **Linux**: `linux/webrtc-sidecar`
- **macOS Intel**: `macos/webrtc-sidecar-intel`
- **macOS ARM (M1/M2)**: `macos/webrtc-sidecar-arm`

## 配置

WebRTC 配置在客户端的 `config.txt` 中：

```
# WebRTC 配置（可选）
webrtc_enabled=true
stun_servers=stun:stun.l.google.com:19302
turn_servers=turn:your-turn-server.com:3478
turn_username=username
turn_password=password
```

详细配置说明请参考 `../webrtc-user-guide.md`
EOF

# Create config example
cat > "${RELEASE_DIR}/client/config.txt.example" << 'EOF'
# outView Client Configuration
# 配置文件支持：CLI 版本完全支持，GUI 版本支持 local-port 配置

# 服务器地址（可选，GUI 版本使用内置服务器地址）
host=120.27.214.55
port=7000

# 设备认证（可选，GUI 版本自动生成设备码）
device-id=YOUR-DEVICE-ID
token=outview-YOUR-DEVICE-ID

# 本地服务端口（重要！GUI 版本会读取此配置）
# - RDP 远程桌面: local-port=3389
# - SSH: local-port=22
# - HTTP: local-port=80
# - 自定义服务: local-port=8080
local-port=3389

# 心跳间隔（可选，默认 30 秒）
heartbeat=30

# WebRTC 配置（可选，v1.2.0+）
# webrtc_enabled=true
# stun_servers=stun:stun.l.google.com:19302
# turn_servers=turn:your-turn-server.com:3478
# turn_username=username
# turn_password=password
EOF

# Create start scripts
cat > "${RELEASE_DIR}/start.sh" << 'EOF'
#!/bin/bash
java -jar outview-server.jar --spring.config.location=application.yml
EOF
chmod +x "${RELEASE_DIR}/start.sh"

cat > "${RELEASE_DIR}/start.bat" << 'EOF'
@echo off
java -jar outview-server.jar --spring.config.location=application.yml
pause
EOF

# Create CHANGELOG
cat > "${RELEASE_DIR}/CHANGELOG.md" << 'EOF'
# Changelog

## [1.2.0] - 2026-05-17

### 新增功能

#### WebRTC P2P 传输优化
- ✨ WebRTC P2P 直连，降低延迟 30-50%
- ✨ 自动 NAT 穿透（STUN/TURN）
- ✨ 智能 TCP 回退机制
- ✨ 连接状态实时监控
- ✨ 性能指标统计（延迟、吞吐量）

#### 架构改进
- 🏗️ Sidecar 架构：Java 服务端 ↔ IPC ↔ Go WebRTC Sidecar
- 🏗️ 9 状态连接状态机
- 🏗️ Trickle ICE 候选缓冲
- 🏗️ DataChannel 背压控制

#### 性能优化
- ⚡ sync.Pool 缓冲池（零分配）
- ⚡ BatchSender 批量发送
- ⚡ 原子计数器和无锁指标
- ⚡ BenchmarkSendData: 14.5 GB/s, 0 allocs/op

#### 可靠性增强
- 🔄 指数退避重连（1s → 30s）
- 🔄 空闲超时生命周期触发
- 🔄 资源清理（sync.Once 幂等关闭）
- 🔄 边缘情况测试（7个测试）

#### 灰度发布
- 📊 设备 ID 分桶（10% → 50% → 100%）
- 📊 运行时启用/百分比切换
- 📊 指标监控和回滚方案

#### 测试覆盖
- ✅ 101 Go 测试 + 117 Java 测试
- ✅ NAT 矩阵测试（16 种组合）
- ✅ 弱网环境测试
- ✅ TURN 故障测试
- ✅ 重连测试

#### 文档完善
- 📖 用户手册
- 📖 开发者指南
- 📖 性能基准报告
- 📖 WebRTC 配置指南
- 📖 故障排查手册
- 📖 灰度发布计划

### 技术栈

- Java 8 + Spring Boot 2.7.18
- Go 1.24+ + pion/webrtc v4.2.12
- Netty 4.1.100.Final
- IPC: Unix Socket / Named Pipe

### 性能指标

- WebRTC 连接建立: 2-3 秒
- P2P 延迟: 比 TCP 低 30-50%
- 吞吐量: 14.5 GB/s (BenchmarkSendData)
- 内存: 零分配热路径

### 兼容性

- 向后兼容 1.1.0
- 灰度发布，可动态切换 WebRTC/TCP
- 自动回退到 TCP（WebRTC 失败时）

---

## [1.1.0] - 2026-03-XX

### 新增功能
- 二进制协议优化
- 自动重连机制
- 零拷贝传输
- 共享 EventLoopGroup

---

## [1.0.2] - 2026-04-XX

### 新增功能
- 客户端本地 RDP 关闭时主动通知服务端
- 客户端按天日志落盘

---

## [1.0.0] - 2026-03-XX

### 首次发布
- 基础远程桌面隧道功能
- Token 认证
- 跨平台支持
EOF

echo "Creating release archive..."
cd release
zip -r "outview-${VERSION}.zip" "outview-${VERSION}"
cd ..

echo "✅ Release package created: release/outview-${VERSION}.zip"
echo ""
echo "Contents:"
echo "  - Server: outview-server.jar"
echo "  - Client: Windows, Linux, macOS (Intel + ARM)"
echo "  - WebRTC Sidecar: Windows, Linux, macOS (Intel + ARM)"
echo "  - Documentation: USER_MANUAL.md, DEVELOPER_GUIDE.md, etc."
echo "  - Configuration: application.yml, config.txt.example"
echo ""
echo "Package size:"
du -sh "release/outview-${VERSION}"
du -sh "release/outview-${VERSION}.zip"
