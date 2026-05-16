package com.outview.webrtc;

import org.springframework.stereotype.Component;

import java.util.Arrays;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicLongArray;
import java.util.concurrent.atomic.LongAdder;

/**
 * 服务端 WebRTC 指标收集器：连接计数、成功率、回退率、握手耗时分位数、
 * 应用层 RTT 分位数、累计字节、吞吐 Mbps 等。
 *
 * <p>所有计数器使用 {@link LongAdder} 或 {@link AtomicLong}，避免锁；分位数
 * 通过环形缓冲快照计算，读侧不阻塞写侧。{@code getXxxRate()} 与 {@link #snapshot()}
 * 取一致性较弱的快照，容许极小竞态，不影响指标用途。
 */
@Component
public class WebRTCMetrics {

    static final int ESTABLISH_SAMPLE_SIZE = 100;
    static final int LATENCY_SAMPLE_SIZE = 100;

    private final LongAdder connectionsTotal = new LongAdder();
    private final LongAdder connectionsActive = new LongAdder();
    private final LongAdder successCount = new LongAdder();
    private final LongAdder fallbackCount = new LongAdder();
    private final LongAdder errorsTotal = new LongAdder();

    private final LongAdder bytesSent = new LongAdder();
    private final LongAdder bytesReceived = new LongAdder();

    private final AtomicLongArray establishSamples = new AtomicLongArray(ESTABLISH_SAMPLE_SIZE);
    private final AtomicLong establishIdx = new AtomicLong();

    private final AtomicLongArray latencySamples = new AtomicLongArray(LATENCY_SAMPLE_SIZE);
    private final AtomicLong latencyIdx = new AtomicLong();

    private volatile long startTimeNanos = System.nanoTime();

    public void recordConnectionAttempt() {
        connectionsTotal.increment();
        connectionsActive.increment();
    }

    /** 记录建链成功（不记录耗时，向后兼容）。 */
    public void recordConnectionSuccess() {
        successCount.increment();
    }

