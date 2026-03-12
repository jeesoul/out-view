package com.outview.entity;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.LocalDateTime;

/**
 * 端口映射
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class PortMapping {

    /**
     * 对外端口
     */
    private int externalPort;

    /**
     * 设备ID
     */
    private String deviceId;

    /**
     * 目标端口 (内网 RDP 端口)
     */
    private int targetPort;

    /**
     * 创建时间
     */
    private LocalDateTime createTime;
}