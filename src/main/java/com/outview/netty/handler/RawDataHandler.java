package com.outview.netty.handler;

import com.outview.entity.ClientSession;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.DataPortService;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.ByteBufUtil;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelInboundHandlerAdapter;
import lombok.extern.slf4j.Slf4j;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Component;

import java.net.InetSocketAddress;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 原始数据处理适配器
 * 处理来自外部用户（如 MSTSC）的原始 TCP 数据
 * 并实现双向数据转发
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class RawDataHandler extends ChannelInboundHandlerAdapter {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;
    private final DataPortService dataPortService;

    /**
     * 外部用户连接 -> 对应的客户端会话
     */
    private final Map<Channel, ClientSession> userToSessionMap = new ConcurrentHashMap<>();

    /**
     * 连接ID -> 用户Channel映射
     * 用于从客户端收到数据后路由到对应用户
     */
    private final Map<String, Channel> connectionIdToUserMap = new ConcurrentHashMap<>();

    /**
     * 用户Channel -> 连接ID映射
     */
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
        // 外部用户连接建立
        InetSocketAddress localAddress = (InetSocketAddress) ctx.channel().localAddress();
        int localPort = localAddress.getPort();

        // 根据端口找到对应的设备
        String deviceId = portMappingService.getDeviceByPort(localPort);
        if (deviceId == null) {
            log.warn("No device found for port: {}, closing connection", localPort);
            ctx.close();
            return;
        }

        // 获取客户端会话
        ClientSession session = sessionStore.getSession(deviceId);
        if (session == null || !session.isActive()) {
            log.warn("Client session not active: deviceId={}, closing connection", deviceId);
            ctx.close();
            return;
        }

        // 生成连接ID
        String connectionId = generateConnectionId(ctx.channel());

        // 注册连接映射
        userToSessionMap.put(ctx.channel(), session);
        connectionIdToUserMap.put(connectionId, ctx.channel());
        userToConnectionIdMap.put(ctx.channel(), connectionId);
        dataPortService.registerConnection(localPort, ctx.channel(), session.getChannel());

        log.info("[RawDataHandler] User connected: port={}, deviceId={}, connectionId={}, userChannel={}",
                localPort, deviceId, connectionId, ctx.channel().id().asShortText());

        super.channelActive(ctx);
    }

    @Override
    public void channelRead(ChannelHandlerContext ctx, Object msg) throws Exception {
        if (!(msg instanceof ByteBuf)) {
            return;
        }

        ByteBuf buf = (ByteBuf) msg;
        try {
            // 获取对应的客户端会话
            ClientSession session = userToSessionMap.get(ctx.channel());
            if (session == null || !session.isActive()) {
                log.warn("No active session for channel, closing");
                ctx.close();
                return;
            }

            // 获取连接ID
            String connectionId = userToConnectionIdMap.get(ctx.channel());
            if (connectionId == null) {
                connectionId = generateConnectionId(ctx.channel());
                userToConnectionIdMap.put(ctx.channel(), connectionId);
                connectionIdToUserMap.put(connectionId, ctx.channel());
            }

            // 读取数据
            byte[] data = ByteBufUtil.getBytes(buf);

            // 构建带连接ID的数据转发消息并发送给客户端
            ProtocolMessage proxyMsg = ProtocolMessage.dataWithConnectionId(connectionId, data);
            session.getChannel().writeAndFlush(proxyMsg);

            log.debug("Data forwarded to device: deviceId={}, connectionId={}, length={}",
                    session.getDeviceId(), connectionId, data.length);

        } finally {
            // ReferenceCountUtil.release(buf); // 由 Netty 自动释放
        }
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        // 清理连接映射
        ClientSession session = userToSessionMap.remove(ctx.channel());
        String connectionId = userToConnectionIdMap.remove(ctx.channel());
        if (connectionId != null) {
            connectionIdToUserMap.remove(connectionId);
        }

        if (session != null) {
            InetSocketAddress localAddress = (InetSocketAddress) ctx.channel().localAddress();
            int localPort = localAddress.getPort();
            dataPortService.removeConnection(localPort, ctx.channel());

            log.info("User disconnected: port={}, deviceId={}, connectionId={}",
                    localPort, session.getDeviceId(), connectionId);
        }

        super.channelInactive(ctx);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        log.error("RawDataHandler exception: channel={}", ctx.channel().id().asShortText(), cause);
        cleanupConnection(ctx.channel());
        ctx.close();
    }

    /**
     * 获取用户连接对应的客户端会话
     */
    public ClientSession getSession(Channel userChannel) {
        return userToSessionMap.get(userChannel);
    }

    /**
     * 根据连接ID获取用户Channel
     */
    public Channel getUserChannelByConnectionId(String connectionId) {
        return connectionIdToUserMap.get(connectionId);
    }

    /**
     * 向用户连接发送数据
     * 用于从客户端收到数据后转发给用户
     *
     * @param connectionId 连接ID
     * @param data         数据
     * @return 是否成功发送
     */
    public boolean sendToUser(String connectionId, byte[] data) {
        Channel userChannel = connectionIdToUserMap.get(connectionId);
        if (userChannel != null && userChannel.isActive()) {
            userChannel.writeAndFlush(userChannel.alloc().buffer().writeBytes(data));
            log.debug("Data sent to user: connectionId={}, length={}", connectionId, data.length);
            return true;
        } else {
            log.warn("User channel not found or inactive: connectionId={}", connectionId);
            return false;
        }
    }

    /**
     * 清理连接相关资源
     */
    private void cleanupConnection(Channel channel) {
        userToSessionMap.remove(channel);
        String connectionId = userToConnectionIdMap.remove(channel);
        if (connectionId != null) {
            connectionIdToUserMap.remove(connectionId);
        }
    }

    /**
     * 生成连接ID
     */
    private String generateConnectionId(Channel channel) {
        return channel.id().asShortText();
    }
}