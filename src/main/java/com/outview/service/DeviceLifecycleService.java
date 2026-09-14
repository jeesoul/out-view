package com.outview.service;

import com.alibaba.fastjson.JSON;
import com.outview.entity.ClientSession;
import com.outview.protocol.MessageHeader;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import io.netty.channel.Channel;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Map;

/** 同设备生命周期串行执行；注册、断线及管理员操作始终使用一致的固定端口状态。 */
@Slf4j
@Service
public class DeviceLifecycleService {
    private final SessionStore sessions;
    private final PortMappingService mappings;
    private final DataPortService ports;
    private final BanService bans;
    private final Object[] locks = new Object[128];

    public DeviceLifecycleService(SessionStore sessions, PortMappingService mappings,
                                  DataPortService ports, BanService bans) {
        this.sessions = sessions;
        this.mappings = mappings;
        this.ports = ports;
        this.bans = bans;
        for (int i = 0; i < locks.length; i++) locks[i] = new Object();
    }

    private Object lock(String deviceId) { return locks[(deviceId.hashCode() & Integer.MAX_VALUE) % locks.length]; }

    public int register(String deviceId, String token, Channel channel, int localPort) {
        if (deviceId == null || deviceId.trim().isEmpty() || deviceId.length() > 64 || token == null || token.isEmpty()) {
            throw new IllegalArgumentException("Invalid register parameters");
        }
        mappings.validateTargetPort(localPort);
        synchronized (lock(deviceId)) {
            if (!channel.isActive()) throw new IllegalStateException("Control connection already closed");
            ClientSession registered = sessions.getSessionByChannel(channel);
            if (registered != null && !deviceId.equals(registered.getDeviceId())) {
                throw new IllegalArgumentException("Control connection already registered to another device");
            }
            if (bans.isBanned(deviceId)) throw new IllegalArgumentException("Device is banned. Contact administrator.");
            int port = mappings.allocatePort(deviceId, localPort);
            if (port < 0) throw new IllegalStateException("No available port");
            boolean previouslyActive = ports.isPortActive(port);
            if (!ports.startDataPort(port, deviceId)) throw new IllegalStateException("Failed to start fixed data port; reservation retained");
            try {
                mappings.markOnline(deviceId, localPort);
            } catch (RuntimeException e) {
                if (!previouslyActive) ports.stopDataPort(port, deviceId);
                throw e;
            }
            if (!channel.isActive()) {
                if (!previouslyActive) {
                    ports.stopDataPort(port, deviceId);
                    mappings.markOffline(deviceId);
                }
                throw new IllegalStateException("Control connection closed during registration");
            }
            ClientSession old = sessions.getSession(deviceId);
            if (old != null) {
                old.setStatus(ClientSession.SessionStatus.OFFLINE);
                ports.closeConnections(port);
            }
            sessions.register(deviceId, token, channel, localPort, port);
            sendRegisterAck(channel, deviceId, port);
            log.info("Client registered: deviceId={}, externalPort={}, localPort={}", deviceId, port, localPort);
            return port;
        }
    }

    public void disconnected(Channel channel) {
        ClientSession observed = sessions.getSessionByChannel(channel);
        if (observed == null) return;
        synchronized (lock(observed.getDeviceId())) {
            if (sessions.getSessionByChannel(channel) != observed) return;
            disconnectLocked(observed.getDeviceId(), "control channel closed");
        }
    }

    public void disconnect(String deviceId) {
        synchronized (lock(deviceId)) { disconnectLocked(deviceId, "administrator disconnect"); }
    }

    private void disconnectLocked(String deviceId, String reason) {
        ClientSession session = sessions.getSession(deviceId);
        if (session == null) return;
        // 先撤销会话所有权，旧连接的异步 channelInactive 无权清理后续重连。
        sessions.removeSessionByChannel(session.getChannel());
        ports.stopDataPort(session.getExternalPort(), deviceId);
        mappings.markOffline(deviceId);
        session.getChannel().close();
        log.info("Device offline: deviceId={}, fixedPort={}, reason={}", deviceId, session.getExternalPort(), reason);
    }

    public void ban(String deviceId, String operator, String reason) {
        synchronized (lock(deviceId)) {
            bans.ban(deviceId, operator, reason);
            disconnectLocked(deviceId, "administrator ban");
        }
    }

    public void unban(String deviceId) { synchronized (lock(deviceId)) { bans.unban(deviceId); } }

    public String preset(String deviceId, int externalPort, int targetPort) {
        synchronized (lock(deviceId)) { return mappings.setFixedPort(deviceId, externalPort, targetPort); }
    }

    public void deleteMapping(String deviceId) {
        synchronized (lock(deviceId)) {
            disconnectLocked(deviceId, "administrator removed reservation");
            mappings.releasePort(deviceId);
        }
    }

    public int updatePort(String deviceId, int newPort) {
        synchronized (lock(deviceId)) {
            mappings.validateExternalPort(newPort);
            Integer oldPort = mappings.getPortByDevice(deviceId);
            if (oldPort == null) return -1;
            if (oldPort == newPort) return oldPort;
            String owner = mappings.getDeviceByPort(newPort);
            if (owner != null) throw new IllegalArgumentException("Port " + newPort + " is already reserved");
            ClientSession session = sessions.getSession(deviceId);
            boolean online = session != null && session.isActive();
            if (online && !ports.startDataPort(newPort, deviceId)) {
                throw new IllegalArgumentException("Failed to bind new port; old port retained");
            }
            try {
                mappings.updateExternalPort(deviceId, newPort);
            } catch (RuntimeException e) {
                if (online) ports.stopDataPort(newPort, deviceId);
                throw e;
            }
            if (session != null) session.setExternalPort(newPort);
            ports.stopDataPort(oldPort, deviceId);
            if (online) sendRegisterAck(session.getChannel(), deviceId, newPort);
            return oldPort;
        }
    }

    private void sendRegisterAck(Channel channel, String deviceId, int port) {
        Map<String, Object> response = new HashMap<>();
        response.put("success", true);
        response.put("deviceId", deviceId);
        response.put("externalPort", port);
        byte[] body = JSON.toJSONString(response).getBytes(StandardCharsets.UTF_8);
        channel.writeAndFlush(ProtocolMessage.builder().header(MessageHeader.builder()
                .magic(ProtocolConstants.MAGIC_NUMBER).version(ProtocolConstants.VERSION)
                .type(ProtocolConstants.TYPE_REGISTER_ACK).length(body.length).reserved((short) 0).build())
                .body(body).build()).addListener(future -> { if (!future.isSuccess()) channel.close(); });
    }
}
