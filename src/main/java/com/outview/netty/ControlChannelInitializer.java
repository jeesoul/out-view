package com.outview.netty;

import com.outview.config.OutViewProperties;
import com.outview.netty.handler.AuthHandler;
import com.outview.netty.handler.HeartbeatHandler;
import com.outview.netty.handler.ProxyHandler;
import com.outview.netty.ssl.SslContextFactory;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelPipeline;
import io.netty.channel.socket.SocketChannel;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.timeout.IdleStateHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import javax.annotation.PostConstruct;
import java.util.concurrent.TimeUnit;

/**
 * 控制通道初始化器
 * 处理客户端注册和心跳
 */
@Slf4j
@Component
public class ControlChannelInitializer extends ChannelInitializer<SocketChannel> {

    private final AuthHandler authHandler;
    private final HeartbeatHandler heartbeatHandler;
    private final ProxyHandler proxyHandler;
    private final OutViewProperties properties;

    private SslContext sslContext;

    public ControlChannelInitializer(AuthHandler authHandler,
                                     HeartbeatHandler heartbeatHandler,
                                     ProxyHandler proxyHandler,
                                     OutViewProperties properties) {
        this.authHandler = authHandler;
        this.heartbeatHandler = heartbeatHandler;
        this.proxyHandler = proxyHandler;
        this.properties = properties;
    }

    @PostConstruct
    public void init() {
        if (properties.getSsl().isEnabled()) {
            try {
                sslContext = SslContextFactory.createServerSslContext(properties.getSsl());
                log.info("SSL enabled for control channel");
            } catch (Exception e) {
                log.error("Failed to initialize SSL context: {}", e.getMessage(), e);
                throw new RuntimeException("Failed to initialize SSL context", e);
            }
        }
    }

    @Override
    protected void initChannel(SocketChannel ch) throws Exception {
        ChannelPipeline pipeline = ch.pipeline();

        // SSL/TLS 处理器 (如果启用)
        if (sslContext != null) {
            pipeline.addLast("ssl", sslContext.newHandler(ch.alloc()));
            log.debug("SSL handler added for channel: {}", ch.id());
        }

        // 空闲检测 (读超时 90 秒)
        pipeline.addLast("idle", new IdleStateHandler(
                properties.getHeartbeatTimeout(), 0, 0, TimeUnit.SECONDS));

        // 编解码器
        pipeline.addLast("decoder", new com.outview.protocol.codec.MessageDecoder());
        pipeline.addLast("encoder", new com.outview.protocol.codec.MessageEncoder());

        // 业务处理器
        pipeline.addLast("auth", authHandler);
        pipeline.addLast("heartbeat", heartbeatHandler);
        pipeline.addLast("proxy", proxyHandler);
    }
}