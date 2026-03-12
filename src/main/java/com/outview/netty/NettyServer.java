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
 * 负责启动控制端口和数据端口
 */
@Slf4j
@Component
public class NettyServer implements CommandLineRunner {

    private final OutViewProperties properties;
    private final ControlChannelInitializer controlChannelInitializer;

    private EventLoopGroup bossGroup;
    private EventLoopGroup workerGroup;

    public NettyServer(OutViewProperties properties, ControlChannelInitializer controlChannelInitializer) {
        this.properties = properties;
        this.controlChannelInitializer = controlChannelInitializer;
    }

    @Override
    public void run(String... args) throws Exception {
        startControlServer();
    }

    /**
     * 启动控制服务 (客户端注册/心跳)
     */
    private void startControlServer() {
        bossGroup = new NioEventLoopGroup(1);
        workerGroup = new NioEventLoopGroup();

        try {
            ServerBootstrap bootstrap = new ServerBootstrap();
            bootstrap.group(bossGroup, workerGroup)
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

    /**
     * 关闭服务
     */
    @PreDestroy
    public void shutdown() {
        if (bossGroup != null) {
            bossGroup.shutdownGracefully();
        }
        if (workerGroup != null) {
            workerGroup.shutdownGracefully();
        }
        log.info("Netty server shutdown completed");
    }
}