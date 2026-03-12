package com.outview.service;

import com.outview.entity.PresetConfig;
import com.outview.repository.PresetConfigRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

/**
 * 预设配置服务
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class PresetConfigService {

    private final PresetConfigRepository presetConfigRepository;

    /**
     * 创建预设配置
     */
    @Transactional
    public PresetConfig createPreset(String clientId, String token, Integer fixedPort, String description) {
        // 检查 clientId 是否已存在
        if (presetConfigRepository.existsByClientId(clientId)) {
            throw new IllegalArgumentException("ClientId already exists: " + clientId);
        }

        // 如果指定了固定端口，检查端口是否已被占用
        if (fixedPort != null && presetConfigRepository.findByFixedPortAndEnabledTrue(fixedPort).isPresent()) {
            throw new IllegalArgumentException("Fixed port already in use: " + fixedPort);
        }

        PresetConfig config = PresetConfig.builder()
                .clientId(clientId)
                .token(token != null ? token : generateToken())
                .fixedPort(fixedPort)
                .description(description)
                .enabled(true)
                .createTime(LocalDateTime.now())
                .build();

        config = presetConfigRepository.save(config);
        log.info("Preset config created: clientId={}, fixedPort={}", clientId, fixedPort);
        return config;
    }

    /**
     * 更新预设配置
     */
    @Transactional
    public PresetConfig updatePreset(Long id, String token, Integer fixedPort, String description, Boolean enabled) {
        PresetConfig config = presetConfigRepository.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("Preset config not found: " + id));

        if (token != null) {
            config.setToken(token);
        }
        if (fixedPort != null) {
            // 检查端口是否已被其他配置占用
            Optional<PresetConfig> existing = presetConfigRepository.findByFixedPortAndEnabledTrue(fixedPort);
            if (existing.isPresent() && !existing.get().getId().equals(id)) {
                throw new IllegalArgumentException("Fixed port already in use: " + fixedPort);
            }
            config.setFixedPort(fixedPort);
        }
        if (description != null) {
            config.setDescription(description);
        }
        if (enabled != null) {
            config.setEnabled(enabled);
        }
        config.setUpdateTime(LocalDateTime.now());

        config = presetConfigRepository.save(config);
        log.info("Preset config updated: id={}, clientId={}", id, config.getClientId());
        return config;
    }

    /**
     * 删除预设配置
     */
    @Transactional
    public void deletePreset(Long id) {
        if (!presetConfigRepository.existsById(id)) {
            throw new IllegalArgumentException("Preset config not found: " + id);
        }
        presetConfigRepository.deleteById(id);
        log.info("Preset config deleted: id={}", id);
    }

    /**
     * 根据客户端ID查询
     */
    public Optional<PresetConfig> getByClientId(String clientId) {
        return presetConfigRepository.findByClientId(clientId);
    }

    /**
     * 根据Token查询
     */
    public Optional<PresetConfig> getByToken(String token) {
        return presetConfigRepository.findByToken(token);
    }

    /**
     * 获取所有预设配置
     */
    public List<PresetConfig> getAllPresets() {
        return presetConfigRepository.findAll();
    }

    /**
     * 获取所有启用的预设配置
     */
    public List<PresetConfig> getEnabledPresets() {
        return presetConfigRepository.findByEnabledTrue();
    }

    /**
     * 验证Token
     * 返回验证结果和对应的预设配置（如果有）
     */
    public ValidationResult validateToken(String clientId, String token) {
        // 先按 clientId 查询预设配置
        Optional<PresetConfig> presetOpt = presetConfigRepository.findByClientIdAndEnabledTrue(clientId);

        if (presetOpt.isPresent()) {
            PresetConfig preset = presetOpt.get();
            // 预设配置存在，验证Token是否匹配
            if (preset.getToken().equals(token)) {
                return new ValidationResult(true, true, preset);
            } else {
                log.warn("Token mismatch for preset clientId: {}", clientId);
                return new ValidationResult(false, true, null);
            }
        }

        // 没有预设配置，检查是否是随机Token
        // 随机Token也能通过验证
        return new ValidationResult(true, false, null);
    }

    /**
     * 生成随机Token
     */
    public String generateToken() {
        return UUID.randomUUID().toString().replace("-", "");
    }

    /**
     * 生成随机ClientId
     */
    public String generateClientId() {
        return "device-" + UUID.randomUUID().toString().substring(0, 8);
    }

    /**
     * 验证结果
     */
    public static class ValidationResult {
        private final boolean valid;
        private final boolean isPreset;
        private final PresetConfig presetConfig;

        public ValidationResult(boolean valid, boolean isPreset, PresetConfig presetConfig) {
            this.valid = valid;
            this.isPreset = isPreset;
            this.presetConfig = presetConfig;
        }

        public boolean isValid() {
            return valid;
        }

        public boolean isPreset() {
            return isPreset;
        }

        public PresetConfig getPresetConfig() {
            return presetConfig;
        }
    }
}