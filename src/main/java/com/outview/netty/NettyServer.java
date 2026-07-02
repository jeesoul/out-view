package com.outview.netty;

import com.outview.config.OutViewProperties;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.CommandLineRunner;
import org.springframework.stereotype.Component;

import javax.annotation.PreDestroy;
import java.net.InetSocketAddress;

/**
 * Netty 服务端
 * 负责启动控制端口，workerGroup 由 NettyThreadPoolConfig 统一管理
 */
@Slf4j
@Component
public class NettyServer implements CommandLineRunner {

    private final OutViewProperties properties;
    private final ControlChannelInitializer controlChannelInitializer;
    private final EventLoopGroup sharedWorkerGroup;

    private EventLoopGroup bossGroup;

    public NettyServer(OutViewProperties properties,
                       ControlChannelInitializer controlChannelInitializer,
                       EventLoopGroup sharedWorkerGroup) {
        this.properties = properties;
        this.controlChannelInitializer = controlChannelInitializer;
        this.sharedWorkerGroup = sharedWorkerGroup;
    }

    @Override
    public void run(String... args) throws Exception {
        startControlServer();
    }

    private void startControlServer() {
        bossGroup = new NioEventLoopGroup(1);

        try {
            ServerBootstrap bootstrap = new ServerBootstrap();
            bootstrap.group(bossGroup, sharedWorkerGroup)
                    .channel(NioServerSocketChannel.class)
                    .option(ChannelOption.SO_BACKLOG, 128)
                    .childOption(ChannelOption.SO_KEEPALIVE, true)
                    .childOption(ChannelOption.TCP_NODELAY, true)
                    .childHandler(controlChannelInitializer);

            int port = properties.getControlPort();
            bootstrap.bind(new InetSocketAddress(port)).sync();
            log.info("OutView Control Server started on port: {}", port);

        } catch (Exception e) {
            log.error("Failed to start Netty server", e);
            shutdown();
        }
    }

    @PreDestroy
    public void shutdown() {
        if (bossGroup != null) {
            bossGroup.shutdownGracefully();
        }
        log.info("Netty server shutdown completed");
    }
}

