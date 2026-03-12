package com.outview.protocol.codec;

import com.outview.protocol.MessageHeader;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import io.netty.buffer.ByteBuf;
import io.netty.channel.ChannelHandlerContext;
import io.netty.handler.codec.ByteToMessageDecoder;
import lombok.extern.slf4j.Slf4j;

import java.util.List;

/**
 * 消息解码器
 * 将 ByteBuf 解码为 ProtocolMessage
 */
@Slf4j
public class MessageDecoder extends ByteToMessageDecoder {

    private int consecutiveErrors = 0;
    private static final int MAX_CONSECUTIVE_ERRORS = 5;

    @Override
    protected void decode(ChannelHandlerContext ctx, ByteBuf in, List<Object> out) throws Exception {
        // 至少需要 HEADER_LENGTH 字节才能读取头部
        if (in.readableBytes() < ProtocolConstants.HEADER_LENGTH) {
            return;
        }

        // 标记读位置
        in.markReaderIndex();

        // 读取 Magic Number
        int magic = in.readInt();
        if (magic != ProtocolConstants.MAGIC_NUMBER) {
            consecutiveErrors++;
            log.warn("Invalid magic number: 0x{}, expected: 0x{} (consecutive errors: {})",
                    Integer.toHexString(magic).toUpperCase(),
                    Integer.toHexString(ProtocolConstants.MAGIC_NUMBER).toUpperCase(),
                    consecutiveErrors);

            // 尝试跳过一个字节重新同步
            in.resetReaderIndex();
            in.skipBytes(1);

            // 如果连续错误太多，关闭连接
            if (consecutiveErrors >= MAX_CONSECUTIVE_ERRORS) {
                log.error("Too many consecutive decode errors, closing connection");
                ctx.close();
            }
            return;
        }

        // 重置连续错误计数
        consecutiveErrors = 0;

        // 读取版本和类型
        byte version = in.readByte();
        byte type = in.readByte();

        // 读取消息体长度
        int length = in.readInt();

        // 读取保留字段
        short reserved = in.readShort();

        // 校验消息体长度
        if (length < 0 || length > ProtocolConstants.MAX_BODY_LENGTH) {
            log.error("Invalid body length: {}", length);
            ctx.close();
            return;
        }

        // 检查是否有足够的字节读取消息体
        if (in.readableBytes() < length) {
            // 重置读位置，等待更多数据
            in.resetReaderIndex();
            return;
        }

        // 读取消息体
        byte[] body = null;
        if (length > 0) {
            body = new byte[length];
            in.readBytes(body);
        }

        // 构建消息
        MessageHeader header = MessageHeader.builder()
                .magic(magic)
                .version(version)
                .type(type)
                .length(length)
                .reserved(reserved)
                .build();

        ProtocolMessage message = ProtocolMessage.builder()
                .header(header)
                .body(body)
                .build();

        out.add(message);

        log.debug("Decoded message: type={}, length={}", type, length);
    }
}