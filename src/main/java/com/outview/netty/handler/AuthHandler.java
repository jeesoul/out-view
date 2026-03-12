package com.outview.netty.handler;

import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.outview.entity.ClientSession;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.DataPortService;
import com.outview.service.PortMappingService;
import com.outview.service.SessionStore;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.Map;

/**
 * 鉴权处理器
 * 处理客户端注册请求
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class AuthHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

    private final SessionStore sessionStore;
    private final PortMappingService portMappingService;
    private final DataPortService dataPortService;

    public AuthHandler(SessionStore sessionStore,
                      PortMappingService portMappingService,
                      DataPortService dataPortService) {
        this.sessionStore = sessionStore;
        this.portMappingService = portMappingService;
        this.dataPortService = dataPortService;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
        // 只处理注册消息
        if (msg.getHeader().getType() != ProtocolConstants.TYPE_REGISTER) {
            // 传递给下一个处理器
            ctx.fireChannelRead(msg);
            return;
        }

        // 解析注册信息
        String body = new String(msg.getBody());
        // 敏感信息脱敏：不在日志中记录完整的 token
        log.info("Register request received from channel: {}", ctx.channel().id().asShortText());

        try {
            JSONObject json = JSON.parseObject(body);
            String deviceId = json.getString("deviceId");
            String token = json.getString("token");
            Integer localPort = json.getInteger("localPort");

            // 校验参数
            if (deviceId == null || token == null || localPort == null) {
                log.warn("Invalid register parameters from channel: {}", ctx.channel().id().asShortText());
                sendErrorResponse(ctx, "Invalid register parameters");
                return;
            }

            // TODO: 校验 Token 合法性
            // 这里简化处理，实际应该从数据库或配置中验证

            // 分配对外端口
            int externalPort = portMappingService.allocatePort(deviceId, localPort);
            if (externalPort < 0) {
                sendErrorResponse(ctx, "No available port");
                return;
            }

            // 启动数据端口监听
            boolean portStarted = dataPortService.startDataPort(externalPort, deviceId);
            if (!portStarted) {
                portMappingService.releasePort(deviceId);
                sendErrorResponse(ctx, "Failed to start data port");
                return;
            }

            // 注册会话
            sessionStore.register(deviceId, token, ctx.channel(), localPort, externalPort);

            // 发送注册成功响应
            sendRegisterAck(ctx, deviceId, externalPort);

            // 敏感信息脱敏：日志中不记录 token
            log.info("Client registered: deviceId={}, externalPort={}, localPort={}",
                    deviceId, externalPort, localPort);

        } catch (Exception e) {
            log.error("Register failed: {}", e.getMessage());
            sendErrorResponse(ctx, "Register failed: " + e.getMessage());
        }
    }

    /**
     * 发送注册响应
     */
    private void sendRegisterAck(ChannelHandlerContext ctx, String deviceId, int externalPort) {
        Map<String, Object> response = new HashMap<>();
        response.put("success", true);
        response.put("deviceId", deviceId);
        response.put("externalPort", externalPort);

        byte[] body = JSON.toJSONString(response).getBytes();
        ProtocolMessage ack = ProtocolMessage.builder()
                .header(com.outview.protocol.MessageHeader.builder()
                        .magic(ProtocolConstants.MAGIC_NUMBER)
                        .version(ProtocolConstants.VERSION)
                        .type(ProtocolConstants.TYPE_REGISTER_ACK)
                        .length(body.length)
                        .reserved((short) 0)
                        .build())
                .body(body)
                .build();

        ctx.writeAndFlush(ack);
    }

    /**
     * 发送错误响应
     */
    private void sendErrorResponse(ChannelHandlerContext ctx, String message) {
        ProtocolMessage error = ProtocolMessage.error(message);
        ctx.writeAndFlush(error);
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        // 连接断开，清理会话和端口映射
        ClientSession session = sessionStore.getSessionByChannel(ctx.channel());
        if (session != null) {
            int externalPort = session.getExternalPort();
            String deviceId = session.getDeviceId();

            // 停止数据端口
            dataPortService.stopDataPort(externalPort);

            // 释放端口映射
            portMappingService.releasePort(deviceId);

            // 移除会话
            sessionStore.removeSession(deviceId);

            log.info("Client disconnected: deviceId={}, externalPort={}", deviceId, externalPort);
        }
        super.channelInactive(ctx);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        log.error("AuthHandler exception: channel={}", ctx.channel().id().asShortText(), cause);
        ctx.close();
    }
}