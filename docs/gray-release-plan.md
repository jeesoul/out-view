# outView 1.2.0 WebRTC 灰度发布方案

## 发布目标

将 WebRTC 传输层从 0% 逐步推广到 100% 用户，确保：
- 连接成功率 ≥ 99.5%（含 TCP 降级）
- WebRTC 直连率 ≥ 80%
- P95 延迟降低 ≥ 30%
- 零生产事故

## 发布时间线

### 第 1 天：10% 灰度

**配置**:
```yaml
webrtc:
  gray-release:
    enabled: true
    percentage: 10
```

**监控指标**:
- `webrtc_connections_active` > 0（确认 WebRTC 在使用）
- `webrtc_success_rate` ≥ 0.80（WebRTC 直连成功率）
- `webrtc_fallback_rate` ≤ 0.20（TCP 降级率）
- `webrtc_establish_duration_ms_p95` ≤ 3000ms
- 总连接成功率（WebRTC + TCP 降级）≥ 99.5%

**回滚条件**:
- 总连接成功率 < 99%
- 用户投诉增加 > 10%
- 服务端 CPU/内存异常

**观察时间**: 24 小时

---

### 第 3 天：50% 灰度

**前提**: 第 1 天指标全部达标

**配置**:
```yaml
webrtc:
  gray-release:
    percentage: 50
```

**额外监控**:
- 不同 NAT 类型的成功率分布
- TURN 服务器负载
- Sidecar 进程内存使用（应 < 100MB/连接）

**观察时间**: 48 小时

---

### 第 5 天：100% 全量

**前提**: 第 3 天指标全部达标

**配置**:
```yaml
webrtc:
  gray-release:
    percentage: 100
```

**全量后持续监控**: 7 天

---

## 回滚方案

### 快速回滚（< 5 分钟）

```yaml
# 方案 1: 禁用灰度
webrtc:
  gray-release:
    enabled: false

# 方案 2: 完全禁用 WebRTC
webrtc:
  enabled: false
```

无需重启服务，配置热更新生效。

### 回滚触发条件

| 指标 | 阈值 | 动作 |
|------|------|------|
| 总连接成功率 | < 99% | 立即回滚 |
| WebRTC 降级率 | > 50% | 降低灰度比例 |
| P95 延迟 | > 5000ms | 调查后决定 |
| Sidecar 崩溃率 | > 1% | 立即回滚 |
| 用户投诉 | 显著增加 | 立即回滚 |

---

## 监控看板

### Grafana 面板（建议）

```
Row 1: 连接概览
  - webrtc_connections_active (gauge)
  - webrtc_success_rate (gauge, 目标 > 80%)
  - webrtc_fallback_rate (gauge, 目标 < 20%)
  - total_connection_success_rate (gauge, 目标 > 99.5%)

Row 2: 性能指标
  - webrtc_establish_duration_ms P50/P95/P99 (time series)
  - webrtc_latency_ms P50/P95 (time series)
  - webrtc_throughput_mbps (time series)

Row 3: 错误分析
  - webrtc_errors_total by reason (bar chart)
  - webrtc_fallback_count by reason (bar chart)
  - sidecar_restart_count (counter)
```

### 告警规则

```yaml
# Prometheus alerting rules
groups:
  - name: webrtc
    rules:
      - alert: WebRTCHighFallbackRate
        expr: webrtc_fallback_rate > 0.3
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "WebRTC fallback rate too high: {{ $value }}"

      - alert: WebRTCConnectionFailure
        expr: (webrtc_connections_total - webrtc_success_count - webrtc_fallback_count) / webrtc_connections_total > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "WebRTC connection failure rate > 1%"

      - alert: WebRTCHighLatency
        expr: webrtc_establish_duration_ms_p95 > 5000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "WebRTC P95 establish time > 5s: {{ $value }}ms"
```

---

## 发布检查清单

### 发布前

- [ ] 所有单元测试通过（`go test ./...`）
- [ ] Java 集成测试通过（`mvn test`）
- [ ] Sidecar 二进制已构建并签名
- [ ] 监控看板已配置
- [ ] 告警规则已配置
- [ ] 回滚方案已验证
- [ ] 文档已更新
- [ ] 值班人员已通知

### 发布中

- [ ] 10% 灰度：观察 24h，指标达标
- [ ] 50% 灰度：观察 48h，指标达标
- [ ] 100% 全量：持续监控 7 天

### 发布后

- [ ] 生成灰度报告
- [ ] 更新版本号到 1.2.0
- [ ] 打 Git tag: `v1.2.0`
- [ ] 发布 Release Notes

---

## 预期效果

基于 POC 测试数据：

| 指标 | TCP (当前) | WebRTC (预期) | 改善 |
|------|-----------|--------------|------|
| P50 延迟 | 20ms | 12ms | -40% |
| P95 延迟 | 50ms | 28ms | -44% |
| 弱网断连率 | 5% | 1% | -80% |
| 连接建立时间 | 100ms | 800ms | -8x (首次) |
| 重连时间 | 3s | 1s | -67% |

注：WebRTC 首次连接需要 ICE 协商，但重连更快。整体用户体验显著提升。
