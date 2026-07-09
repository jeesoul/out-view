package com.outview.entity;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import javax.persistence.*;
import java.time.LocalDateTime;

/**
 * 端口映射（持久化）
 * <p>
 * 记录设备 ID 与对外固定端口的映射关系。客户端首次上线时分配端口并持久化，
 * 此后客户端重连或服务端重启，均复用同一端口，实现"固定映射端口"。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
@Entity
@Table(name = "port_mapping")
public class PortMapping {

    /**
     * 设备ID（主键，唯一标识一台设备）
     */
    @Id
    @Column(name = "device_id", length = 64)
    private String deviceId;

    /**
     * 对外端口（固定，客户端重连后复用）
     */
    @Column(name = "external_port", nullable = false)
    private int externalPort;

    /**
     * 目标端口（内网 RDP 端口）
     */
    @Column(name = "target_port", nullable = false)
    private int targetPort;

    /**
     * 创建时间
     */
    @Column(name = "create_time")
    private LocalDateTime createTime;

    /**
     * 最后上线时间
     */
    @Column(name = "last_online_time")
    private LocalDateTime lastOnlineTime;

    /**
     * 是否当前在线
     */
    @Column(name = "online")
    private boolean online;
}
