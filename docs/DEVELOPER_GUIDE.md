# outView WebRTC 开发者指南

## 环境要求

### 服务端（Java）
- JDK 8+
- Maven 3.6+
- Spring Boot 2.7.x

### 客户端（Go）
- Go 1.24+
- GCC（Windows 需要 MinGW-w64）
- Git

### WebRTC Sidecar（Go）
- Go 1.24+
- pion/webrtc v4

## 项目结构

```
out-view/
├── src/main/java/com/outview/          # Java 服务端
│   ├── netty/                          # Netty 网络层
│   ├── protocol/                       # 协议定义
│   └── webrtc/                         # WebRTC 相关
│       ├── GrayReleaseManager.java     # 灰度发布管理
│       ├── SidecarManager.java         # Sidecar 进程管理
│       ├── WebRTCMetrics.java          # 监控指标
│       └── WebRTCProxyService.java     # WebRTC 代理服务
├── client/                             # Go 客户端
│   ├── cmd/
│   │   ├── outview-client/             # CLI 客户端
│   │   └── outview-gui/                # GUI 客户端（Fyne）
│   └── internal/
│       ├── client/                     # 客户端核心
│       ├── webrtc/                     # WebRTC 管理器
│       ├── gui/                        # GUI 组件
│       ├── logger/                     # 日志系统
│       └── protocol/                   # 协议实现
├── webrtc-sidecar/                     # WebRTC Sidecar
│   ├── cmd/sidecar/                    # Sidecar 主程序
│   └── internal/
│       ├── ipc/                        # IPC 通信（Unix Socket/Named Pipe）
│       └── webrtc/                     # WebRTC 管理和连接池
├── test/                               # 集成测试
│   ├── nat/                            # NAT 矩阵测试
│   ├── reconnect/                      # 重连场景测试
│   ├── turn/                           # TURN 故障测试
│   └── performance/                    # 性能测试
└── docs/                               # 文档
```

## 构建指南

### 构建 Java 服务端

```bash
cd out-view
mvn clean package -DskipTests

# 输出: target/outview-server-1.2.0.jar
```

### 构建 Go 客户端

```bash
cd client

# CLI 客户端
go build -o outview-client ./cmd/outview-client

# GUI 客户端（需要 OpenGL/X11）
go build -o outview-gui ./cmd/outview-gui

# Windows 下隐藏控制台窗口
go build -ldflags="-H windowsgui" -o outview-gui.exe ./cmd/outview-gui
```

### 构建 WebRTC Sidecar

```bash
cd webrtc-sidecar
go build -o sidecar ./cmd/sidecar

# Windows
go build -o sidecar.exe ./cmd/sidecar
```

### 交叉编译

```bash
# Linux → Windows
GOOS=windows GOARCH=amd64 go build -o outview-client.exe ./cmd/outview-client

# Windows → Linux
GOOS=linux GOARCH=amd64 go build -o outview-client ./cmd/outview-client

# macOS
GOOS=darwin GOARCH=amd64 go build -o outview-client ./cmd/outview-client
```

## 测试指南

### Java 测试

```bash
# 全量测试
mvn test

# 指定测试类
mvn test -Dtest=GrayReleaseManagerTest,WebRTCMetricsTest

# 跳过测试
mvn package -DskipTests
```

**测试结果**: 117 个测试，3 个 skip（网络相关）

### Go 测试

**重要**: Windows 下需要设置 `GOTMPDIR` 避免杀毒软件拦截测试二进制。

```bash
# 创建测试临时目录
mkdir -p _testbin

# 客户端测试
cd client
GOTMPDIR="$(pwd)/../_testbin" go test ./internal/... -count=1 -v

# Sidecar 测试
cd webrtc-sidecar
GOTMPDIR="$(pwd)/../_testbin" go test ./internal/... -count=1 -v

# 重连测试
cd test/reconnect
GOTMPDIR="$(pwd)/../../_testbin" go test ./... -count=1 -v

# TURN 故障测试
cd test/turn
GOTMPDIR="$(pwd)/../../_testbin" go test ./... -count=1 -v

# 清理
rm -rf _testbin
```

