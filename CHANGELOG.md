# Changelog

## [1.2.0] - 2026-07-09（迭代：固定端口 + 断网重连稳定性）

### 新增功能
- 🔒 固定外网映射端口：端口映射持久化到数据库（`PortMapping` JPA 实体），客户端重连/服务端重启端口不变
- 🛠️ 管理后台预设固定端口（设备未上线也可预设，上线时自动复用）
- 🗑️ 管理后台删除固定端口映射

### 修复
- 🐛 断网快速重连竞态：旧连接心跳超时误清新连接会话与端口
  - `SessionStore.getSessionByChannel` 严格匹配 channel
  - `SessionStore.register` 覆盖前清理旧 channel 映射
  - `AuthHandler.channelInactive` 严格匹配避免误杀新连接
- 🐛 客户端重连后外网端口变化（端口映射持久化后复用固定端口）

### 改进
- ♻️ 被控端默认无限重连（`MaxRetries=0`），断网恢复自动重连，保证常驻在线
- ♻️ CLI 新增 `-max-retries` flag；config 文件支持 `max-retries` / `auto-reconnect`
- 🎨 管理后台端口映射表显示在线/离线状态、最后上线时间

### 数据库
- 新增 `port_mapping` 表（H2 自动建表，`ddl-auto=update`）

---

## [1.2.0] - 2026-05-17（WebRTC P2P 初版）

### 新增功能
- ✨ WebRTC P2P 直连，延迟降低 30-50%
- ✨ 自动 NAT 穿透（STUN/TURN）+ 智能 TCP 回退
- 🔐 Spring Security + HTTP Basic Auth 多账号
- 🚫 设备封禁管理（JPA + H2/MySQL，注册时校验）
- 🔧 端口映射在线修改
- 🏗️ Sidecar 架构（Java ↔ IPC ↔ Go WebRTC）
- 📊 灰度发布（设备 ID 分桶，运行时切换）

### 技术栈
- Java 8 + Spring Boot 2.7.18 + Netty 4.1.100
- Go 1.24+ + pion/webrtc v4

---

## [1.1.0] - 2026-03
- 二进制协议优化、自动重连、零拷贝传输、共享 EventLoopGroup

---

## [1.0.2] - 2026-04
- 客户端本地 RDP 关闭通知、按天日志落盘

---

## [1.0.0] - 2026-03
- 基础远程桌面隧道、Token 认证、跨平台支持
