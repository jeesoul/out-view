package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.entity.PortMapping;
import com.outview.repository.PortMappingRepository;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import javax.annotation.PostConstruct;
import java.time.LocalDateTime;
import java.util.HashMap;
import java.util.Map;

/** 固定端口预留：数据库提交成功后再发布缓存，分配和管理操作使用同一把锁。 */
@Slf4j
@Service
public class PortMappingService {
    private final OutViewProperties properties;
    private final PortMappingRepository repository;
    private final Map<Integer, PortMapping> mappings = new HashMap<>();
    private final Map<String, Integer> devicePorts = new HashMap<>();
    // IO线程只读取已提交快照，不等待管理线程持锁进行的数据库写入。
    private volatile Snapshot snapshot = new Snapshot(mappings, devicePorts);

    private static final class Snapshot {
        final Map<Integer, PortMapping> mappings;
        final Map<String, Integer> devicePorts;
        Snapshot(Map<Integer, PortMapping> mappings, Map<String, Integer> devicePorts) {
            this.mappings = java.util.Collections.unmodifiableMap(new HashMap<>(mappings));
            this.devicePorts = java.util.Collections.unmodifiableMap(new HashMap<>(devicePorts));
        }
    }

    public PortMappingService(OutViewProperties properties, PortMappingRepository repository) {
        this.properties = properties;
        this.repository = repository;
    }

    @PostConstruct
    public synchronized void loadPersistedMappings() {
        if (getPortStart() < 1 || getPortEnd() > 65535 || getPortStart() > getPortEnd()) {
            throw new IllegalArgumentException("Invalid data port range");
        }
        mappings.clear();
        devicePorts.clear();
        snapshot = new Snapshot(mappings, devicePorts);
        for (PortMapping mapping : repository.findAll()) {
            if (mappings.containsKey(mapping.getExternalPort())) {
                throw new IllegalStateException("Duplicate persisted port: " + mapping.getExternalPort());
            }
            mapping.setOnline(false);
            publish(mapping);
        }
        log.info("Loaded {} fixed port reservations", mappings.size());
    }

    public synchronized int allocatePort(String deviceId, int targetPort) {
        validateTargetPort(targetPort);
        Integer existing = devicePorts.get(deviceId);
        if (existing != null) return existing;
        for (int port = getPortStart(); port <= getPortEnd(); port++) {
            if (!mappings.containsKey(port)) {
                PortMapping mapping = PortMapping.builder().deviceId(deviceId).externalPort(port)
                        .targetPort(targetPort).createTime(LocalDateTime.now()).online(false).build();
                save(mapping);
                return port;
            }
        }
        return -1;
    }

    public synchronized void markOnline(String deviceId, int targetPort) {
        validateTargetPort(targetPort);
        PortMapping mapping = byDevice(deviceId);
        if (mapping == null) throw new IllegalArgumentException("Device mapping not found");
        mapping.setOnline(true);
        mapping.setTargetPort(targetPort);
        mapping.setLastOnlineTime(LocalDateTime.now());
        save(mapping);
    }

    public synchronized void markOffline(String deviceId) {
        PortMapping mapping = byDevice(deviceId);
        if (mapping == null) return;
        // 在线状态是运行时状态；数据库暂时不可写也必须停止展示在线。
        mapping.setOnline(false);
        publish(mapping);
    }

    public synchronized int updateExternalPort(String deviceId, int newPort) {
        validateExternalPort(newPort);
        PortMapping mapping = byDevice(deviceId);
        if (mapping == null) return -1;
        int oldPort = mapping.getExternalPort();
        if (oldPort == newPort) return oldPort;
        String owner = getDeviceByPort(newPort);
        if (owner != null) throw new IllegalArgumentException("Port " + newPort + " is already reserved");
        mapping.setExternalPort(newPort);
        save(mapping);
        return oldPort;
    }

    public synchronized String setFixedPort(String deviceId, int externalPort, int targetPort) {
        try {
            validateExternalPort(externalPort);
            validateTargetPort(targetPort);
        } catch (IllegalArgumentException e) { return e.getMessage(); }
        if (devicePorts.containsKey(deviceId)) return "Device already has a fixed port mapping";
        if (mappings.containsKey(externalPort)) return "Port " + externalPort + " is already reserved";
        save(PortMapping.builder().deviceId(deviceId).externalPort(externalPort).targetPort(targetPort)
                .createTime(LocalDateTime.now()).online(false).build());
        return null;
    }

    public synchronized void releasePort(String deviceId) {
        Integer port = devicePorts.get(deviceId);
        if (port == null) return;
        repository.deleteById(deviceId);
        devicePorts.remove(deviceId);
        mappings.remove(port);
        snapshot = new Snapshot(mappings, devicePorts);
    }

    public void validateExternalPort(int port) {
        if (port < getPortStart() || port > getPortEnd()) {
            throw new IllegalArgumentException("Port must be within " + getPortStart() + "-" + getPortEnd());
        }
    }

    public void validateTargetPort(int port) {
        if (port < 1 || port > 65535) throw new IllegalArgumentException("Target port must be within 1-65535");
    }

    public int getPortStart() { return properties.getDataPortStart(); }
    public int getPortEnd() { return properties.getDataPortEnd(); }
    public Integer getPortByDevice(String deviceId) { return snapshot.devicePorts.get(deviceId); }
    public String getDeviceByPort(int port) {
        PortMapping mapping = snapshot.mappings.get(port);
        return mapping == null ? null : mapping.getDeviceId();
    }
    public PortMapping getMapping(int port) { return copy(snapshot.mappings.get(port)); }
    public Map<Integer, PortMapping> getAllMappings() {
        Map<Integer, PortMapping> result = new HashMap<>();
        snapshot.mappings.forEach((port, mapping) -> result.put(port, copy(mapping)));
        return result;
    }
    private PortMapping byDevice(String deviceId) {
        Integer port = devicePorts.get(deviceId);
        return port == null ? null : copy(mappings.get(port));
    }
    private void save(PortMapping mapping) {
        repository.saveAndFlush(mapping);
        publish(mapping);
    }
    private void publish(PortMapping mapping) {
        Integer oldPort = devicePorts.put(mapping.getDeviceId(), mapping.getExternalPort());
        if (oldPort != null && oldPort != mapping.getExternalPort()) mappings.remove(oldPort);
        mappings.put(mapping.getExternalPort(), copy(mapping));
        snapshot = new Snapshot(mappings, devicePorts);
    }
    private PortMapping copy(PortMapping m) {
        return m == null ? null : PortMapping.builder().deviceId(m.getDeviceId()).externalPort(m.getExternalPort())
                .targetPort(m.getTargetPort()).online(m.isOnline()).createTime(m.getCreateTime())
                .lastOnlineTime(m.getLastOnlineTime()).build();
    }
}
