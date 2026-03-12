package com.outview.protocol.codec;

import com.outview.protocol.MessageHeader;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.MessageToByteEncoder;
import lombok.extern.slf4j.Slf4j;

/**
 * 消息编码器
 * 将 ProtocolMessage 编码为 ByteBuf
 */
@Slf4j
public class MessageEncoder extends MessageToByteEncoder<ProtocolMessage> {

    @Override
    protected void encode(ChannelHandlerContext ctx, ProtocolMessage msg, ByteBuf out) throws Exception {
        if (msg == null || msg.getHeader() == null) {
            log.warn("Message or header is null");
            return;
        }

        MessageHeader header = msg.getHeader();
        byte[] body = msg.getBody();

        // 写入消息头 (12 bytes)
        out.writeInt(header.getMagic());          // 4 bytes
        out.writeByte(header.getVersion());       // 1 byte
        out.writeByte(header.getType());          // 1 byte
        out.writeInt(header.getLength());         // 4 bytes
        out.writeShort(header.getReserved());     // 2 bytes

        // 写入消息体
        if (body != null && body.length > 0) {
            out.writeBytes(body);
        }

        log.debug("Encoded message: type={}, length={}", header.getType(), header.getLength());
    }
}