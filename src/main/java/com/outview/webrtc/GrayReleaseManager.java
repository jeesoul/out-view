package com.outview.webrtc;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * 灰度发布开关：根据设备 ID 的稳定哈希分桶决定是否启用 WebRTC。
 *
 * <p>同一个 deviceId 的判定结果保持稳定，便于灰度过程中的流量切分和回滚。
 * 仅当 {@code webrtc.gray-release.enabled=true} 且 {@code percentage>0} 时才会有设备命中 WebRTC。
 */
@Component
public class GrayReleaseManager {

    @Value("${webrtc.gray-release.enabled:false}")
    private boolean enabled;

    /** 灰度比例 0-100。<=0 关闭，>=100 全量。 */
    @Value("${webrtc.gray-release.percentage:0}")
    private int percentage;

    /**
     * 判定指定设备是否应使用 WebRTC。
     */
    public boolean shouldUseWebRTC(String deviceId) {
        if (!enabled || deviceId == null) {
            return false;
        }
        if (percentage <= 0) {
            return false;
        }
        if (percentage >= 100) {
            return true;
        }
        int bucket = Math.abs(deviceId.hashCode() % 100);
        return bucket < percentage;
    }

    public int getPercentage() {
        return percentage;
    }

    public boolean isEnabled() {
        return enabled;
    }
}
