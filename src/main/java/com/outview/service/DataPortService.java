package com.outview.service;

import com.outview.entity.ClientSession;
import com.outview.netty.DataChannelInitializer;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.Channel;
import io.netty.channel.ChannelFuture;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;

import javax.annotation.PreDestroy;
import java.net.InetSocketAddress;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 数据端口服务
 * 动态管理数据端口的启动和停止
 */
@Slf4j
@Service
public class DataPortService {

    private final DataChannelInitializer dataChannelInitializer;
    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;

    /**
     * 数据端口 EventLoopGroup (共享)
     */
    private EventLoopGroup dataBossGroup;
    private EventLoopGroup dataWorkerGroup;

    /**
     * 端口 -> 服务端 Channel 的映射
     */
    private final Map<Integer, Channel> dataPortChannels = new ConcurrentHashMap<>();

    /**
     * 端口 -> 外部连接 Channel 的映射 (用于数据转发)
     */
    private final Map<Integer, Map<Channel, Channel>> portConnectionMap = new ConcurrentHashMap<>();

    public DataPortService(DataChannelInitializer dataChannelInitializer,
                          SessionStore sessionStore,
                          PortMappingService portMappingService) {
        this.dataChannelInitializer = dataChannelInitializer;
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        initDataGroups();
    }

    /**
     * 初始化数据端口的 EventLoopGroup
     */
    private void initDataGroups() {
        dataBossGroup = new NioEventLoopGroup(1);
        dataWorkerGroup = new NioEventLoopGroup();
        log.info("Data port EventLoopGroups initialized");
    }

    /**
     * 为设备启动数据端口监听
     *
     * @param port         对外端口
     * @param deviceId     设备ID
     * @return 是否启动成功
     */
    public boolean startDataPort(int port, String deviceId) {
        // 检查端口是否已启动
        if (dataPortChannels.containsKey(port)) {
            log.info("Data port already started: port={}", port);
            return true;
        }

        try {
            ServerBootstrap bootstrap = new ServerBootstrap();
            bootstrap.group(dataBossGroup, dataWorkerGroup)
                    .channel(NioServerSocketChannel.class)
                    .option(ChannelOption.SO_BACKLOG, 128)
                    .childOption(ChannelOption.SO_KEEPALIVE, true)
                    .childOption(ChannelOption.TCP_NODELAY, true)
                    .childHandler(dataChannelInitializer);

            // 绑定端口
            ChannelFuture future = bootstrap.bind(new InetSocketAddress(port)).sync();
            Channel serverChannel = future.channel();
            dataPortChannels.put(port, serverChannel);
            portConnectionMap.put(port, new ConcurrentHashMap<>());

            log.info("Data port started: port={}, deviceId={}", port, deviceId);
            return true;

        } catch (Exception e) {
            log.error("Failed to start data port: port={}, deviceId={}", port, deviceId, e);
            return false;
        }
    }

    /**
     * 停止指定端口的数据服务
     *
     * @param port 端口号
     */
    public void stopDataPort(int port) {
        Channel serverChannel = dataPortChannels.remove(port);
        if (serverChannel != null) {
            serverChannel.close();
            log.info("Data port stopped: port={}", port);
        }

        // 关闭该端口上的所有连接
        Map<Channel, Channel> connections = portConnectionMap.remove(port);
        if (connections != null) {
            for (Channel channel : connections.keySet()) {
                if (channel.isActive()) {
                    channel.close();
                }
            }
        }
    }

    /**
     * 注册外部连接到指定端口
     *
     * @param port            数据端口
     * @param externalChannel 外部连接（用户）
     * @param clientChannel   客户端连接（设备端）
     */
    public void registerConnection(int port, Channel externalChannel, Channel clientChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            connections.put(externalChannel, clientChannel);
            log.debug("Connection registered: port={}, externalChannel={}", port, externalChannel.id().asShortText());
        }
    }

    /**
     * 移除外部连接
     *
     * @param port            数据端口
     * @param externalChannel 外部连接
     */
    public void removeConnection(int port, Channel externalChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            connections.remove(externalChannel);
            log.debug("Connection removed: port={}, externalChannel={}", port, externalChannel.id().asShortText());
        }
    }

    /**
     * 获取指定端口上的客户端连接
     *
     * @param port            数据端口
     * @param externalChannel 外部连接
     * @return 客户端连接
     */
    public Channel getClientChannel(int port, Channel externalChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            return connections.get(externalChannel);
        }
        return null;
    }

    /**
     * 检查端口是否已启动
     */
    public boolean isPortActive(int port) {
        Channel channel = dataPortChannels.get(port);
        return channel != null && channel.isActive();
    }

    /**
     * 获取活跃的数据端口数量
     */
    public int getActivePortCount() {
        return (int) dataPortChannels.values().stream()
                .filter(Channel::isActive)
                .count();
    }

    /**
     * 关闭所有数据端口
     */
    @PreDestroy
    public void shutdown() {
        log.info("Shutting down data ports...");

        // 关闭所有数据端口服务 Channel
        for (Map.Entry<Integer, Channel> entry : dataPortChannels.entrySet()) {
            try {
                entry.getValue().close();
                log.info("Data port closed: port={}", entry.getKey());
            } catch (Exception e) {
                log.error("Error closing data port: port={}", entry.getKey(), e);
            }
        }
        dataPortChannels.clear();
        portConnectionMap.clear();

        // 关闭 EventLoopGroup
        if (dataBossGroup != null) {
            dataBossGroup.shutdownGracefully();
        }
        if (dataWorkerGroup != null) {
            dataWorkerGroup.shutdownGracefully();
        }

        log.info("Data ports shutdown completed");
    }
}