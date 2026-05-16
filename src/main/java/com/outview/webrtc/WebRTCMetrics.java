package com.outview.webrtc;

import org.springframework.stereotype.Component;

import java.util.concurrent.atomic.LongAdder;

/**
 * 服务端 WebRTC 指标收集器：连接计数、成功率、回退率等。
 *
 * <p>使用 {@link LongAdder} 保证高并发下计数性能，{@code getXxxRate()} 计算时
 * 取一致性较弱的快照（容许极小竞态），不影响指标用途。
 */
@Component
public class WebRTCMetrics {

    private final LongAdder connectionsTotal = new LongAdder();
    private final LongAdder connectionsActive = new LongAdder();
    private final LongAdder successCount = new LongAdder();
    private final LongAdder fallbackCount = new LongAdder();
    private final LongAdder errorsTotal = new LongAdder();

    public void recordConnectionAttempt() {
        connectionsTotal.increment();
        connectionsActive.increment();
    }

    public void recordConnectionSuccess() {
        successCount.increment();
    }

    public void recordConnectionClosed() {
        connectionsActive.decrement();
    }

    public void recordFallback() {
        fallbackCount.increment();
    }

    public void recordError() {
        errorsTotal.increment();
    }

    public long getConnectionsTotal() {
        return connectionsTotal.sum();
    }

    public long getConnectionsActive() {
        return connectionsActive.sum();
    }

    public long getSuccessCount() {
        return successCount.sum();
    }

    public long getFallbackCount() {
        return fallbackCount.sum();
    }

    public long getErrorsTotal() {
        return errorsTotal.sum();
    }

    public double getSuccessRate() {
        long total = connectionsTotal.sum();
        return total == 0 ? 0.0 : (double) successCount.sum() / total;
    }

    public double getFallbackRate() {
        long total = connectionsTotal.sum();
        return total == 0 ? 0.0 : (double) fallbackCount.sum() / total;
    }

    /** 复位所有计数器，主要用于测试与运维诊断。 */
    public void reset() {
        connectionsTotal.reset();
        connectionsActive.reset();
        successCount.reset();
        fallbackCount.reset();
        errorsTotal.reset();
    }
}
