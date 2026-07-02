package com.outview.service;

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
     * 独立的 boss group（每个数据端口只需 1 个 acceptor 线程）
     * worker group 复用 NettyServer 注入的实例，避免重复创建线程池
     */
    private final EventLoopGroup dataBossGroup;
    private final EventLoopGroup sharedWorkerGroup;

    /** 端口 -> 服务端 Channel */
    private final Map<Integer, Channel> dataPortChannels = new ConcurrentHashMap<>();

    /** 端口 -> (外部Channel -> 客户端Channel) */
    private final Map<Integer, Map<Channel, Channel>> portConnectionMap = new ConcurrentHashMap<>();

    public DataPortService(DataChannelInitializer dataChannelInitializer,
                          SessionStore sessionStore,
                          PortMappingService portMappingService,
                          EventLoopGroup sharedWorkerGroup) {
        this.dataChannelInitializer = dataChannelInitializer;
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        this.dataBossGroup = new NioEventLoopGroup(1);
        this.sharedWorkerGroup = sharedWorkerGroup;
        log.info("DataPortService initialized, sharing workerGroup with NettyServer");
    }

    public boolean startDataPort(int port, String deviceId) {
        if (dataPortChannels.containsKey(port)) {
            log.info("Data port already started: port={}", port);
            return true;
        }

        try {
            ServerBootstrap bootstrap = new ServerBootstrap();
            bootstrap.group(dataBossGroup, sharedWorkerGroup)
                    .channel(NioServerSocketChannel.class)
                    .option(ChannelOption.SO_BACKLOG, 128)
                    .childOption(ChannelOption.SO_KEEPALIVE, true)
                    .childOption(ChannelOption.TCP_NODELAY, true)
                    .childHandler(dataChannelInitializer);

            ChannelFuture future = bootstrap.bind(new InetSocketAddress(port)).sync();
            dataPortChannels.put(port, future.channel());
            portConnectionMap.put(port, new ConcurrentHashMap<>());

            log.info("Data port started: port={}, deviceId={}", port, deviceId);
            return true;

        } catch (Exception e) {
            log.error("Failed to start data port: port={}, deviceId={}", port, deviceId, e);
            return false;
        }
    }

    public void stopDataPort(int port) {
        Channel serverChannel = dataPortChannels.remove(port);
        if (serverChannel != null) {
            serverChannel.close();
            log.info("Data port stopped: port={}", port);
        }

        Map<Channel, Channel> connections = portConnectionMap.remove(port);
        if (connections != null) {
            connections.keySet().forEach(ch -> {
                if (ch.isActive()) ch.close();
            });
        }
    }

    public void registerConnection(int port, Channel externalChannel, Channel clientChannel) {
        Map<Channel, Channel> connections = portConnectionMap.get(port);
        if (connections != null) {
            connections.put(externalChannel, clientChannel);
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
    public void shutdown() {
        log.info("Shutting down data ports...");
        dataPortChannels.forEach((port, ch) -> {
            try {
                ch.close();
                log.info("Data port closed: port={}", port);
            } catch (Exception e) {
                log.error("Error closing data port: port={}", port, e);
            }
        });
        dataPortChannels.clear();
        portConnectionMap.clear();

        // 只关闭自己创建的 boss group；sharedWorkerGroup 由 NettyServer 管理
        if (dataBossGroup != null) {
            dataBossGroup.shutdownGracefully();
        }
        log.info("Data ports shutdown completed");
    }
}
