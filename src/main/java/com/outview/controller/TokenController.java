package com.outview.controller;

import com.outview.service.PortMappingService;
import org.springframework.web.bind.annotation.*;

import java.util.*;

/**
 * Token 管理 REST API
 * (简化版，实际生产环境应使用数据库存储)
 */
@RestController
@RequestMapping("/api/tokens")
public class TokenController {

    private final PortMappingService portMappingService;

    public TokenController(PortMappingService portMappingService) {
        this.portMappingService = portMappingService;
    }

    /**
     * 生成新 Token
     */
    @PostMapping
    public Map<String, Object> generateToken(@RequestBody(required = false) Map<String, Object> params) {
        String deviceId = UUID.randomUUID().toString().replace("-", "").substring(0, 16);
        String token = UUID.randomUUID().toString().replace("-", "");

        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("deviceId", deviceId);
        result.put("token", token);
        result.put("createTime", new Date());
        return result;
    }

    /**
     * 查询 Token 状态
     */
    @GetMapping("/{deviceId}")
    public Map<String, Object> getTokenStatus(@PathVariable String deviceId) {
        Integer port = portMappingService.getPortByDevice(deviceId);

        Map<String, Object> result = new HashMap<>();
        result.put("deviceId", deviceId);
        result.put("active", port != null);
        result.put("externalPort", port);
        return result;
    }
}