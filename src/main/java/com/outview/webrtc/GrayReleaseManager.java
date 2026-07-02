package com.outview.webrtc;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/**
 * 灰度发布开关：根据设备 ID 的稳定哈希分桶决定是否启用 WebRTC。
 *
 * <p>同一个 deviceId 的判定结果保持稳定，便于灰度过程中的流量切分和回滚。
 * 仅当 {@code webrtc.gray-release.enabled=true} 且 {@code percentage>0} 时才会有设备命中 WebRTC。
 *
 * <p>支持运行时动态开关：通过 {@link #setEnabled(boolean)} 与 {@link #setPercentage(int)}
 * 在不重启进程的前提下调整灰度规则。读路径使用 atomic 字段避免锁开销。
 */
@Component
public class GrayReleaseManager {

    /** 灰度白名单触发的虚拟桶号，永远命中。 */
    private static final int FORCE_INCLUDE_BUCKET = -1;
    /** 灰度黑名单触发的虚拟桶号，永远不命中。 */
    private static final int FORCE_EXCLUDE_BUCKET = Integer.MAX_VALUE;

    private final AtomicReference<Boolean> enabledRef = new AtomicReference<>(Boolean.FALSE);
    private final AtomicInteger percentageRef = new AtomicInteger(0);

    /** Spring 注入的初始值；运行时更新通过 setters 改写 atomic 字段。 */
    @Value("${webrtc.gray-release.enabled:false}")
    private void initEnabled(boolean v) { enabledRef.set(v); }

    @Value("${webrtc.gray-release.percentage:0}")
    private void initPercentage(int v) { percentageRef.set(clampPercentage(v)); }

    /**
     * 判定指定设备是否应使用 WebRTC。无锁读路径，O(1) 哈希计算。
     *
     * @param deviceId 设备唯一标识；为 null 时直接返回 false
     */
    public boolean shouldUseWebRTC(String deviceId) {
        if (!enabledRef.get() || deviceId == null) {
            return false;
        }
        int pct = percentageRef.get();
        if (pct <= 0) {
            return false;
        }
        if (pct >= 100) {
            return true;
        }
        return bucketOf(deviceId) < pct;
    }

    /**
     * 计算 deviceId 的桶号（0-99）。先取模再取绝对值以正确处理
     * {@link Integer#MIN_VALUE}（{@code Math.abs(MIN_VALUE)} 仍为负数）。
     */
    static int bucketOf(String deviceId) {
        return Math.abs(deviceId.hashCode() % 100);
    }

    /** 当前灰度比例（0-100）。 */
    public int getPercentage() {
        return percentageRef.get();
    }

    /** 灰度开关是否打开。 */
    public boolean isEnabled() {
        return enabledRef.get();
    }

    /**
     * 运行时动态切换灰度开关。返回旧值，便于审计或回滚。
     */
    public boolean setEnabled(boolean enabled) {
        return enabledRef.getAndSet(enabled);
    }

    /**
     * 运行时动态调整灰度比例。入参越界会被夹紧到 [0,100]。返回应用后的值。
     */
    public int setPercentage(int percentage) {
        int v = clampPercentage(percentage);
        percentageRef.set(v);
        return v;
    }

    private static int clampPercentage(int v) {
        if (v < 0) return 0;
        if (v > 100) return 100;
        return v;
    }
}
