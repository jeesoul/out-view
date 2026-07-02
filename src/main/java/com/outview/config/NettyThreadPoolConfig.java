package com.outview.config;

import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import javax.annotation.PreDestroy;

/**
 * Netty 线程池配置
 * 独立管理 sharedWorkerGroup，打破 NettyServer ↔ DataPortService 循环依赖
 */
@Configuration
public class NettyThreadPoolConfig {

    private NioEventLoopGroup workerGroup;

    @Bean
    public EventLoopGroup sharedWorkerGroup() {
        workerGroup = new NioEventLoopGroup();
        return workerGroup;
    }

    @PreDestroy
    public void shutdown() {
        if (workerGroup != null) {
            workerGroup.shutdownGracefully();
        }
    }
}
