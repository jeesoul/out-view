# 开发、构建与验收

## 工具链

- JDK 8，Maven；设置 `JAVA_HOME`，Maven 可从 PATH、MAVEN_HOME/M2_HOME 或标准 wrapper 缓存寻找。
- Go 1.24+，模块指定工具链 go1.24.3。
- 原生 Windows GUI 使用 Fyne 2.4.3，需要 MinGW GCC 和 `CGO_ENABLED=1`。
- Python 3 用于独立 JAR/CLI 持续连接验收；浏览器页面不需要 npm 构建。

## 构建入口

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1
# 本机 GUI 与 CLI，明确要求可用的 C 编译器：
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -ClientMode Both -OutputDirectory artifacts/desktop-build
# 发布矩阵（CLI / Sidecar 包含多个平台，GUI为Windows AMD64）：
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Release -ClientMode Both
```

```bash
bash scripts/build.sh
bash scripts/build.sh --release --client-mode cli --output release/custom-build
```

版本从 `pom.xml` 读取并注入客户端。产物使用 `outview-client-cli-平台-架构`、`outview-client-gui-windows-amd64.exe` 明确区分；GUI 构建失败不会伪装为 CLI 成功。已有输出目录拒绝覆盖，临时 staging 完成后才发布。

根目录 `build-release.sh` 和 client 下旧构建脚本是兼容包装，具体参数以 `scripts/` 为准。Windows 安装器通过 `build-installer.bat` 调用 Inno Setup；缺少编译器或 GUI 产物会明确失败。

## 测试

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1
```

```bash
bash scripts/test.sh
```

单独运行：

```bash
mvn test
cd client
go test -count=1 -timeout=120s -tags=ci ./...
go test -count=1 -timeout=120s -tags=integration ./test/integration
cd ../webrtc-sidecar
go test -count=1 -timeout=120s ./...
```

`ci` 使用 Fyne 内置的软件驱动，编译和测试实际 GUI 代码；`headless_test` 是旧镜像测试路径，不能替代真实 GUI 代码验证。原生窗口显示、字体和 mstsc 交互需要 Windows 实机验收。

具备 C 工具链时执行并发检测：

```bash
(cd client && CGO_ENABLED=1 go test -race -count=1 -timeout=120s -tags=ci ./internal/client ./internal/protocol ./cmd/outview-gui)
```

Java `ConnectionLifecycleTest` 使用真实 H2 和业务组件；`RealLifecycleIntegrationTest` 使用生产处理器和回环 Netty。覆盖固定预留、绑定失败、旧会话清理、端口变更、心跳、关闭传播和背压。历史部分集成测试使用专用 TestServerHandler，不能据此判断生产链路完整。

## 真实进程持续连接测试

```powershell
python test/soak/tunnel_soak.py --java "$env:JAVA_HOME/bin/java.exe" --client artifacts/outview-1.2.1/client/windows/outview-client-cli-windows-amd64.exe --idle-seconds 125
```

需要更长空闲观测时把 `--idle-seconds` 改为 `3600`。该脚本启动真实 JAR 和 CLI、独立文件数据库及回环 echo 服务，验收同一连接空闲、四连接并发、端口冲突、在线修改、服务器重启和封禁，并在 finally 中结束自己启动的进程。结果及日志保存在系统临时目录，路径由脚本输出。

这是回环 TCP 测试，不能替代公网 NAT、防火墙、睡眠唤醒或真实 RDP 验收；当前已执行的结果见 [发布总结](../RELEASE_SUMMARY.md)。

## 维护约定

- 保持 OVWS 协议兼容，协议修改需要同时覆盖 Java/Go。
- 改生命周期时先给出可失败的行为测试，避免复制实现或只检查字段存在。
- 固定映射删除仅限显式管理操作；离线、监听失败和重试不应删除预留。
- 客户端回调在启动前设置并及时返回；需要从回调停止时，异步调用 Stop，避免任务等待自己退出。
- 默认 INFO 日志；客户端 `OUTVIEW_LOG_LEVEL=DEBUG` 可开启诊断，服务端可用 `--logging.level.com.outview=DEBUG`。持续传输默认不逐包输出。
- 运行数据、构建产物、本地工具和 agent 工作记录不进入源码发布包。
