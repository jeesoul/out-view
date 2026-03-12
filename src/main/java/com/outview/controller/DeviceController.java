package com.outview.controller;

import com.outview.entity.ClientSession;
import com.outview.entity.PortMapping;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import org.springframework.web.bind.annotation.*;

import java.util.*;

/**
 * 设备管理 REST API
 */
@RestController
@RequestMapping("/api/devices")
public class DeviceController {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;

    public DeviceController(SessionStore sessionStore, PortMappingService portMappingService) {
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
    }

    /**
     * 获取所有在线设备
     */
    @GetMapping
    public Map<String, Object> listDevices() {
        List<Map<String, Object>> devices = new ArrayList<>();
        for (ClientSession session : sessionStore.getAllSessions()) {
            Map<String, Object> device = new HashMap<>();
            device.put("deviceId", session.getDeviceId());
            device.put("externalPort", session.getExternalPort());
            device.put("localPort", session.getLocalPort());
            device.put("status", session.getStatus().name());
            device.put("lastHeartbeat", session.getLastHeartbeat());
            device.put("createTime", session.getCreateTime());
            devices.add(device);
        }

        Map<String, Object> result = new HashMap<>();
        result.put("total", devices.size());
        result.put("online", sessionStore.getOnlineCount());
        result.put("devices", devices);
        return result;
    }

    /**
     * 获取单个设备信息
     */
    @GetMapping("/{deviceId}")
    public Map<String, Object> getDevice(@PathVariable String deviceId) {
        ClientSession session = sessionStore.getSession(deviceId);
        if (session == null) {
            return Collections.singletonMap("error", "Device not found");
        }

        Map<String, Object> device = new HashMap<>();
        device.put("deviceId", session.getDeviceId());
        device.put("externalPort", session.getExternalPort());
        device.put("localPort", session.getLocalPort());
        device.put("status", session.getStatus().name());
        device.put("lastHeartbeat", session.getLastHeartbeat());
        device.put("createTime", session.getCreateTime());
        return device;
    }

    /**
     * 强制下线设备
     */
    @DeleteMapping("/{deviceId}")
    public Map<String, Object> disconnectDevice(@PathVariable String deviceId) {
        ClientSession session = sessionStore.getSession(deviceId);
        if (session == null) {
            return Collections.singletonMap("error", "Device not found");
        }

        // 关闭连接
        if (session.getChannel() != null && session.getChannel().isActive()) {
            session.getChannel().close();
        }

        // 释放端口
        portMappingService.releasePort(deviceId);

        // 移除会话
        sessionStore.removeSession(deviceId);

        return Collections.singletonMap("success", true);
    }

    /**
     * 获取端口映射列表
     */
    @GetMapping("/mappings")
    public Map<String, Object> listMappings() {
        List<Map<String, Object>> mappings = new ArrayList<>();
        for (PortMapping mapping : portMappingService.getAllMappings().values()) {
            Map<String, Object> m = new HashMap<>();
            m.put("externalPort", mapping.getExternalPort());
            m.put("deviceId", mapping.getDeviceId());
            m.put("targetPort", mapping.getTargetPort());
            m.put("createTime", mapping.getCreateTime());
            mappings.add(m);
        }

        Map<String, Object> result = new HashMap<>();
        result.put("total", mappings.size());
        result.put("mappings", mappings);
        return result;
    }
}