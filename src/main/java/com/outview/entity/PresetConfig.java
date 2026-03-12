package com.outview.entity;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import javax.persistence.*;
import java.time.LocalDateTime;

/**
 * 预设配置
 * 预先设置的 ClientId/Token/端口 映射
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
@Entity
@Table(name = "preset_config", indexes = {
        @Index(name = "idx_client_id", columnList = "clientId", unique = true),
        @Index(name = "idx_token", columnList = "token")
})
public class PresetConfig {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    /**
     * 客户端ID (唯一)
     */
    @Column(nullable = false, unique = true, length = 64)
    private String clientId;

    /**
     * 认证Token
     */
    @Column(nullable = false, length = 128)
    private String token;

    /**
     * 固定端口 (null 表示随机分配)
     */
    @Column
    private Integer fixedPort;

    /**
     * 描述
     */
    @Column(length = 256)
    private String description;

    /**
     * 是否启用
     */
    @Column(nullable = false)
    @Builder.Default
    private Boolean enabled = true;

    /**
     * 创建时间
     */
    @Column(nullable = false)
    private LocalDateTime createTime;

    /**
     * 更新时间
     */
    @Column
    private LocalDateTime updateTime;
}