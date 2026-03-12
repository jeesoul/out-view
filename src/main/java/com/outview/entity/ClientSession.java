package com.outview.entity;

import io.netty.channel.Channel;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.LocalDateTime;

/**
 * 客户端会话
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ClientSession {

    /**
     * 设备ID
     */
    private String deviceId;

    /**
     * Token
     */
    private String token;

    /**
     * 客户端 Channel
     */
    private Channel channel;

    /**
     * 本地端口 (RDP 端口)
     */
    private int localPort;

    /**
     * 分配的对外端口
     */
    private int externalPort;

    /**
     * 上次心跳时间
     */
    private LocalDateTime lastHeartbeat;

    /**
     * 创建时间
     */
    private LocalDateTime createTime;

    /**
     * 会话状态
     */
    private SessionStatus status;

    /**
     * 会话状态枚举
     */
    public enum SessionStatus {
        /**
         * 在线
         */
        ONLINE,
        /**
         * 离线
         */
        OFFLINE,
        /**
         * 超时
         */
        TIMEOUT
    }

    /**
     * 更新心跳时间
     */
    public void updateHeartbeat() {
        this.lastHeartbeat = LocalDateTime.now();
    }

    /**
     * 检查是否活跃
     */
    public boolean isActive() {
        return channel != null && channel.isActive() && status == SessionStatus.ONLINE;
    }
}