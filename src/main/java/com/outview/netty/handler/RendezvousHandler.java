package com.outview.netty.handler;

import com.alibaba.fastjson.JSON;
import com.alibaba.fastjson.JSONObject;
import com.outview.entity.ClientSession;
import com.outview.protocol.MessageHeader;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.service.SessionStore;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.Map;

/**
 * 设备码汇聚处理器
 * 控制端通过设备码查询被控端的连接信息（外部端口）
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class RendezvousHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

    private final SessionStore sessionStore;

    public RendezvousHandler(SessionStore sessionStore) {
        this.sessionStore = sessionStore;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
        if (msg.getHeader().getType() != ProtocolConstants.TYPE_DEVICE_QUERY) {
            ctx.fireChannelRead(msg);
            return;
        }

        String body = new String(msg.getBody());
        JSONObject json = JSON.parseObject(body);
        String deviceCode = json.getString("deviceCode");

        log.info("Device query: deviceCode={}", deviceCode);

        if (deviceCode == null || deviceCode.isEmpty()) {
            sendQueryAck(ctx, false, null, 0, "设备码不能为空");
            return;
        }

        ClientSession session = sessionStore.getSession(deviceCode);
        if (session == null || !session.isActive()) {
            log.info("Device not found or offline: deviceCode={}", deviceCode);
            sendQueryAck(ctx, false, deviceCode, 0, "设备不在线，请确认设备码是否正确");
            return;
        }

        log.info("Device found: deviceCode={}, externalPort={}", deviceCode, session.getExternalPort());
        sendQueryAck(ctx, true, deviceCode, session.getExternalPort(), null);
    }

    private void sendQueryAck(ChannelHandlerContext ctx, boolean found, String deviceCode,
                               int externalPort, String message) {
        Map<String, Object> resp = new HashMap<>();
        resp.put("found", found);
        if (deviceCode != null) resp.put("deviceCode", deviceCode);
        if (externalPort > 0) resp.put("externalPort", externalPort);
        if (message != null) resp.put("message", message);

        byte[] bodyBytes = JSON.toJSONString(resp).getBytes();
        ProtocolMessage ack = ProtocolMessage.builder()
                .header(MessageHeader.builder()
                        .magic(ProtocolConstants.MAGIC_NUMBER)
                        .version(ProtocolConstants.VERSION)
                        .type(ProtocolConstants.TYPE_DEVICE_QUERY_ACK)
                        .length(bodyBytes.length)
                        .reserved((short) 0)
                        .build())
                .body(bodyBytes)
                .build();

        ctx.writeAndFlush(ack);
    }
}
