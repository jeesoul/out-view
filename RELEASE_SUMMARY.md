# outView 1.2.1 发布验证说明

本文记录 `v1.2.1` 发布资产的验证范围。发布资产已上传到 GitHub Release；该验证不等同于公网、真实 RDP 或小时级稳定性保证。

## 已验证项目

| 检查项 | 结果 |
| --- | --- |
| Java 全量测试 | 141 项，失败 0，错误 0，跳过 3 |
| Go 客户端、协议、信令与 Sidecar 测试 | 通过 |
| Fyne GUI 软件驱动测试 | 通过 |
| Go 客户端、协议和 GUI race 检测 | 通过 |
| PowerShell / Bash 构建脚本契约 | 通过 |
| Windows JAR、CLI、GUI、Sidecar | 构建成功 |
| Linux x64/ARM64 CLI、Sidecar | 交叉构建成功 |
| macOS Intel/Apple Silicon CLI、Sidecar | 交叉构建成功 |
| 真实 JAR + CLI 回环 | 空闲 125 秒、四连接共 5 MiB 数据一致 |
| 后台固定端口流程 | 预设、修改、删除、冲突与离线保留已验证 |

## 发布前检查

- 后续版本发布时，在最终提交上创建正式版本 tag，并以 tag 重新构建全部产物。
- 生成发布包和 `.sha256` 校验文件，确认包内 README、CHANGELOG 与版本一致。
- 本次已将 `release/outview-1.2.1-platforms/` 中的 Windows、Linux、macOS 压缩包作为独立 Release assets 上传。
- 在 GitHub Release 中使用简短标题 `outView 1.2.1`，将变更说明放入正文，不把描述拼接到标题。
- 发布前补充真实 Windows RDP 和公网环境验证；当前回环结果不外推为公网、真实 RDP 或小时级稳定性保证。

## 当前边界

WebRTC 仍是实验模块。网络真实断开会结束原 TCP 流，连接恢复后需要重新建立远程桌面。GUI 原生编译成功不等于已完成真实桌面窗口、系统托盘和 RDP 登录验收。

