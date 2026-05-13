package com.outview.netty;

import com.outview.config.OutViewProperties;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.CommandLineRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.stereotype.Component;

import javax.annotation.PreDestroy;
import java.net.InetSocketAddress;

/**
 * Netty 服务端
 * 负责启动控制端口，并将 workerGroup 暴露为 Bean 供 DataPortService 复用
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

    /**
     * 将 workerGroup 暴露为 Spring Bean，供 DataPortService 注入复用，
     * 避免为数据端口重复创建线程池。
     */
    @Bean
    public EventLoopGroup sharedWorkerGroup() {
        if (workerGroup == null) {
            workerGroup = new NioEventLoopGroup();
        }
        return workerGroup;
    }

    @Override
    public void run(String... args) throws Exception {
        startControlServer();
    }

    private void startControlServer() {
        bossGroup = new NioEventLoopGroup(1);
        if (workerGroup == null) {
            workerGroup = new NioEventLoopGroup();
        }

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
