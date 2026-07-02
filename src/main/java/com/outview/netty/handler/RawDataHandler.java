package com.outview.netty.handler;

import com.outview.entity.ClientSession;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.DataPortService;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.ByteBufUtil;
import io.netty.buffer.Unpooled;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.util.ReferenceCountUtil;
import lombok.extern.slf4j.Slf4j;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Component;

import java.net.InetSocketAddress;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 原始数据处理适配器
 * 处理来自外部用户（如 MSTSC）的原始 TCP 数据，实现双向数据转发
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class RawDataHandler extends ChannelInboundHandlerAdapter {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;
    private final DataPortService dataPortService;

    private final Map<Channel, ClientSession> userToSessionMap = new ConcurrentHashMap<>();
    private final Map<String, Channel> connectionIdToUserMap = new ConcurrentHashMap<>();
    private final Map<Channel, String> userToConnectionIdMap = new ConcurrentHashMap<>();

    public RawDataHandler(SessionStore sessionStore,
                         PortMappingService portMappingService,
                         @Lazy DataPortService dataPortService) {
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        this.dataPortService = dataPortService;
    }

    @Override
    public void channelActive(ChannelHandlerContext ctx) throws Exception {
        InetSocketAddress localAddress = (InetSocketAddress) ctx.channel().localAddress();
        int localPort = localAddress.getPort();

        String deviceId = portMappingService.getDeviceByPort(localPort);
        if (deviceId == null) {
            log.warn("No device found for port: {}, closing connection", localPort);
            ctx.close();
            return;
        }

        ClientSession session = sessionStore.getSession(deviceId);
        if (session == null || !session.isActive()) {
            log.warn("Client session not active: deviceId={}, closing connection", deviceId);
            ctx.close();
            return;
        }

        String connectionId = generateConnectionId(ctx.channel());
        userToSessionMap.put(ctx.channel(), session);
        connectionIdToUserMap.put(connectionId, ctx.channel());
        userToConnectionIdMap.put(ctx.channel(), connectionId);
        dataPortService.registerConnection(localPort, ctx.channel(), session.getChannel());

        log.info("[RawDataHandler] User connected: port={}, deviceId={}, connectionId={}",
                localPort, deviceId, connectionId);

        super.channelActive(ctx);
    }

    @Override
    public void channelRead(ChannelHandlerContext ctx, Object msg) throws Exception {
        if (!(msg instanceof ByteBuf)) {
            return;
        }

        ByteBuf buf = (ByteBuf) msg;
        try {
            ClientSession session = userToSessionMap.get(ctx.channel());
            if (session == null || !session.isActive()) {
                InetSocketAddress localAddress = (InetSocketAddress) ctx.channel().localAddress();
                if (localAddress != null) {
                    String deviceId = portMappingService.getDeviceByPort(localAddress.getPort());
                    if (deviceId != null) {
                        ClientSession fresh = sessionStore.getSession(deviceId);
                        if (fresh != null && fresh.isActive()) {
                            userToSessionMap.put(ctx.channel(), fresh);
                            session = fresh;
                        }
                    }
                }
            }
            if (session == null || !session.isActive()) {
                log.warn("No active session for channel, closing");
                ctx.close();
                return;
            }

            String connectionId = userToConnectionIdMap.get(ctx.channel());
            if (connectionId == null) {
                connectionId = generateConnectionId(ctx.channel());
                userToConnectionIdMap.put(ctx.channel(), connectionId);
                connectionIdToUserMap.put(connectionId, ctx.channel());
            }

            byte[] data = ByteBufUtil.getBytes(buf);

            ProtocolMessage proxyMsg = ProtocolMessage.dataWithConnectionId(connectionId, data);
            Channel clientChannel = session.getChannel();
            if (clientChannel == null || !clientChannel.isActive()) {
                log.warn("Target client channel inactive: deviceId={}", session.getDeviceId());
                return;
            }
            clientChannel.writeAndFlush(proxyMsg);

        } finally {
            ReferenceCountUtil.release(buf);
        }
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        ClientSession session = userToSessionMap.remove(ctx.channel());
        String connectionId = userToConnectionIdMap.remove(ctx.channel());
        if (connectionId != null) {
            connectionIdToUserMap.remove(connectionId);
        }

        if (session != null) {
            int localPort = session.getExternalPort();
            if (localPort <= 0) {
                InetSocketAddress localAddress = (InetSocketAddress) ctx.channel().localAddress();
                if (localAddress != null) {
                    localPort = localAddress.getPort();
                }
            }
            if (localPort > 0) {
                dataPortService.removeConnection(localPort, ctx.channel());
            }
            log.info("User disconnected: port={}, deviceId={}, connectionId={}",
                    localPort, session.getDeviceId(), connectionId);
        }

        super.channelInactive(ctx);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        if (cause instanceof java.io.IOException &&
            cause.getMessage() != null &&
            cause.getMessage().contains("Connection reset")) {
            log.warn("External connection reset: channel={}", ctx.channel().id().asShortText());
        } else {
            log.error("RawDataHandler exception: channel={}", ctx.channel().id().asShortText(), cause);
        }
        cleanupConnection(ctx.channel());
        ctx.close();
    }

    public boolean closeUserConnectionByConnectionId(String connectionId) {
        Channel userChannel = connectionIdToUserMap.get(connectionId);
        if (userChannel != null && userChannel.isActive()) {
            cleanupConnection(userChannel);
            userChannel.close();
            log.info("User channel closed by client request: connectionId={}", connectionId);
            return true;
        }
        connectionIdToUserMap.remove(connectionId);
        return false;
    }

    public ClientSession getSession(Channel userChannel) {
        return userToSessionMap.get(userChannel);
    }

    public Channel getUserChannelByConnectionId(String connectionId) {
        return connectionIdToUserMap.get(connectionId);
    }

    /**
     * 向用户连接发送数据（零拷贝：wrappedBuffer 避免额外内存拷贝）
     */
    public boolean sendToUser(String connectionId, byte[] data) {
        Channel userChannel = connectionIdToUserMap.get(connectionId);
        if (userChannel != null && userChannel.isActive()) {
            userChannel.writeAndFlush(Unpooled.wrappedBuffer(data));
            log.debug("Data sent to user: connectionId={}, length={}", connectionId, data.length);
            return true;
        }
        log.warn("User channel not found or inactive: connectionId={}", connectionId);
        return false;
    }

    private void cleanupConnection(Channel channel) {
        userToSessionMap.remove(channel);
        String connectionId = userToConnectionIdMap.remove(channel);
        if (connectionId != null) {
            connectionIdToUserMap.remove(connectionId);
        }
    }

    private String generateConnectionId(Channel channel) {
        return channel.id().asShortText();
    }
}
