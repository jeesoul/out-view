# outView 文档目录

## 用户文档

- **[USER_MANUAL.md](USER_MANUAL.md)** - 完整的用户使用手册
  - 快速开始
  - 服务端部署
  - 客户端安装（Windows/Linux/macOS）
  - WebRTC 配置
  - 连接状态说明
  - 性能监控
  - 常见场景
  - 故障排查
  - 安全建议
  - 升级指南

- **[webrtc-user-guide.md](webrtc-user-guide.md)** - WebRTC 功能配置指南
  - STUN/TURN 服务器配置
  - 连接状态监控
  - 性能指标查看

- **[webrtc-troubleshooting.md](webrtc-troubleshooting.md)** - WebRTC 故障排查
  - 常见问题及解决方案
  - 连接失败诊断
  - 性能问题排查

## 开发者文档

- **[DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md)** - 开发者指南
  - 构建说明
  - 测试指南
  - 调试技巧
  - 贡献指南
  - 性能优化建议

- **[PERFORMANCE_BENCHMARK.md](PERFORMANCE_BENCHMARK.md)** - 性能基准测试报告
  - Go 性能基准
  - WebRTC 连接性能
  - 吞吐量测试
  - 延迟对比
  - 并发连接测试
  - 弱网环境测试
  - 内存使用分析
  - CPU 性能分析

## 运维文档

- **[gray-release-plan.md](gray-release-plan.md)** - 灰度发布计划
  - 10% → 50% → 100% 发布策略
  - 监控指标
  - 回滚方案

## 设计文档

位于 `superpowers/` 目录：

- **[superpowers/specs/2026-05-14-webrtc-transport-design.md](superpowers/specs/2026-05-14-webrtc-transport-design.md)** - WebRTC 传输优化设计文档
- **[superpowers/specs/2026-05-14-design-review-summary.md](superpowers/specs/2026-05-14-design-review-summary.md)** - 设计评审总结
- **[superpowers/plans/2026-05-14-webrtc-transport-implementation.md](superpowers/plans/2026-05-14-webrtc-transport-implementation.md)** - 实施计划（35个任务）

## 文档层级

```
docs/
├── README.md                          # 本文件 - 文档导航
├── USER_MANUAL.md                     # 用户手册（主要）
├── webrtc-user-guide.md              # WebRTC 配置指南
├── webrtc-troubleshooting.md         # 故障排查
├── DEVELOPER_GUIDE.md                # 开发者指南
├── PERFORMANCE_BENCHMARK.md          # 性能基准
├── gray-release-plan.md              # 灰度发布计划
└── superpowers/                      # 设计文档
    ├── specs/                        # 设计规格
    │   ├── 2026-05-14-webrtc-transport-design.md
    │   └── 2026-05-14-design-review-summary.md
    └── plans/                        # 实施计划
        └── 2026-05-14-webrtc-transport-implementation.md
```

## 快速链接

- 新用户？从 [USER_MANUAL.md](USER_MANUAL.md) 开始
- 开发者？查看 [DEVELOPER_GUIDE.md](DEVELOPER_GUIDE.md)
- 遇到问题？参考 [webrtc-troubleshooting.md](webrtc-troubleshooting.md)
- 性能数据？查看 [PERFORMANCE_BENCHMARK.md](PERFORMANCE_BENCHMARK.md)
