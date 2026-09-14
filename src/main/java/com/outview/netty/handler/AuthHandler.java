package com.outview.netty.handler;

import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.DeviceLifecycleService;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import java.nio.charset.StandardCharsets;

/** 注册和断线委托生命周期服务，在独立业务线程中执行数据库和端口绑定操作。 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class AuthHandler extends SimpleChannelInboundHandler<ProtocolMessage> {
    private final DeviceLifecycleService lifecycle;
    public AuthHandler(DeviceLifecycleService lifecycle) { this.lifecycle = lifecycle; }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
        if (msg.getHeader().getType() != ProtocolConstants.TYPE_REGISTER) {
            ctx.fireChannelRead(msg);
            return;
        }
        try {
            JSONObject json = JSON.parseObject(new String(msg.getBody(), StandardCharsets.UTF_8));
            Integer localPort = json.getInteger("localPort");
            if (localPort == null) throw new IllegalArgumentException("Invalid localPort");
            lifecycle.register(json.getString("deviceId"), json.getString("token"), ctx.channel(), localPort);
        } catch (Exception e) {
            log.warn("Register rejected: channel={}, reason={}", ctx.channel().id().asShortText(), e.getMessage());
            ctx.writeAndFlush(ProtocolMessage.error(e.getMessage() == null ? "Register failed" : e.getMessage()))
                    .addListener(io.netty.channel.ChannelFutureListener.CLOSE);
        }
    }

    @Override public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        lifecycle.disconnected(ctx.channel());
        super.channelInactive(ctx);
    }

    @Override public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        log.warn("Control connection failed: channel={}, reason={}", ctx.channel().id().asShortText(), cause.toString());
        ctx.close();
    }
}
