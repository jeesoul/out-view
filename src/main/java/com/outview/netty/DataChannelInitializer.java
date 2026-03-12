package com.outview.netty;

import com.outview.config.OutViewProperties;
import com.outview.netty.handler.RawDataHandler;
import com.outview.netty.ssl.SslContextFactory;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelPipeline;
import io.netty.channel.socket.SocketChannel;
import io.netty.handler.ssl.SslContext;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import javax.annotation.PostConstruct;

/**
 * 数据通道初始化器
 * 处理原始 TCP 数据转发（不使用协议编解码器）
 * 外部用户（如 MSTSC）直接发送原始 TCP 数据
 */
@Slf4j
@Component
public class DataChannelInitializer extends ChannelInitializer<SocketChannel> {

    private final RawDataHandler rawDataHandler;
    private final OutViewProperties properties;

    private SslContext sslContext;

    public DataChannelInitializer(RawDataHandler rawDataHandler, OutViewProperties properties) {
        this.rawDataHandler = rawDataHandler;
        this.properties = properties;
    }

    @PostConstruct
    public void init() {
        if (properties.getSsl().isEnabled()) {
            try {
                sslContext = SslContextFactory.createServerSslContext(properties.getSsl());
                log.info("SSL enabled for data channel");
            } catch (Exception e) {
                log.error("Failed to initialize SSL context for data channel: {}", e.getMessage(), e);
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
            log.debug("SSL handler added for data channel: {}", ch.id());
        }

        // 数据端口不需要协议编解码器，直接处理原始 TCP 数据
        // 外部用户（如 MSTSC）发送的是原始 TCP 数据流
        pipeline.addLast("rawData", rawDataHandler);
    }
}