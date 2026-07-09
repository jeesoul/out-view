package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.entity.ClientSession;
import io.netty.channel.Channel;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import java.time.LocalDateTime;
import java.util.Collection;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 会话存储服务
 * 管理所有客户端会话
 */
@Slf4j
@Service
public class SessionStore {

    private final OutViewProperties properties;

    /**
     * 设备ID -> 会话
     */
    private final Map<String, ClientSession> sessionMap = new ConcurrentHashMap<>();

    /**
     * Channel ID -> 设备ID
     */
    private final Map<String, String> channelToDeviceMap = new ConcurrentHashMap<>();

    public SessionStore(OutViewProperties properties) {
        this.properties = properties;
    }

    /**
     * 注册会话
     */
    public void register(String deviceId, String token, Channel channel, int localPort, int externalPort) {
        // 覆盖前清理同 deviceId 的旧 channel 映射，防止重连时旧映射残留导致误判
        ClientSession old = sessionMap.get(deviceId);
        if (old != null && old.getChannel() != null) {
            channelToDeviceMap.remove(old.getChannel().id().asLongText());
        }

        ClientSession session = ClientSession.builder()
                .deviceId(deviceId)
                .token(token)
                .channel(channel)
                .localPort(localPort)
                .externalPort(externalPort)
                .lastHeartbeat(LocalDateTime.now())
                .createTime(LocalDateTime.now())
                .status(ClientSession.SessionStatus.ONLINE)
                .build();

        sessionMap.put(deviceId, session);
        channelToDeviceMap.put(channel.id().asLongText(), deviceId);

        log.info("Session registered: deviceId={}, externalPort={}", deviceId, externalPort);
    }

    /**
     * 获取会话
     */
    public ClientSession getSession(String deviceId) {
        return sessionMap.get(deviceId);
    }

    /**
     * 根据 Channel 获取会话
     */
    public ClientSession getSessionByChannel(Channel channel) {
        String deviceId = channelToDeviceMap.get(channel.id().asLongText());
        if (deviceId == null) {
            return null;
        }
        ClientSession session = sessionMap.get(deviceId);
        // 严格匹配：仅当 session 的 channel 就是传入的 channel 时才返回。
        // 避免客户端断网快速重连时，旧连接心跳超时触发 channelInactive 误清新连接的 session。
        if (session != null && session.getChannel() == channel) {
            return session;
        }
        return null;
    }

    /**
     * 更新心跳
     */
    public void updateHeartbeat(String deviceId) {
        ClientSession session = sessionMap.get(deviceId);
        if (session != null) {
            session.updateHeartbeat();
            session.setStatus(ClientSession.SessionStatus.ONLINE);
            log.debug("Heartbeat updated: deviceId={}", deviceId);
        }
    }

    /**
     * 移除会话
     */
    public void removeSession(String deviceId) {
        ClientSession session = sessionMap.remove(deviceId);
        if (session != null && session.getChannel() != null) {
            channelToDeviceMap.remove(session.getChannel().id().asLongText());
            log.info("Session removed: deviceId={}", deviceId);
        }
    }

    /**
     * 根据 Channel 移除会话
     */
    public void removeSessionByChannel(Channel channel) {
        String deviceId = channelToDeviceMap.remove(channel.id().asLongText());
        if (deviceId != null) {
            sessionMap.remove(deviceId);
            log.info("Session removed by channel: deviceId={}", deviceId);
        }
    }

    /**
     * 获取所有在线会话
     */
    public Collection<ClientSession> getAllSessions() {
        return sessionMap.values();
    }

    /**
     * 获取在线会话数量
     */
    public int getOnlineCount() {
        return (int) sessionMap.values().stream()
                .filter(ClientSession::isActive)
                .count();
    }

    /**
     * 检查会话是否超时
     */
    public boolean isSessionTimeout(ClientSession session) {
        if (session == null || session.getLastHeartbeat() == null) {
            return true;
        }
        LocalDateTime timeoutThreshold = LocalDateTime.now().minusSeconds(properties.getHeartbeatTimeout());
        return session.getLastHeartbeat().isBefore(timeoutThreshold);
    }
}