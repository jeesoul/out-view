package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.entity.PortMapping;
import com.outview.repository.PortMappingRepository;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import javax.annotation.PostConstruct;
import java.time.LocalDateTime;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * 端口映射服务
 * 管理对外端口与设备的映射关系（持久化）。
 * <p>
 * 客户端首次上线时分配端口并写入数据库，此后客户端重连或服务端重启均复用同一端口，
 * 实现"固定映射端口"。管理员可在管理后台预设/修改/删除固定端口。
 */
@Slf4j
@Service
public class PortMappingService {

    private final OutViewProperties properties;
    private final PortMappingRepository repository;

    /**
     * 端口 -> 映射（内存镜像，含在线+离线，启动时从 DB 加载）
     */
    private final Map<Integer, PortMapping> portMappingMap = new ConcurrentHashMap<>();

    /**
     * 设备ID -> 端口
     */
    private final Map<String, Integer> deviceToPortMap = new ConcurrentHashMap<>();

    /**
     * 下一个可用端口
     */
    private final AtomicInteger nextPort;

    public PortMappingService(OutViewProperties properties, PortMappingRepository repository) {
        this.properties = properties;
        this.repository = repository;
        this.nextPort = new AtomicInteger(properties.getDataPortStart());
    }

    /**
     * 启动时从 DB 加载全部持久化映射到内存。
     * 注意：服务端重启后所有设备都视为离线（端口监听尚未建立），
     * 待客户端重新上线时由 allocatePort 标记在线并重新 bind 端口。
     */
    @PostConstruct
    public void loadPersistedMappings() {
        for (PortMapping m : repository.findAll()) {
            portMappingMap.put(m.getExternalPort(), m);
            deviceToPortMap.put(m.getDeviceId(), m.getExternalPort());
            m.setOnline(false);
        }
        if (!portMappingMap.isEmpty()) {
            log.info("Loaded {} persisted port mappings from database", portMappingMap.size());
        }
    }

    /**
     * 为设备分配端口：若该设备已有持久化映射则复用固定端口，否则分配新端口并持久化。
     */
    public int allocatePort(String deviceId, int targetPort) {
        // 1. 已有持久化映射 -> 复用固定端口
        Integer existingPort = deviceToPortMap.get(deviceId);
        if (existingPort != null) {
            PortMapping mapping = portMappingMap.get(existingPort);
            if (mapping != null) {
                mapping.setOnline(true);
                mapping.setLastOnlineTime(LocalDateTime.now());
                if (mapping.getTargetPort() != targetPort) {
                    mapping.setTargetPort(targetPort);
                }
                repository.save(mapping);
                log.info("Port reused (fixed): deviceId={}, externalPort={}, targetPort={}",
                        deviceId, existingPort, targetPort);
                return existingPort;
            }
        }

        // 2. 无持久化映射 -> 分配新端口并持久化
        int port = allocateNextPort();
        if (port < 0) {
            log.error("No available port for device: {}", deviceId);
            return -1;
        }

        LocalDateTime now = LocalDateTime.now();
        PortMapping mapping = PortMapping.builder()
                .deviceId(deviceId)
                .externalPort(port)
                .targetPort(targetPort)
                .createTime(now)
                .lastOnlineTime(now)
                .online(true)
                .build();

        portMappingMap.put(port, mapping);
        deviceToPortMap.put(deviceId, port);
        repository.save(mapping);

        log.info("Port allocated and persisted: deviceId={}, externalPort={}, targetPort={}",
                deviceId, port, targetPort);
        return port;
    }

    /**
     * 获取端口对应的设备ID
     */
    public String getDeviceByPort(int port) {
        PortMapping mapping = portMappingMap.get(port);
        return mapping != null ? mapping.getDeviceId() : null;
    }

    /**
     * 获取设备对应的对外端口
     */
    public Integer getPortByDevice(String deviceId) {
        return deviceToPortMap.get(deviceId);
    }

    /**
     * 获取端口映射
     */
    public PortMapping getMapping(int port) {
        return portMappingMap.get(port);
    }

