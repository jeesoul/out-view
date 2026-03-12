# CHANGELOG

All notable changes to the outView project will be documented in this file.

## [SNAPSHOT-1.0.2] - 2026-03-12

### Bug Fixes

- **修复并发写入导致连接断开的问题**
  - 问题：多个goroutine同时写入同一个bufio.Writer，导致数据交错错乱
  - 现象：远程连接一段时间后自动断开，服务端报 `Invalid magic number` 错误
  - 原因：心跳goroutine和数据转发goroutine并发写入冲突
  - 修复：添加专用 `writeMu` 互斥锁，确保所有写操作原子性
  - 影响文件：`client/internal/client/client.go`

- **改进服务端解码器容错性**
  - 问题：收到错乱数据后直接关闭连接
  - 修复：尝试重新同步数据流，累计错误超过阈值才关闭连接
  - 影响文件：`src/main/java/com/outview/protocol/codec/MessageDecoder.java`

### New Features

*开发中...*

---

## [1.0.0] - 2026-03-12

### Features

- 服务端核心功能（Spring Boot 2.7 + Netty 4.1）
- Go客户端CLI版本
- 自定义二进制协议（Magic: 0x4F565753）
- Token认证机制
- 心跳保活（30秒间隔/90秒超时）
- 动态端口分配（6000-6500）
- 配置文件支持（config.txt）
- 管理后台Web UI
- SSL/TLS加密支持

---

## 版本说明

- **SNAPSHOT-x.x.x**: 开发快照版本，包含最新的bug修复和功能开发
- **x.x.x**: 稳定发布版本

## 版本规划

### SNAPSHOT-1.0.2 (当前开发中)

- [x] 修复并发写入导致连接断开
- [ ] 待定新功能...

### 1.0.1 (计划中)

- 整合SNAPSHOT-1.0.2的bug修复
- 发布稳定版本

---

*最后更新: 2026-03-12*