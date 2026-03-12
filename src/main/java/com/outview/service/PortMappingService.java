package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.entity.PortMapping;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * 端口映射服务
 * 管理对外端口与设备的映射关系
 */
@Slf4j
@Service
public class PortMappingService {

    private final OutViewProperties properties;

    /**
     * 端口 -> 映射
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

    public PortMappingService(OutViewProperties properties) {
        this.properties = properties;
        this.nextPort = new AtomicInteger(properties.getDataPortStart());
    }

    /**
     * 为设备分配端口
     */
    public int allocatePort(String deviceId, int targetPort) {
        // 检查是否已分配
        Integer existingPort = deviceToPortMap.get(deviceId);
        if (existingPort != null) {
            return existingPort;
        }

        // 分配新端口
        int port = allocateNextPort();
        if (port < 0) {
            log.error("No available port for device: {}", deviceId);
            return -1;
        }

        // 创建映射
        PortMapping mapping = PortMapping.builder()
                .externalPort(port)
                .deviceId(deviceId)
                .targetPort(targetPort)
                .createTime(LocalDateTime.now())
                .build();

        portMappingMap.put(port, mapping);
        deviceToPortMap.put(deviceId, port);

        log.info("Port allocated: deviceId={}, externalPort={}, targetPort={}", deviceId, port, targetPort);
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
     * 释放端口
     */
    public void releasePort(String deviceId) {
        Integer port = deviceToPortMap.remove(deviceId);
        if (port != null) {
            portMappingMap.remove(port);
            log.info("Port released: deviceId={}, port={}", deviceId, port);
        }
    }

    /**
     * 获取所有映射
     */
    public Map<Integer, PortMapping> getAllMappings() {
        return portMappingMap;
    }

    /**
     * 分配下一个可用端口
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