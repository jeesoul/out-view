package com.outview.protocol;

/**
 * 协议常量定义
 * 消息头结构: Magic(4) + Version(1) + Type(1) + Length(4) + Reserved(2) = 12 Bytes
 */
public final class ProtocolConstants {

    private ProtocolConstants() {}

    /**
     * Magic Number: "OVWS" (0x4F565753)
     */
    public static final int MAGIC_NUMBER = 0x4F565753;

    /**
     * 协议版本
     */
    public static final byte VERSION = 1;

    /**
     * 消息头长度
     */
    public static final int HEADER_LENGTH = 12;

    /**
     * 消息类型
     */
    public static final byte TYPE_REGISTER = 1;      // 注册请求
    public static final byte TYPE_REGISTER_ACK = 5;  // 注册响应
    public static final byte TYPE_HEARTBEAT = 2;     // 心跳请求
    public static final byte TYPE_HEARTBEAT_ACK = 6; // 心跳响应
    public static final byte TYPE_DATA = 3;          // 数据转发
    public static final byte TYPE_ERROR = 4;         // 错误消息

    /**
     * 最大消息体长度 (10MB)
     */
    public static final int MAX_BODY_LENGTH = 10 * 1024 * 1024;
}