**测试覆盖率**:
- client/internal/webrtc: 61 个测试
- client/internal/logger: 22 个测试
- client/internal/guitest: 6 个测试
- webrtc-sidecar/internal: 34+ 个测试

### 性能基准测试

```bash
cd client
go test ./internal/webrtc/... -bench=. -benchmem -count=3

# 预期结果:
# BenchmarkSendData          17.40 ns/op   14715 MB/s   0 B/op   0 allocs/op
# BenchmarkBufferPool        18.54 ns/op                24 B/op  1 allocs/op
# BenchmarkStateTransition   206.8 ns/op                16 B/op  1 allocs/op
# BenchmarkMetricsRecord     22.44 ns/op                 0 B/op  0 allocs/op
# BenchmarkMetricsSnapshot   354.0 ns/op               952 B/op  3 allocs/op
```

### GUI 测试（无头模式）

```bash
cd client
go test -tags=headless_test ./cmd/outview-gui/... -v
```

### 弱网性能测试

```bash
cd test/performance
bash weak_network_test.sh

# 需要 tc (traffic control) 工具
# 如果没有 tc，脚本会运行模拟模式
```

### NAT 矩阵测试

```bash
cd test/nat
bash run_matrix_test.sh

# 需要 Docker 和 docker-compose
```

## 代码规范

### Go 代码规范

```bash
# 格式化
go fmt ./...

# 静态检查
go vet ./...

# 导入排序
goimports -w .
```

### Java 代码规范

遵循 Spring Boot 标准规范：
- 使用 4 空格缩进
- 类名使用 PascalCase
- 方法名使用 camelCase
- 常量使用 UPPER_SNAKE_CASE

## 调试指南

### 启用详细日志

**Go 客户端**:
```bash
OUTVIEW_LOG_LEVEL=debug ./outview-client
```

**Java 服务端** (`application.yml`):
```yaml
logging:
  level:
    com.outview: DEBUG
    com.outview.webrtc: TRACE
```

### 使用调试工具

```bash
# 检查 STUN 可达性和 NAT 类型
./tools/webrtc-debug.sh

# 实时查看 WebRTC 统计
./tools/webrtc-stats -watch

# JSON 格式输出（用于监控集成）
./tools/webrtc-stats -json
```

### 查看 Sidecar 日志

**Linux/macOS**:
```bash
tail -f /tmp/outview-sidecar.log
```

**Windows**:
```powershell
Get-Content -Path "C:\Temp\outview-sidecar.log" -Wait
```

### 抓包分析

```bash
# 抓取 STUN/TURN 流量（UDP 3478）
tcpdump -i any -n udp port 3478 -w webrtc.pcap

# 抓取 WebRTC 媒体流量（UDP 49152-65535）
tcpdump -i any -n 'udp portrange 49152-65535' -w webrtc-media.pcap

# 使用 Wireshark 分析
wireshark webrtc.pcap
```

## 常见开发问题

### 1. Go 测试被杀毒软件拦截

**症状**: `fork/exec ... Access is denied`

**解决**:
```bash
# 方案 1: 设置 GOTMPDIR
mkdir -p _testbin
GOTMPDIR="$(pwd)/_testbin" go test ./...

# 方案 2: 添加 Windows Defender 白名单
# 设置 → 更新和安全 → Windows 安全中心 → 病毒和威胁防护 → 排除项
# 添加项目目录
```

### 2. Fyne GUI 编译失败

**症状**: `build constraints exclude all Go files`

**解决**:
```bash
# Windows: 安装 MinGW-w64
# https://www.mingw-w64.org/

# Linux: 安装开发库
sudo apt-get install libgl1-mesa-dev xorg-dev

# macOS: 无需额外依赖
```

### 3. pion/webrtc 编译慢

**原因**: pion/webrtc 依赖较多，首次编译需要下载大量依赖。

**解决**:
```bash
# 使用 Go module proxy
export GOPROXY=https://goproxy.cn,direct

# 或使用 Athens
export GOPROXY=https://athens.azurefd.net
```

