package com.outview.protocol;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 消息头
 * 12 字节固定长度
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class MessageHeader {

    /**
     * Magic Number (4 bytes)
     */
    private int magic;

    /**
     * 协议版本 (1 byte)
     */
    private byte version;

    /**
     * 消息类型 (1 byte)
     */
    private byte type;

    /**
     * 消息体长度 (4 bytes)
     */
    private int length;

    /**
     * 保留字段 (2 bytes)
     */
    private short reserved;

    /**
     * 创建心跳请求头
     */
    public static MessageHeader heartbeat(int bodyLength) {
        return MessageHeader.builder()
                .magic(ProtocolConstants.MAGIC_NUMBER)
                .version(ProtocolConstants.VERSION)
                .type(ProtocolConstants.TYPE_HEARTBEAT)
                .length(bodyLength)
                .reserved((short) 0)
                .build();
    }

    /**
     * 创建数据转发头
     */
    public static MessageHeader data(int bodyLength) {
        return MessageHeader.builder()
                .magic(ProtocolConstants.MAGIC_NUMBER)
                .version(ProtocolConstants.VERSION)
                .type(ProtocolConstants.TYPE_DATA)
                .length(bodyLength)
                .reserved((short) 0)
                .build();
    }

    /**
     * 创建注册请求头
     */
    public static MessageHeader register(int bodyLength) {
        return MessageHeader.builder()
                .magic(ProtocolConstants.MAGIC_NUMBER)
                .version(ProtocolConstants.VERSION)
                .type(ProtocolConstants.TYPE_REGISTER)
                .length(bodyLength)
                .reserved((short) 0)
                .build();
    }

    /**
     * 创建错误消息头
     */
    public static MessageHeader error(int bodyLength) {
        return MessageHeader.builder()
                .magic(ProtocolConstants.MAGIC_NUMBER)
                .version(ProtocolConstants.VERSION)
                .type(ProtocolConstants.TYPE_ERROR)
                .length(bodyLength)
                .reserved((short) 0)
                .build();
    }
}