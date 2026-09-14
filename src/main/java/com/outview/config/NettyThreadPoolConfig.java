package com.outview.config;

import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.util.concurrent.DefaultEventExecutorGroup;
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

    /** 数据库与 bind 等阻塞操作单独执行，控制连接的心跳仍留在 IO 线程。 */
    @Bean(destroyMethod = "shutdownGracefully")
    public DefaultEventExecutorGroup controlBusinessExecutor() {
        return new DefaultEventExecutorGroup(4);
    }

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