### 4. Java 测试编译版本不匹配

**症状**: `class file version 61.0 ... only recognizes ... 52.0`

**解决**:
```bash
# 确保使用 JDK 8 编译
export JAVA_HOME="/path/to/jdk-1.8"
mvn clean test-compile
```

## 贡献指南

### 提交代码前检查清单

- [ ] 代码通过 `go fmt` / `goimports` 格式化
- [ ] 通过 `go vet` 静态检查
- [ ] 所有单元测试通过
- [ ] 添加必要的测试覆盖新功能
- [ ] 更新相关文档
- [ ] Commit message 遵循规范（见下）

### Commit Message 规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Type**:
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `test`: 测试相关
- `refactor`: 重构
- `perf`: 性能优化
- `chore`: 构建/工具链相关

**示例**:
```
feat(webrtc): add Trickle ICE support

Implement OnICECandidate callback and candidate buffering queue.
Candidates arriving before SetRemoteDescription are cached and
flushed after remote description is set.

Closes #123
```

### 分支策略

- `main`: 生产分支，只接受 PR
- `feature/*`: 功能分支
- `fix/*`: Bug 修复分支
- `release/*`: 发布分支

### Pull Request 流程

1. Fork 项目
2. 创建功能分支: `git checkout -b feature/my-feature`
3. 提交代码: `git commit -am 'feat: add my feature'`
4. 推送分支: `git push origin feature/my-feature`
5. 创建 Pull Request
6. 等待 Code Review
7. 合并后删除分支

## 发布流程

### 版本号规范

遵循 [Semantic Versioning 2.0.0](https://semver.org/):
- `MAJOR.MINOR.PATCH`
- 例如: `1.2.0`

### 发布步骤

1. 更新版本号
2. 更新 CHANGELOG.md
3. 创建 Git tag: `git tag -a v1.2.0 -m "Release v1.2.0"`
4. 推送 tag: `git push origin v1.2.0`
5. 构建发布包
6. 创建 GitHub Release
7. 按灰度发布方案推广

详见 [灰度发布方案](gray-release-plan.md)。

## 性能优化建议

### Go 客户端

1. **使用 sync.Pool 减少内存分配**
   - 已实现: `client/internal/webrtc/pool.go`
   - 目标: SendData 热路径 0 allocs/op ✅

2. **批量发送减少系统调用**
   - 已实现: `BatchSender` 32KB/32msg/5ms 阈值
   - 适用场景: 高频小包发送

3. **减少锁竞争**
   - 使用 `atomic` 替代 `sync.Mutex` 用于计数器
   - 缩小临界区范围

### Java 服务端

1. **使用 LongAdder 替代 AtomicLong**
   - 已实现: `WebRTCMetrics.java`
   - 高并发场景性能提升 4-8x

2. **连接池复用**
   - Sidecar IPC 连接池
   - WebRTC PeerConnection 池

3. **异步处理**
   - 使用 CompletableFuture 处理 IPC 响应
   - 避免阻塞 Netty EventLoop

## 安全注意事项

1. **Token 管理**
   - 不要在日志中输出完整 Token
   - 使用环境变量或配置文件存储

2. **IPC 权限**
   - Unix Socket: 设置 0600 权限
   - Named Pipe: 使用 ACL 限制访问

3. **TURN 凭证**
   - 使用短期凭证（TTL < 24h）
   - 定期轮换 TURN 密码

4. **输入验证**
   - 验证所有外部输入（SDP、ICE candidate）
   - 防止注入攻击

## 参考资源

- [pion/webrtc 文档](https://github.com/pion/webrtc)
- [WebRTC 规范](https://www.w3.org/TR/webrtc/)
- [ICE RFC 8445](https://datatracker.ietf.org/doc/html/rfc8445)
- [STUN RFC 5389](https://datatracker.ietf.org/doc/html/rfc5389)
- [TURN RFC 5766](https://datatracker.ietf.org/doc/html/rfc5766)
