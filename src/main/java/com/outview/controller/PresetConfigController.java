package com.outview.controller;

import com.alibaba.fastjson.JSON;
import com.outview.entity.PresetConfig;
import com.outview.service.PresetConfigService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 预设配置 Controller
 */
@RestController
@RequestMapping("/api/presets")
@RequiredArgsConstructor
public class PresetConfigController {

    private final PresetConfigService presetConfigService;

    /**
     * 获取所有预设配置
     */
    @GetMapping
    public ResponseEntity<Map<String, Object>> getAllPresets() {
        List<PresetConfig> presets = presetConfigService.getAllPresets();
        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("total", presets.size());
        result.put("presets", presets);
        return ResponseEntity.ok(result);
    }

    /**
     * 获取单个预设配置
     */
    @GetMapping("/{id}")
    public ResponseEntity<Map<String, Object>> getPreset(@PathVariable Long id) {
        return presetConfigService.getByClientId(id.toString())
                .map(preset -> {
                    Map<String, Object> result = new HashMap<>();
                    result.put("success", true);
                    result.put("preset", preset);
                    return ResponseEntity.ok(result);
                })
                .orElse(ResponseEntity.notFound().build());
    }

    /**
     * 创建预设配置
     */
    @PostMapping
    public ResponseEntity<Map<String, Object>> createPreset(@RequestBody String body) {
        Map<String, Object> result = new HashMap<>();
        try {
            Map<String, Object> params = JSON.parseObject(body, Map.class);
            String clientId = (String) params.get("clientId");
            String token = (String) params.get("token");
            Integer fixedPort = params.get("fixedPort") != null ?
                    Integer.parseInt(params.get("fixedPort").toString()) : null;
            String description = (String) params.get("description");

            // 如果未提供clientId，自动生成
            if (clientId == null || clientId.isEmpty()) {
                clientId = presetConfigService.generateClientId();
            }

            // 如果未提供token，自动生成
            if (token == null || token.isEmpty()) {
                token = presetConfigService.generateToken();
            }

            PresetConfig preset = presetConfigService.createPreset(clientId, token, fixedPort, description);
            result.put("success", true);
            result.put("preset", preset);
            result.put("message", "Preset config created successfully");
            return ResponseEntity.ok(result);
        } catch (IllegalArgumentException e) {
            result.put("success", false);
            result.put("message", e.getMessage());
            return ResponseEntity.badRequest().body(result);
        } catch (Exception e) {
            result.put("success", false);
            result.put("message", "Failed to create preset: " + e.getMessage());
            return ResponseEntity.internalServerError().body(result);
        }
    }

    /**
     * 更新预设配置
     */
    @PutMapping("/{id}")
    public ResponseEntity<Map<String, Object>> updatePreset(
            @PathVariable Long id,
            @RequestBody String body) {
        Map<String, Object> result = new HashMap<>();
        try {
            Map<String, Object> params = JSON.parseObject(body, Map.class);
            String token = (String) params.get("token");
            Integer fixedPort = params.get("fixedPort") != null ?
                    Integer.parseInt(params.get("fixedPort").toString()) : null;
            String description = (String) params.get("description");
            Boolean enabled = params.get("enabled") != null ?
                    Boolean.parseBoolean(params.get("enabled").toString()) : null;

            PresetConfig preset = presetConfigService.updatePreset(id, token, fixedPort, description, enabled);
            result.put("success", true);
            result.put("preset", preset);
            result.put("message", "Preset config updated successfully");
            return ResponseEntity.ok(result);
        } catch (IllegalArgumentException e) {
            result.put("success", false);
            result.put("message", e.getMessage());
            return ResponseEntity.badRequest().body(result);
        } catch (Exception e) {
            result.put("success", false);
            result.put("message", "Failed to update preset: " + e.getMessage());
            return ResponseEntity.internalServerError().body(result);
        }
    }

    /**
     * 删除预设配置
     */
    @DeleteMapping("/{id}")
    public ResponseEntity<Map<String, Object>> deletePreset(@PathVariable Long id) {
        Map<String, Object> result = new HashMap<>();
        try {
            presetConfigService.deletePreset(id);
            result.put("success", true);
            result.put("message", "Preset config deleted successfully");
            return ResponseEntity.ok(result);
        } catch (IllegalArgumentException e) {
            result.put("success", false);
            result.put("message", e.getMessage());
            return ResponseEntity.badRequest().body(result);
        } catch (Exception e) {
            result.put("success", false);
            result.put("message", "Failed to delete preset: " + e.getMessage());
            return ResponseEntity.internalServerError().body(result);
        }
    }

    /**
     * 生成随机ClientId和Token
     */
    @PostMapping("/generate")
    public ResponseEntity<Map<String, Object>> generateCredentials() {
        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("clientId", presetConfigService.generateClientId());
        result.put("token", presetConfigService.generateToken());
        return ResponseEntity.ok(result);
    }
}