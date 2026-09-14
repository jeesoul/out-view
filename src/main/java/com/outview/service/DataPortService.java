package com.outview.service;

import com.outview.netty.DataChannelInitializer;
import com.outview.config.OutViewProperties;
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
    private final OutViewProperties properties;

    /**
     * 独立的 boss group（每个数据端口只需 1 个 acceptor 线程）
     * worker group 复用 NettyServer 注入的实例，避免重复创建线程池
     */
    private final EventLoopGroup dataBossGroup;
    private final EventLoopGroup sharedWorkerGroup;

    /** 端口 -> 服务端 Channel */
    private final Map<Integer, Channel> dataPortChannels = new ConcurrentHashMap<>();
    private final Map<Integer, String> portOwners = new ConcurrentHashMap<>();

    /** 端口 -> (外部Channel -> 客户端Channel) */
    private final Map<Integer, Map<Channel, Channel>> portConnectionMap = new ConcurrentHashMap<>();

    public DataPortService(DataChannelInitializer dataChannelInitializer,
                          OutViewProperties properties,
                          EventLoopGroup sharedWorkerGroup) {
        this.dataChannelInitializer = dataChannelInitializer;
        this.properties = properties;
        this.dataBossGroup = new NioEventLoopGroup(1);
        this.sharedWorkerGroup = sharedWorkerGroup;
        log.info("DataPortService initialized, sharing workerGroup with NettyServer");
    }

    public synchronized boolean startDataPort(int port, String deviceId) {
        if (isPortActive(port)) {
            return deviceId.equals(portOwners.get(port));
        }

        try {
            ServerBootstrap bootstrap = new ServerBootstrap();
            bootstrap.group(dataBossGroup, sharedWorkerGroup)
                    .channel(NioServerSocketChannel.class)
                    .option(ChannelOption.SO_BACKLOG, 128)
                    .childOption(ChannelOption.SO_KEEPALIVE, true)
                    .childOption(ChannelOption.TCP_NODELAY, true)
                    .childHandler(dataChannelInitializer);

            ChannelFuture future = bootstrap.bind(new InetSocketAddress(properties.getBindAddress(), port)).sync();
            dataPortChannels.put(port, future.channel());
            portOwners.put(port, deviceId);
            portConnectionMap.put(port, new ConcurrentHashMap<>());

            log.info("Data port started: port={}, deviceId={}", port, deviceId);
            return true;

        } catch (Exception e) {
            if (e instanceof InterruptedException) Thread.currentThread().interrupt();
            log.error("Failed to start data port: port={}, deviceId={}", port, deviceId, e);
            return false;
        }
    }

    public synchronized void stopDataPort(int port) {
        Channel serverChannel = dataPortChannels.remove(port);
        if (serverChannel != null) {
            // 等待 acceptor 确认关闭，保证随后同端口重连不会撞到尚未释放的监听。
            serverChannel.close().syncUninterruptibly();
            log.info("Data port stopped: port={}", port);
        }

        closeConnections(port);
        portConnectionMap.remove(port);
        portOwners.remove(port);
    }

    public synchronized void stopDataPort(int port, String deviceId) {
        if (deviceId.equals(portOwners.get(port))) stopDataPort(port);
    }

    public synchronized void closeConnections(int port) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            connections.keySet().forEach(ch -> {
                if (ch.isActive()) ch.close();
            });
            connections.clear();
        }
    }

    public synchronized void registerConnection(int port, Channel externalChannel, Channel clientChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        // 旧 acceptor 已经接收但尚未初始化的连接，不得进入新一代监听。
        if (connections != null && externalChannel.parent() == dataPortChannels.get(port) && isPortActive(port)) {
            connections.put(externalChannel, clientChannel);
        } else {
            externalChannel.close();
        }
    }

    public void removeConnection(int port, Channel externalChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            connections.remove(externalChannel);
        }
    }

    public Channel getClientChannel(int port, Channel externalChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        return connections != null ? connections.get(externalChannel) : null;
    }

    public boolean isPortActive(int port) {
        Channel channel = dataPortChannels.get(port);
        return channel != null && channel.isActive();
    }

    public int getActivePortCount() {
        return (int) dataPortChannels.values().stream().filter(Channel::isActive).count();
    }

    @PreDestroy
    public synchronized void shutdown() {
        log.info("Shutting down data ports...");
        for (Integer port : new java.util.ArrayList<>(dataPortChannels.keySet())) stopDataPort(port);

        // 只关闭自己创建的 boss group；sharedWorkerGroup 由 NettyServer 管理
        if (dataBossGroup != null) {
            dataBossGroup.shutdownGracefully();
        }
        log.info("Data ports shutdown completed");
    }
}
