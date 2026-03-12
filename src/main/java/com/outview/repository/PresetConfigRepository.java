package com.outview.repository;

import com.outview.entity.PresetConfig;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Optional;

/**
 * 预设配置 Repository
 */
@Repository
public interface PresetConfigRepository extends JpaRepository<PresetConfig, Long> {

    /**
     * 根据客户端ID查询
     */
    Optional<PresetConfig> findByClientId(String clientId);

    /**
     * 根据客户端ID查询启用的配置
     */
    Optional<PresetConfig> findByClientIdAndEnabledTrue(String clientId);

    /**
     * 根据Token查询
     */
    Optional<PresetConfig> findByToken(String token);

    /**
     * 查询所有启用的配置
     */
    List<PresetConfig> findByEnabledTrue();

    /**
     * 检查客户端ID是否存在
     */
    boolean existsByClientId(String clientId);

    /**
     * 检查Token是否存在
     */
    boolean existsByToken(String token);

    /**
     * 根据固定端口查询启用的配置
     */
    Optional<PresetConfig> findByFixedPortAndEnabledTrue(Integer fixedPort);
}