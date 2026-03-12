package com.outview.protocol;

import com.outview.protocol.codec.MessageDecoder;
import com.outview.protocol.codec.MessageEncoder;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.Unpooled;
import io.netty.channel.embedded.EmbeddedChannel;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 协议编解码测试
 */
class ProtocolCodecTest {

    @Test
    void testEncodeAndDecode() {
        // 创建测试消息
        ProtocolMessage original = ProtocolMessage.register("test-device-001", "test-token", 3389);

        // 创建 EmbeddedChannel
        EmbeddedChannel channel = new EmbeddedChannel(
                new MessageEncoder(),
                new MessageDecoder()
        );

        // 编码
        assertTrue(channel.writeOutbound(original));
        ByteBuf encoded = channel.readOutbound();
        assertNotNull(encoded);

        // 解码
        assertTrue(channel.writeInbound(encoded));
        ProtocolMessage decoded = channel.readInbound();
        assertNotNull(decoded);

        // 验证
        assertEquals(ProtocolConstants.MAGIC_NUMBER, decoded.getHeader().getMagic());
        assertEquals(ProtocolConstants.VERSION, decoded.getHeader().getVersion());
        assertEquals(ProtocolConstants.TYPE_REGISTER, decoded.getHeader().getType());
        assertNotNull(decoded.getBody());

        channel.finish();
    }

    @Test
    void testHeartbeatMessage() {
        ProtocolMessage heartbeat = ProtocolMessage.heartbeat();
        assertNotNull(heartbeat);
        assertEquals(ProtocolConstants.TYPE_HEARTBEAT, heartbeat.getHeader().getType());
    }

    @Test
    void testErrorMessage() {
        ProtocolMessage error = ProtocolMessage.error("Test error");
        assertNotNull(error);
        assertEquals(ProtocolConstants.TYPE_ERROR, error.getHeader().getType());
        assertTrue(new String(error.getBody()).contains("Test error"));
    }

    @Test
    void testDataMessage() {
        byte[] data = "Hello, World!".getBytes();
        ProtocolMessage dataMsg = ProtocolMessage.data(data);
        assertNotNull(dataMsg);
        assertEquals(ProtocolConstants.TYPE_DATA, dataMsg.getHeader().getType());
        assertArrayEquals(data, dataMsg.getBody());
    }

    @Test
    void testInvalidMagicNumber() {
        EmbeddedChannel channel = new EmbeddedChannel(new MessageDecoder());

        // 创建无效的 Magic Number
        ByteBuf buf = Unpooled.buffer(16);
        buf.writeInt(0x12345678); // Invalid magic
        buf.writeByte(1);         // Version
        buf.writeByte(1);         // Type
        buf.writeInt(0);          // Length
        buf.writeShort(0);        // Reserved

        // 写入应该关闭通道
        channel.writeInbound(buf);
        assertFalse(channel.isActive());

        channel.finish();
    }
}