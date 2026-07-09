package com.outview.controller;

import com.outview.entity.BannedDevice;
import com.outview.entity.ClientSession;
import com.outview.entity.PortMapping;
import com.outview.service.BanService;
import com.outview.service.DataPortService;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.context.SecurityContextHolder;
import org.springframework.web.bind.annotation.*;

import java.util.*;

@RestController
@RequestMapping("/api/devices")
public class DeviceController {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;
    private final DataPortService dataPortService;
    private final BanService banService;

    public DeviceController(SessionStore sessionStore,
                            PortMappingService portMappingService,
                            DataPortService dataPortService,
                            BanService banService) {
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        this.dataPortService = dataPortService;
        this.banService = banService;
    }

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

    /** 普通断开：踢下线，允许重连 */
    @DeleteMapping("/{deviceId}")
    public Map<String, Object> disconnectDevice(@PathVariable String deviceId) {
        forceDisconnect(deviceId);
        return Collections.singletonMap("success", true);
    }

    /** 彻底封禁：踢下线 + 加入黑名单，重连时直接拒绝 */
    @PostMapping("/{deviceId}/ban")
    public Map<String, Object> banDevice(@PathVariable String deviceId,
                                         @RequestBody(required = false) Map<String, String> body) {
        String reason = body != null ? body.getOrDefault("reason", "管理员封禁") : "管理员封禁";
        String operator = currentUser();

        // 先踢下线
        forceDisconnect(deviceId);

        // 加入封禁名单
        banService.ban(deviceId, operator, reason);

        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("deviceId", deviceId);
        result.put("bannedBy", operator);
        result.put("reason", reason);
        return result;
    }

    /** 解封：从黑名单移除，允许重新连接 */
    @DeleteMapping("/{deviceId}/ban")
    public Map<String, Object> unbanDevice(@PathVariable String deviceId) {
        banService.unban(deviceId);
        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("deviceId", deviceId);
        return result;
    }

    /** 获取封禁列表 */
    @GetMapping("/banned")
    public Map<String, Object> listBanned() {
        List<Map<String, Object>> list = new ArrayList<>();
        for (BannedDevice b : banService.listBanned()) {
            Map<String, Object> m = new HashMap<>();
            m.put("deviceId", b.getDeviceId());
            m.put("bannedAt", b.getBannedAt());
            m.put("bannedBy", b.getBannedBy());
            m.put("reason", b.getReason());
            list.add(m);
        }
        Map<String, Object> result = new HashMap<>();
        result.put("total", list.size());
        result.put("banned", list);
        return result;
    }

    /** 更新设备的对外端口 */
    @PutMapping("/mappings/{deviceId}")
    public Map<String, Object> updateMapping(@PathVariable String deviceId,
                                             @RequestBody Map<String, Integer> body) {
        Integer newPort = body.get("externalPort");
        if (newPort == null || newPort < 1 || newPort > 65535) {
            Map<String, Object> err = new HashMap<>();
            err.put("success", false);
            err.put("error", "Invalid port number");
            return err;
        }
        try {
            int oldPort = portMappingService.updateExternalPort(deviceId, newPort);
            if (oldPort < 0) {
                Map<String, Object> err = new HashMap<>();
                err.put("success", false);
                err.put("error", "Device not found or not connected");
                return err;
            }
            // Restart the data port listener on the new port
            dataPortService.stopDataPort(oldPort);
            dataPortService.startDataPort(newPort, deviceId);

            Map<String, Object> result = new HashMap<>();
            result.put("success", true);
            result.put("deviceId", deviceId);
            result.put("oldPort", oldPort);
            result.put("newPort", newPort);
            return result;
        } catch (IllegalArgumentException e) {
            Map<String, Object> err = new HashMap<>();
            err.put("success", false);
            err.put("error", e.getMessage());
            return err;
        }
    }

    @GetMapping("/mappings")
    public Map<String, Object> listMappings() {
        List<Map<String, Object>> mappings = new ArrayList<>();
        for (PortMapping mapping : portMappingService.getAllMappings().values()) {
            Map<String, Object> m = new HashMap<>();
            m.put("externalPort", mapping.getExternalPort());
            m.put("deviceId", mapping.getDeviceId());
            m.put("targetPort", mapping.getTargetPort());
            m.put("online", mapping.isOnline());
            m.put("createTime", mapping.getCreateTime());
            m.put("lastOnlineTime", mapping.getLastOnlineTime());
            mappings.add(m);
        }
        // 在线优先，再按端口升序
        mappings.sort((a, b) -> {
            boolean aOnline = Boolean.TRUE.equals(a.get("online"));
            boolean bOnline = Boolean.TRUE.equals(b.get("online"));
            if (aOnline != bOnline) return aOnline ? -1 : 1;
            return Integer.compare((Integer) a.get("externalPort"), (Integer) b.get("externalPort"));
        });
        Map<String, Object> result = new HashMap<>();
        result.put("total", mappings.size());
        result.put("mappings", mappings);
        return result;
    }

    /** 预设固定端口映射（设备未上线也可预设，客户端上线时自动复用） */
    @PostMapping("/mappings")
    public Map<String, Object> presetMapping(@RequestBody Map<String, Object> body) {
        String deviceId = body.get("deviceId") instanceof String ? (String) body.get("deviceId") : null;
        Integer externalPort = body.get("externalPort") instanceof Number
                ? ((Number) body.get("externalPort")).intValue() : null;
        Integer targetPort = body.get("targetPort") instanceof Number
                ? ((Number) body.get("targetPort")).intValue() : null;

        Map<String, Object> result = new HashMap<>();
        if (deviceId == null || deviceId.trim().isEmpty() || externalPort == null) {
            result.put("success", false);
            result.put("error", "deviceId 和 externalPort 不能为空");
            return result;
        }
        if (targetPort == null) {
            targetPort = 3389; // 默认 RDP
        }

        String failReason = portMappingService.setFixedPort(deviceId.trim(), externalPort, targetPort);
        if (failReason != null) {
            result.put("success", false);
            result.put("error", failReason);
        } else {
            result.put("success", true);
            result.put("deviceId", deviceId.trim());
            result.put("externalPort", externalPort);
            result.put("targetPort", targetPort);
        }
        return result;
    }

    /** 删除固定端口映射（释放端口；若设备在线则一并断开） */
    @DeleteMapping("/mappings/{deviceId}")
    public Map<String, Object> deleteMapping(@PathVariable String deviceId) {
        ClientSession session = sessionStore.getSession(deviceId);
        if (session != null) {
            if (session.getChannel() != null && session.getChannel().isActive()) {
                session.getChannel().close();
            }
            dataPortService.stopDataPort(session.getExternalPort());
            sessionStore.removeSession(deviceId);
        }
        portMappingService.releasePort(deviceId);

        Map<String, Object> result = new HashMap<>();
        result.put("success", true);
        result.put("deviceId", deviceId);
        return result;
    }

    private void forceDisconnect(String deviceId) {
        ClientSession session = sessionStore.getSession(deviceId);
        if (session != null) {
            if (session.getChannel() != null && session.getChannel().isActive()) {
                session.getChannel().close();
            }
            dataPortService.stopDataPort(session.getExternalPort());
            portMappingService.markOffline(deviceId);
            sessionStore.removeSession(deviceId);
        }
    }

    private String currentUser() {
        Authentication auth = SecurityContextHolder.getContext().getAuthentication();
        return auth != null ? auth.getName() : "system";
    }
}
