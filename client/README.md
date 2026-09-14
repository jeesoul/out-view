# outView 客户端

CLI 入口为 `cmd/outview-client`，Windows GUI 为 `cmd/outview-gui`，两者共用 `internal/client`。

## 平台支持

| 平台 | CLI | GUI |
| --- | --- | --- |
| Windows x64 | 支持 | 支持 |
| Linux x64 / ARM64 | 支持 | 暂无 |
| macOS Intel / Apple Silicon | 支持 | 暂无 |

Linux 和 macOS CLI 无需 CGO。仓库根目录的 `scripts/build.ps1 -Release` / `scripts/build.sh --release` 会生成上述五种 CLI；普通构建只生成当前主机平台，不能把普通构建目录当成完整发布包。

```bash
go build -o outview-client-cli ./cmd/outview-client
go test -count=1 -timeout=120s -tags=ci ./...
```

发布构建请从仓库根目录调用 `scripts/build.ps1` / `scripts/build.sh`，由脚本从 POM 注入版本。原生GUI需要C编译器；CLI无需CGO。

配置、固定端口、超时和 GUI 使用方法统一见 [用户手册](../docs/USER_MANUAL.md)。默认无限自动重连，同一设备ID复用固定端口；控制连接失效后会回收旧本地连接。

客户端默认INFO日志，`OUTVIEW_LOG_LEVEL=DEBUG` 用于诊断。WebRTC保留为实验信令，不能据此认为数据已走P2P。
