package com.outview.netty.handler;

import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.outview.entity.ClientSession;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.SessionStore;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.handler.timeout.IdleState;
import io.netty.handler.timeout.IdleStateEvent;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.Map;

/**
 * 心跳处理器
 * 处理心跳请求和超时检测
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class HeartbeatHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

    private final SessionStore sessionStore;

    public HeartbeatHandler(SessionStore sessionStore) {
        this.sessionStore = sessionStore;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
        // 只处理心跳消息
        if (msg.getHeader().getType() != ProtocolConstants.TYPE_HEARTBEAT) {
            ctx.fireChannelRead(msg);
            return;
        }

        // 更新心跳时间
        ClientSession session = sessionStore.getSessionByChannel(ctx.channel());
        if (session != null) {
            sessionStore.updateHeartbeat(session.getDeviceId());
            log.debug("Heartbeat received: deviceId={}", session.getDeviceId());
        }

        // 发送心跳响应
        sendHeartbeatAck(ctx);
    }

    /**
     * 发送心跳响应
     */
    private void sendHeartbeatAck(ChannelHandlerContext ctx) {
        Map<String, Object> response = new HashMap<>();
        response.put("success", true);
        response.put("timestamp", System.currentTimeMillis());

        byte[] body = JSON.toJSONString(response).getBytes();
        ProtocolMessage ack = ProtocolMessage.builder()
                .header(com.outview.protocol.MessageHeader.builder()
                        .magic(ProtocolConstants.MAGIC_NUMBER)
                        .version(ProtocolConstants.VERSION)
                        .type(ProtocolConstants.TYPE_HEARTBEAT_ACK)
                        .length(body.length)
                        .reserved((short) 0)
                        .build())
                .body(body)
                .build();

        ctx.writeAndFlush(ack);
    }

    @Override
    public void userEventTriggered(ChannelHandlerContext ctx, Object evt) throws Exception {
        if (evt instanceof IdleStateEvent) {
            IdleStateEvent event = (IdleStateEvent) evt;
            if (event.state() == IdleState.READER_IDLE) {
                // 读超时，关闭连接
                ClientSession session = sessionStore.getSessionByChannel(ctx.channel());
                if (session != null) {
                    log.warn("Client heartbeat timeout: deviceId={}", session.getDeviceId());
                }
                ctx.close();
            }
        }
        super.userEventTriggered(ctx, evt);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        log.error("HeartbeatHandler exception", cause);
        ctx.close();
    }
}