    /**
     * 标记设备离线：不删除持久化映射（保留固定端口），仅更新在线状态。
     * 端口监听的停止由调用方（AuthHandler）通过 DataPortService.stopDataPort 处理。
     */
    public void markOffline(String deviceId) {
        Integer port = deviceToPortMap.get(deviceId);
        if (port == null) {
            return;
        }
        PortMapping mapping = portMappingMap.get(port);
        if (mapping != null) {
            mapping.setOnline(false);
            mapping.setLastOnlineTime(LocalDateTime.now());
            repository.save(mapping);
            log.info("Device marked offline (port mapping retained): deviceId={}, port={}", deviceId, port);
        }
    }

    /**
     * 释放端口：从内存与 DB 同时删除映射（端口不再固定）。
     * 仅在管理员显式删除固定端口映射时调用。
     */
    public void releasePort(String deviceId) {
        Integer port = deviceToPortMap.remove(deviceId);
        if (port != null) {
            portMappingMap.remove(port);
            repository.deleteById(deviceId);
            log.info("Port mapping released: deviceId={}, port={}", deviceId, port);
        }
    }

    /**
     * 获取所有映射（含在线+离线）
     */
    public Map<Integer, PortMapping> getAllMappings() {
        return portMappingMap;
    }

    /**
     * 更新设备的对外端口（管理员操作），返回旧端口，设备不存在返回 -1
     */
    public int updateExternalPort(String deviceId, int newPort) {
        Integer oldPort = deviceToPortMap.get(deviceId);
        if (oldPort == null) {
            return -1;
        }
        if (oldPort == newPort) {
            return oldPort;
        }
        if (portMappingMap.containsKey(newPort)) {
            throw new IllegalArgumentException("Port " + newPort + " is already in use");
        }

        PortMapping old = portMappingMap.remove(oldPort);
        PortMapping updated = PortMapping.builder()
                .deviceId(old.getDeviceId())
                .externalPort(newPort)
                .targetPort(old.getTargetPort())
                .createTime(old.getCreateTime())
                .lastOnlineTime(LocalDateTime.now())
                .online(old.isOnline())
                .build();

        portMappingMap.put(newPort, updated);
        deviceToPortMap.put(deviceId, newPort);
        repository.save(updated);

        log.info("Port updated: deviceId={}, oldPort={}, newPort={}", deviceId, oldPort, newPort);
        return oldPort;
    }

    /**
     * 管理员预设固定端口：为设备指定固定端口（设备未上线也可预设）。
     * 返回 null 表示成功，否则返回失败原因。
     */
    public String setFixedPort(String deviceId, int externalPort, int targetPort) {
        int startPort = properties.getDataPortStart();
        int endPort = properties.getDataPortEnd();
        if (externalPort < startPort || externalPort > endPort) {
            return "端口必须在 " + startPort + "-" + endPort + " 范围内";
        }
        if (deviceToPortMap.containsKey(deviceId)) {
            return "设备 " + deviceId + " 已存在固定端口映射";
        }
        PortMapping occupied = portMappingMap.get(externalPort);
        if (occupied != null) {
            return "端口 " + externalPort + " 已被设备 " + occupied.getDeviceId() + " 占用";
        }

        LocalDateTime now = LocalDateTime.now();
        PortMapping mapping = PortMapping.builder()
                .deviceId(deviceId)
                .externalPort(externalPort)
                .targetPort(targetPort)
                .createTime(now)
                .lastOnlineTime(now)
                .online(false)
                .build();

        portMappingMap.put(externalPort, mapping);
        deviceToPortMap.put(deviceId, externalPort);
        repository.save(mapping);

        log.info("Fixed port preset: deviceId={}, externalPort={}, targetPort={}",
                deviceId, externalPort, targetPort);
        return null;
    }

    /**
     * 分配下一个可用端口（跳过已被持久化映射占用的端口）
     */
    private int allocateNextPort() {
        int startPort = properties.getDataPortStart();
        int endPort = properties.getDataPortEnd();

        for (int i = 0; i < endPort - startPort + 1; i++) {
            int port = nextPort.getAndIncrement();
            if (port > endPort) {
                port = startPort;
                nextPort.set(startPort + 1);
            }

            if (!portMappingMap.containsKey(port)) {
                return port;
            }
        }

        return -1; // 无可用端口
    }
}
