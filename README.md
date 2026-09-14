# outView

outView 为内网 TCP 服务提供公网端口映射，主要用于 Windows 远程桌面（RDP）。桌面显示和远程登录由系统 RDP 完成，outView 负责设备发现、隧道转发、固定端口与连接恢复。

当前开发版本：**1.2.1**。许可证：MIT。

## 已实现能力

- Java / Netty 双向 TCP 隧道，按连接 ID 区分多个远程连接。
- 后台为在线或离线设备设置固定外网端口，断线、重连和服务重启保留预留。
- 修改在线设备端口时同步监听、数据库、会话和设备码查询；端口冲突时保留原映射。
- Go CLI 和 Windows Fyne GUI，支持配置自建服务器、自动重连和真实心跳 RTT。
- 读写、注册及心跳应答期限；单连接有界队列；双向关闭通知与资源回收。
- 后台登录、设备断开、封禁与解封，H2 默认持久化，可配置 MySQL。

WebRTC 代码保留为实验模块，当前发布主链路仍为 TCP，GUI 新安装默认关闭 WebRTC。Token 签发校验体系及 Go TLS 隧道尚未完成；不要把生成 Token 或存在服务端 TLS 开关理解为完整的隧道身份认证和加密实现。管理后台账号与隧道注册是不同机制。

## 快速开始

需要 **JDK 8、Maven、Go 1.24+**；构建原生 Windows GUI 还需要可用的 MinGW GCC / CGO 环境。

Windows PowerShell：

```powershell
# 先设置 JAVA_HOME 为本机 JDK 8 目录，Maven 和 Go 放入 PATH。
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/start-server.ps1
```

Linux / macOS：

```bash
bash scripts/build.sh
bash scripts/start-server.sh
```

默认构建产物位于 `artifacts/outview-1.2.1/`，包含服务端、CLI 与实验 Sidecar。脚本拒绝覆盖已存在的输出目录，重复构建时使用新的 `-OutputDirectory` / `--output`。

被控端 CLI 示例：

```text
outview-client-cli-windows-amd64.exe -host YOUR_SERVER -port 7000 -device-id MY_DEVICE -token YOUR_TOKEN -local-port 3389
```

访问 `http://YOUR_SERVER:8080` 登录后台，进入“固定端口管理”，输入相同设备 ID 和允许范围内的外网端口。远程电脑通过 `mstsc /v:YOUR_SERVER:固定端口` 连接。

默认后台账号来自 `application.yml`；部署时请配置自己的账号。被控电脑需要已有可用的 RDP 服务，端口映射不会替系统开启 RDP。

## 端口与数据

| 默认端口 | 用途 |
| --- | --- |
| 8080 | 管理后台与 HTTP API |
| 7000 | 客户端注册、心跳、查询和 TCP 隧道 |
| 6000–6500 | 固定外网端口池，可通过服务端配置调整 |

固定映射保存在 H2 `data/outview.mv.db` 中。启动脚本固定工作目录，支持 `OUTVIEW_DATA_DIR` 指定持久数据目录。**升级或更换启动目录时继续使用原数据目录和原设备 ID**，详见用户手册。

## 项目骨架

```text
src/main/java/com/outview/   Java 服务端、协议和生命周期管理
src/main/resources/static/  后台 HTML 与 assets 下的 CSS/JavaScript
src/test/                   Java 单元及真实 H2/Netty 集成测试
client/cmd/                 CLI / GUI 入口
client/internal/client/     控制连接、转发、信令及配置
webrtc-sidecar/             实验 WebRTC / IPC 组件
scripts/                    构建、测试、启动入口
test/soak/                  真实 JAR + CLI 的回环持续连接验收
installer/windows/          Windows 安装器
docs/                       用户、开发、架构和排错文档
artifacts/、release/         本地构建产物，不提交 Git
```

## 文档与验证

- [用户手册：部署、固定端口、升级](docs/USER_MANUAL.md)
- [开发与测试](docs/DEVELOPER_GUIDE.md)
- [实际架构](docs/ARCHITECTURE.md)
- [长连接与端口排错](docs/TROUBLESHOOTING.md)
- [本次验证结果及边界](RELEASE_SUMMARY.md)
- [变更记录](CHANGELOG.md)

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1
```

```bash
bash scripts/test.sh
```

已有回环验收覆盖 125 秒空闲、四连接共 5 MiB 数据往返、端口冲突、在线改端口、服务重启后的固定端口恢复。它不是公网、真实 RDP 或小时级连接稳定性的保证。持续运行更久可使用 `test/soak/tunnel_soak.py`。

欢迎提交带复现步骤、版本、配置和已脱敏日志的 Issue 或 Pull Request。