    /** 记录建链成功并写入握手耗时（毫秒）到环形缓冲。 */
    public void recordConnectionSuccess(long establishDurationMs) {
        successCount.increment();
        if (establishDurationMs > 0) {
            long idx = establishIdx.getAndIncrement();
            establishSamples.set((int) Math.floorMod(idx, ESTABLISH_SAMPLE_SIZE), establishDurationMs);
        }
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

    /** 记录应用层 RTT（毫秒）。非正值会被丢弃，避免污染分位数。 */
    public void recordLatency(long latencyMs) {
        if (latencyMs <= 0) {
            return;
        }
        long idx = latencyIdx.getAndIncrement();
        latencySamples.set((int) Math.floorMod(idx, LATENCY_SAMPLE_SIZE), latencyMs);
    }

    public void recordBytesSent(long n) {
        if (n > 0) {
            bytesSent.add(n);
        }
    }

    public void recordBytesReceived(long n) {
        if (n > 0) {
            bytesReceived.add(n);
        }
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

    public long getBytesSent() {
        return bytesSent.sum();
    }

    public long getBytesReceived() {
        return bytesReceived.sum();
    }

    public double getSuccessRate() {
        long total = connectionsTotal.sum();
        return total == 0 ? 0.0 : (double) successCount.sum() / total;
    }

    public double getFallbackRate() {
        long total = connectionsTotal.sum();
        return total == 0 ? 0.0 : (double) fallbackCount.sum() / total;
    }

    /** 自启动以来的发送吞吐，单位 Mbps。 */
    public double getThroughputSentMbps() {
        return throughputMbps(bytesSent.sum());
    }

    /** 自启动以来的接收吞吐，单位 Mbps。 */
    public double getThroughputReceivedMbps() {
        return throughputMbps(bytesReceived.sum());
    }

    private double throughputMbps(long bytes) {
        long elapsedNanos = System.nanoTime() - startTimeNanos;
        if (elapsedNanos <= 0) {
            return 0.0;
        }
        // bytes * 8 / 1_000_000 / (elapsedNanos / 1e9)
        return (double) bytes * 8.0 * 1_000.0 / (double) elapsedNanos;
    }

    public double getEstablishP50Ms() {
        return percentile(establishSamples, establishIdx.get(), ESTABLISH_SAMPLE_SIZE, 0.50);
    }

    public double getEstablishP95Ms() {
        return percentile(establishSamples, establishIdx.get(), ESTABLISH_SAMPLE_SIZE, 0.95);
    }

    public double getEstablishP99Ms() {
        return percentile(establishSamples, establishIdx.get(), ESTABLISH_SAMPLE_SIZE, 0.99);
    }

    public double getLatencyP50Ms() {
        return percentile(latencySamples, latencyIdx.get(), LATENCY_SAMPLE_SIZE, 0.50);
    }

    public double getLatencyP95Ms() {
        return percentile(latencySamples, latencyIdx.get(), LATENCY_SAMPLE_SIZE, 0.95);
    }

    public double getLatencyP99Ms() {
        return percentile(latencySamples, latencyIdx.get(), LATENCY_SAMPLE_SIZE, 0.99);
    }

    /**
     * 计算环形缓冲中已填充部分的 p 分位数（0&lt;=p&lt;=1）。
     * 0 或负值的样本视为未填充槽位，被忽略。
     */
    private static double percentile(AtomicLongArray buf, long writtenCount, int capacity, double p) {
        if (writtenCount <= 0) {
            return 0.0;
        }
        int n = (int) Math.min(writtenCount, (long) capacity);
        long[] samples = new long[n];
        int collected = 0;
        for (int i = 0; i < n; i++) {
            long v = buf.get(i);
            if (v > 0) {
                samples[collected++] = v;
            }
        }
        if (collected == 0) {
            return 0.0;
        }
        long[] valid = (collected == samples.length) ? samples : Arrays.copyOf(samples, collected);
        Arrays.sort(valid);
        int idx = (int) ((valid.length - 1) * p);
        if (idx < 0) idx = 0;
        if (idx >= valid.length) idx = valid.length - 1;
        return valid[idx];
    }

    /** 取一份点-in-time 快照，便于一次性导出所有指标（如 Prometheus 抓取）。 */
    public Snapshot snapshot() {
        long total = connectionsTotal.sum();
        long success = successCount.sum();
        long fallback = fallbackCount.sum();
        long bSent = bytesSent.sum();
        long bRecv = bytesReceived.sum();

        Snapshot s = new Snapshot();
        s.connectionsTotal = total;
        s.connectionsActive = connectionsActive.sum();
        s.successCount = success;
        s.fallbackCount = fallback;
        s.errorsTotal = errorsTotal.sum();
        s.successRate = total == 0 ? 0.0 : (double) success / total;
        s.fallbackRate = total == 0 ? 0.0 : (double) fallback / total;
        s.establishP50Ms = getEstablishP50Ms();
        s.establishP95Ms = getEstablishP95Ms();
        s.establishP99Ms = getEstablishP99Ms();
        s.latencyP50Ms = getLatencyP50Ms();
        s.latencyP95Ms = getLatencyP95Ms();
        s.latencyP99Ms = getLatencyP99Ms();
        s.bytesSent = bSent;
        s.bytesReceived = bRecv;
        s.throughputSentMbps = throughputMbps(bSent);
        s.throughputReceivedMbps = throughputMbps(bRecv);
        s.uptimeSeconds = (System.nanoTime() - startTimeNanos) / 1e9;
        return s;
    }

    /** 复位所有计数器与采样窗口，主要用于测试与运维诊断。 */
    public void reset() {
        connectionsTotal.reset();
        connectionsActive.reset();
        successCount.reset();
        fallbackCount.reset();
        errorsTotal.reset();
        bytesSent.reset();
        bytesReceived.reset();
        for (int i = 0; i < ESTABLISH_SAMPLE_SIZE; i++) {
            establishSamples.set(i, 0L);
        }
        for (int i = 0; i < LATENCY_SAMPLE_SIZE; i++) {
            latencySamples.set(i, 0L);
        }
        establishIdx.set(0);
        latencyIdx.set(0);
        startTimeNanos = System.nanoTime();
    }

    /** 一次性快照，避免读侧多次取值导致的不一致。 */
    public static class Snapshot {
        public long connectionsTotal;
        public long connectionsActive;
        public long successCount;
        public long fallbackCount;
        public long errorsTotal;
        public double successRate;
        public double fallbackRate;
        public double establishP50Ms;
        public double establishP95Ms;
        public double establishP99Ms;
        public double latencyP50Ms;
        public double latencyP95Ms;
        public double latencyP99Ms;
        public long bytesSent;
        public long bytesReceived;
        public double throughputSentMbps;
        public double throughputReceivedMbps;
        public double uptimeSeconds;
    }
}
