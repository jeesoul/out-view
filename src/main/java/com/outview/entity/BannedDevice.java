package com.outview.entity;

import javax.persistence.*;
import java.time.LocalDateTime;

@Entity
@Table(name = "banned_device")
public class BannedDevice {

    @Id
    @Column(name = "device_id", length = 64)
    private String deviceId;

    @Column(name = "banned_at", nullable = false)
    private LocalDateTime bannedAt;

    @Column(name = "banned_by", length = 64)
    private String bannedBy;

    @Column(name = "reason", length = 255)
    private String reason;

    public BannedDevice() {}

    public BannedDevice(String deviceId, String bannedBy, String reason) {
        this.deviceId = deviceId;
        this.bannedAt = LocalDateTime.now();
        this.bannedBy = bannedBy;
        this.reason = reason;
    }

    public String getDeviceId() { return deviceId; }
    public void setDeviceId(String deviceId) { this.deviceId = deviceId; }
    public LocalDateTime getBannedAt() { return bannedAt; }
    public void setBannedAt(LocalDateTime bannedAt) { this.bannedAt = bannedAt; }
    public String getBannedBy() { return bannedBy; }
    public void setBannedBy(String bannedBy) { this.bannedBy = bannedBy; }
    public String getReason() { return reason; }
    public void setReason(String reason) { this.reason = reason; }
}
