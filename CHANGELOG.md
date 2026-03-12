# CHANGELOG

All notable changes to the outView project will be documented in this file.

## [SNAPSHOT-1.0.2] - 2026-03-12

### New Features

- **预设配置管理功能**
  - 支持预先设置固定的 ClientId、Token 和端口号
  - 内置 H2 数据库存储预设配置，支持切换到 MySQL
  - 管理后台新增预设配置管理页面，支持增删改查
  - Token 验证逻辑：预设 Token 优先，随机 Token 兼容
  - 端口分配逻辑：预设固定端口优先，随机分配兜底
  - 新增 API 接口：`/api/presets` (GET/POST/PUT/DELETE)
  - 新增凭证生成功能：一键生成 ClientId 和 Token

- **管理后台登录认证**
  - 添加 Spring Security 安全框架
  - 管理后台需要登录才能访问
  - 默认管理员账号：`admin`，初始密码：`admin123`
  - 密码使用 BCrypt 加密存储
  - 支持登出功能

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

### Technical Details

- 新增 `PresetConfig` 实体类
- 新增 `PresetConfigRepository` 数据访问层
- 新增 `PresetConfigService` 业务服务层
- 新增 `PresetConfigController` REST API 控制器
- 修改 `PortMappingService` 支持固定端口分配
- 修改 `AuthHandler` 实现 Token 验证逻辑
- 更新管理后台 UI，新增预设配置管理卡片
- 配置文件支持 H2/MySQL 数据源切换
- 新增 `SysUser` 用户实体类
- 新增 `SysUserRepository` 用户数据访问层
- 新增 `SysUserService` 用户服务
- 新增 `SecurityConfig` Spring Security 配置
- 新增 `CustomUserDetailsService` 用户认证服务
- 新增 `AppInitializer` 应用启动初始化
- 新增登录页面 `login.html`

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
- [x] 预设配置管理功能
- [x] H2/MySQL 数据库支持
- [x] Token 验证逻辑
- [x] 固定端口分配
- [x] 管理后台登录认证

### 1.0.1 (计划中)

- 整合SNAPSHOT-1.0.2的bug修复和功能
- 发布稳定版本

---

*最后更新: 2026-03-12*