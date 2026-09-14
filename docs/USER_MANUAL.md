# 部署与使用

## 服务端

服务端需要 JDK 8。默认 HTTP 为 8080，控制端口为 7000，固定数据端口范围为 6000–6500。按实际需要开放端口。

从项目根目录构建并启动：

```powershell
$env:JAVA_HOME = 'C:\path\to\jdk8'
$env:OUTVIEW_DATA_DIR = 'D:\outview-data'
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/start-server.ps1
```

```bash
export JAVA_HOME=/path/to/jdk8
export OUTVIEW_DATA_DIR=/var/lib/outview
bash scripts/build.sh
bash scripts/start-server.sh
```

发布包根目录包含 `outview-server.jar` 时，同样从包内执行 `scripts/start-server.*`。配置示例是 `application.yml.example`，需要外部配置时复制为包根目录的 `application.yml` 并修改自己的账号和端口。构建不会覆盖现有输出目录或原部署配置。

服务端配置示例：

```yaml
outview:
  bind-address: 0.0.0.0
  control-port: 7000
  data-port-start: 6000
  data-port-end: 6500
  heartbeat-timeout: 90
outview-admin:
  users:
    - username: admin
      password: REPLACE_WITH_YOUR_PASSWORD
      role: ADMIN
```

当前后台使用配置账号与 Spring Security 登录。隧道 Token 校验体系和 Go TLS 配套仍未完成；请按自己的访问控制环境部署。

## 被控端 CLI

在可执行文件旁放置 `config.txt`：

```ini
host=YOUR_SERVER
port=7000
device-id=MY_DEVICE
token=YOUR_TOKEN
local-port=3389
heartbeat=30
auto-reconnect=true
max-retries=0
retry-delay=5
```

`device-id` 是固定映射的身份键，应长期保持不变。`max-retries=0` 表示无限重试；首次服务器不可达也会在后台继续重试。CLI 可使用 `-config 文件路径` 指定配置。

可选连接设置：

| 配置键 | 默认值 | 作用 |
| --- | --- | --- |
| dial-timeout | 10s | 控制连接拨号期限 |
| register-timeout | 15s | 注册应答期限 |
| read-timeout | 90s | 控制连接读空闲期限 |
| write-timeout | 10s | 单次控制写入期限 |
| heartbeat-timeout | 90s | 心跳应答期限 |
| local-dial-timeout | 5s | 本地服务拨号期限 |
| local-write-timeout | 10s | 本地写入期限 |
| local-queue-bytes | 1048576 | 单连接排队字节上限 |
| local-queue-size | 64 | 单连接排队消息上限 |
| max-local-connections | 128 | 同时转发连接上限 |

时间可写为 `500ms`、`10s`、`2m`，纯数字按秒处理。心跳发送间隔应小于服务端读空闲期限和客户端心跳应答期限。不要通过关闭故障检测来掩盖断线。

## Windows GUI

- “被控端”显示本机六位设备码，点击启动后等待注册成功。
- “控制端”输入对方设备码，查询成功后启动 `mstsc`；RDP 登录凭据由 Windows 处理。
- 可用 `-config config.txt`、`-host YOUR_SERVER`、`-port 7000` 配置自建服务器，两端使用同一服务器。
- 为兼容旧版本，未配置服务器时 GUI 保留历史内置地址。自建部署请明确填写 host 和 port。
- GUI 使用本机持久化设备码作为注册 ID；本地端口由 `local-port` 决定。
- `-auto-start` 会随 GUI 启动被控服务。安装器的自启动项发生在当前用户登录后，不是后台 Windows 服务。
- 关闭窗口收起到托盘；托盘“退出”会停止服务、关闭连接和定时任务。
- “心跳往返”来自真实应答测量，未采样时不显示估算值。

WebRTC 标签页是实验配置，新安装默认关闭；当前实际数据仍使用 TCP。

## 在后台固定端口

1. 登录 `http://YOUR_SERVER:8080`。
2. 在“固定端口管理”输入设备 ID。GUI 使用显示的六位设备码，CLI 使用配置的 `device-id`。
3. 输入页面显示的允许范围内的外网端口，点击“保存固定端口”。设备离线也可以提前预设。
4. 已有映射使用同一表单修改，也可以点击设备行的“固定端口”或映射行的“修改固定端口”预填。
5. 远程电脑连接 `YOUR_SERVER:固定端口`。

断开设备、网络中断和封禁都保留端口预留。只有主动删除预留才允许下次重新分配。离线预留不启动监听。

修改在线设备的外网端口会中断现有远程连接；之后连接新端口。服务端先确认新端口可监听，再更新持久化和会话；端口冲突时原映射和原监听保持可用。

目标端口以上线客户端的 `local-port` 为准；后台预设目标端口不会远程修改客户端配置。

## 升级时保留映射

- 停止旧服务后备份原 `data/` 和外部配置；数据库文件不要在两个不同部署中混用。
- 保留相同设备 ID / GUI 的用户配置目录 `outview/device.json`。
- 将 `OUTVIEW_DATA_DIR` 指向原数据目录的绝对路径。若旧服务通过另一个工作目录启动，先找出它实际使用的 `data/outview.mv.db`。
- 如果沿用旧 `application.yml`，检查其中的 `spring.datasource.url`，避免旧的字面相对路径覆盖新配置。H2 URL 应指向原目录，例如 `jdbc:h2:file:${OUTVIEW_DATA_DIR:./data}/outview;AUTO_SERVER=TRUE;WRITE_DELAY=0`。
- 新服务启动日志应出现已加载的固定端口预留数量，随后客户端上线复用原端口。

`WRITE_DELAY=0` 缩小提交后强制退出的写盘窗口，不能替代磁盘可靠性和备份。本次回环测试验证了修改端口后立即结束 JVM 再启动的恢复行为。

## 断开时的语义

网络中断期间无法保留原有 TCP 字节流。修复保证失效检测、清理和恢复后可重新连接，RDP 是否自动恢复会话还取决于系统远程桌面客户端及 Windows 策略。

定位步骤见 [排错指南](TROUBLESHOOTING.md)。
