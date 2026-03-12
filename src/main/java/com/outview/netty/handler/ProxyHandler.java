package com.outview.netty.handler;

import com.outview.entity.ClientSession;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

/**
 * 数据代理处理器
 * 处理来自客户端的数据转发消息
 * 将数据转发给对应的外部用户连接
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class ProxyHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;
    private final RawDataHandler rawDataHandler;

    public ProxyHandler(SessionStore sessionStore,
                       PortMappingService portMappingService,
                       RawDataHandler rawDataHandler) {
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        this.rawDataHandler = rawDataHandler;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
        byte type = msg.getHeader().getType();

        switch (type) {
            case ProtocolConstants.TYPE_DATA:
                handleDataMessage(ctx, msg);
                break;
            case ProtocolConstants.TYPE_REGISTER:
            case ProtocolConstants.TYPE_HEARTBEAT:
                // 这些消息由其他处理器处理，直接传递
                ctx.fireChannelRead(msg);
                break;
            default:
                log.warn("Unknown message type in proxy handler: {}", type);
                break;
        }
    }

    /**
     * 处理数据消息
     * 从客户端收到数据后转发给对应的外部用户
     */
    private void handleDataMessage(ChannelHandlerContext ctx, ProtocolMessage msg) {
        // 解析数据包（支持带连接ID的格式）
        ProtocolMessage.DataPacket dataPacket = msg.parseDataPacket();
        if (dataPacket == null) {
            log.warn("Failed to parse data packet");
            return;
        }

        String connectionId = dataPacket.getConnectionId();
        byte[] data = dataPacket.getData();

        log.info("[ProxyHandler] Received data: connectionId={}, length={}", connectionId, data.length);

        if (connectionId != null) {
            // 带连接ID的数据，直接路由到对应用户
            boolean sent = rawDataHandler.sendToUser(connectionId, data);
            log.info("[ProxyHandler] Forwarded to user: connectionId={}, length={}, sent={}",
                    connectionId, data.length, sent);
        } else {
            // 无连接ID的数据（旧格式兼容）
            ClientSession session = sessionStore.getSessionByChannel(ctx.channel());
            if (session != null) {
                log.debug("Received data without connectionId from device: deviceId={}, length={}",
                        session.getDeviceId(), data.length);
            }
        }
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        ClientSession session = sessionStore.getSessionByChannel(ctx.channel());
        if (session != null) {
            log.info("Client connection closed: deviceId={}", session.getDeviceId());
        }

        super.channelInactive(ctx);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        log.error("ProxyHandler exception: channel={}", ctx.channel().id().asShortText(), cause);
        ctx.close();
    }
}