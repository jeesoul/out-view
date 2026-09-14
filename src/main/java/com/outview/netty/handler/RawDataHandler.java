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

    private static final long MAX_PENDING_BYTES = 4L * 1024 * 1024;
    private static final io.netty.util.AttributeKey<java.util.concurrent.atomic.AtomicLong> PENDING_BYTES =
            io.netty.util.AttributeKey.valueOf("outview.pendingExternalBytes");
    private static final io.netty.util.AttributeKey<Boolean> CLEANUP_STARTED =
            io.netty.util.AttributeKey.valueOf("outview.externalCleanupStarted");

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

        // 初始化期间可能发生替换注册；已经属于旧会话的连接必须立即关闭。
        if (!session.isActive() || sessionStore.getSession(deviceId) != session) {
            cleanupConnection(ctx.channel(), true);
            ctx.close();
            return;
        }

        log.info("[RawDataHandler] User connected: port={}, deviceId={}, connectionId={}",
                localPort, deviceId, connectionId);

        super.channelActive(ctx);
    }

    @Override
    public void channelRead(ChannelHandlerContext ctx, Object msg) throws Exception {
        if (!(msg instanceof ByteBuf)) {
            ReferenceCountUtil.release(msg);
            return;
        }

        ByteBuf buf = (ByteBuf) msg;
        try {
            ClientSession session = userToSessionMap.get(ctx.channel());
            if (session == null || !session.isActive()) {
                log.warn("No active session for channel, closing");
                ctx.close();
                return;
            }

            String connectionId = userToConnectionIdMap.get(ctx.channel());
            if (connectionId == null) {
                // 注册时已创建 ID；缺失表示并发清理已开始，不能重新生成索引。
                ctx.close();
                return;
            }

            byte[] data = ByteBufUtil.getBytes(buf);

            ProtocolMessage proxyMsg = ProtocolMessage.dataWithConnectionId(connectionId, data);
            Channel clientChannel = session.getChannel();
            if (clientChannel == null || !clientChannel.isActive()) {
                log.warn("Target client channel inactive: deviceId={}", session.getDeviceId());
                return;
            }
            clientChannel.writeAndFlush(proxyMsg).addListener(future -> {
                if (!future.isSuccess()) ctx.close();
                resumeExternalReads(clientChannel);
            });
            if (!clientChannel.isWritable()) ctx.channel().config().setAutoRead(false);

        } finally {
            ReferenceCountUtil.release(buf);
        }
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        cleanupConnection(ctx.channel(), true);
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
        cleanupConnection(ctx.channel(), true);
        ctx.close();
    }

    public boolean closeUserConnectionByConnectionId(String connectionId) {
        Channel userChannel = connectionIdToUserMap.get(connectionId);
        if (userChannel != null && userChannel.isActive()) {
            cleanupConnection(userChannel, false);
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
            java.util.concurrent.atomic.AtomicLong pending = userChannel.attr(PENDING_BYTES).get();
            if (pending == null) {
                java.util.concurrent.atomic.AtomicLong created = new java.util.concurrent.atomic.AtomicLong();
                java.util.concurrent.atomic.AtomicLong existing = userChannel.attr(PENDING_BYTES).setIfAbsent(created);
                pending = existing == null ? created : existing;
            }
            final java.util.concurrent.atomic.AtomicLong counter = pending;
            if (counter.addAndGet(data.length) > MAX_PENDING_BYTES) {
                counter.addAndGet(-data.length);
                log.warn("Slow external consumer closed: connectionId={}", connectionId);
                cleanupConnection(userChannel, true);
                userChannel.close();
                return false;
            }
            userChannel.writeAndFlush(Unpooled.wrappedBuffer(data)).addListener(future -> {
                counter.addAndGet(-data.length);
                if (!future.isSuccess()) {
                    cleanupConnection(userChannel, true);
                    userChannel.close();
                }
            });
            log.debug("Data sent to user: connectionId={}, length={}", connectionId, data.length);
            return true;
        }
        log.warn("User channel not found or inactive: connectionId={}", connectionId);
        return false;
    }

    public boolean ownsConnection(String connectionId, Channel controlChannel) {
        Channel user = connectionIdToUserMap.get(connectionId);
        ClientSession owner = user == null ? null : userToSessionMap.get(user);
        return owner != null && owner.isActive() && owner.getChannel() == controlChannel
                && sessionStore.getSessionByChannel(controlChannel) == owner;
    }

    // 只暂停数据入口，控制连接继续接收心跳和关闭通知。
    public void resumeExternalReads(Channel controlChannel) {
        if (!controlChannel.isActive() || !controlChannel.isWritable()) return;
        userToSessionMap.forEach((external, session) -> {
            if (session.getChannel() == controlChannel && external.isActive()) {
                external.eventLoop().execute(() -> {
                    if (external.isActive() && controlChannel.isWritable()) external.config().setAutoRead(true);
                });
            }
        });
    }

    private void cleanupConnection(Channel channel, boolean notifyClient) {
        // 外部断线、写失败和拥塞可能来自不同线程。只有胜者能取走全部索引及通知归属。
        if (channel.attr(CLEANUP_STARTED).setIfAbsent(Boolean.TRUE) != null) return;
        ClientSession session = userToSessionMap.remove(channel);
        String connectionId = userToConnectionIdMap.remove(channel);
        if (connectionId != null) {
            connectionIdToUserMap.remove(connectionId);
        }
        // 必须使用原始监听端口；管理员可能已经更新 session.externalPort。
        if (channel.localAddress() instanceof InetSocketAddress) {
            dataPortService.removeConnection(((InetSocketAddress) channel.localAddress()).getPort(), channel);
        }
        if (notifyClient && connectionId != null && session != null && session.getChannel().isActive()) {
            ProtocolMessage close = ProtocolMessage.dataWithConnectionId(connectionId, new byte[0]);
            close.getHeader().setType(com.outview.protocol.ProtocolConstants.TYPE_CLOSE_CONNECTION);
            session.getChannel().writeAndFlush(close);
        }
    }

    private String generateConnectionId(Channel channel) {
        return channel.id().asLongText();
    }
}
