package com.outview.webrtc;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Component
@ConfigurationProperties(prefix = "outview.webrtc")
public class WebRTCConfig {
    private boolean enabled = true;
    private String sidecarSocketPath = "/tmp/outview-webrtc.sock";
    private int connectTimeoutMs = 5000;
    private int readTimeoutMs = 30000;
    private int maxMessageSize = 4 * 1024 * 1024; // 4MB

    public boolean isEnabled() { return enabled; }
    public void setEnabled(boolean enabled) { this.enabled = enabled; }

    public String getSidecarSocketPath() { return sidecarSocketPath; }
    public void setSidecarSocketPath(String path) { this.sidecarSocketPath = path; }

    public int getConnectTimeoutMs() { return connectTimeoutMs; }
    public void setConnectTimeoutMs(int ms) { this.connectTimeoutMs = ms; }

    public int getReadTimeoutMs() { return readTimeoutMs; }
    public void setReadTimeoutMs(int ms) { this.readTimeoutMs = ms; }

    public int getMaxMessageSize() { return maxMessageSize; }
    public void setMaxMessageSize(int size) { this.maxMessageSize = size; }
}
