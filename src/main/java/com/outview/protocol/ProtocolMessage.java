package com.outview.protocol;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.nio.charset.StandardCharsets;

/**
 * 协议消息
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ProtocolMessage {

    private MessageHeader header;
    private byte[] body;

    public static ProtocolMessage heartbeat() {
        String jsonBody = "{\"timestamp\":" + System.currentTimeMillis() + "}";
        byte[] body = jsonBody.getBytes(StandardCharsets.UTF_8);
        return ProtocolMessage.builder()
                .header(MessageHeader.heartbeat(body.length))
                .body(body)
                .build();
    }

    public static ProtocolMessage data(byte[] payload) {
        return ProtocolMessage.builder()
                .header(MessageHeader.data(payload.length))
                .body(payload)
                .build();
    }

    /**
     * 创建带连接ID的数据转发消息（二进制格式）。
     *
     * Body 格式: [2B big-endian: connectionId长度][connectionId bytes][payload bytes]
     */
    public static ProtocolMessage dataWithConnectionId(String connectionId, byte[] payload) {
        byte[] idBytes = connectionId.getBytes(StandardCharsets.UTF_8);
        byte[] body = new byte[2 + idBytes.length + payload.length];
        body[0] = (byte) ((idBytes.length >> 8) & 0xFF);
        body[1] = (byte) (idBytes.length & 0xFF);
        System.arraycopy(idBytes, 0, body, 2, idBytes.length);
        System.arraycopy(payload, 0, body, 2 + idBytes.length, payload.length);
        return ProtocolMessage.builder()
                .header(MessageHeader.data(body.length))
                .body(body)
                .build();
    }

    /**
     * 解析带连接ID的数据消息（二进制格式）。
     *
     * Body 格式: [2B big-endian: connectionId长度][connectionId bytes][payload bytes]
     */
    public DataPacket parseDataPacket() {
        if (header.getType() != ProtocolConstants.TYPE_DATA) {
            return null;
        }
        if (body == null || body.length < 2) {
            return null;
        }
        int idLen = ((body[0] & 0xFF) << 8) | (body[1] & 0xFF);
        if (2 + idLen > body.length) {
            return null;
        }
        String connectionId = new String(body, 2, idLen, StandardCharsets.UTF_8);
        byte[] data = new byte[body.length - 2 - idLen];
        System.arraycopy(body, 2 + idLen, data, 0, data.length);
        return new DataPacket(connectionId, data);
    }

    /**
     * 解析连接关闭通知（body 格式同 dataWithConnectionId，但无 payload）。
     */
    public String parseCloseConnectionId() {
        if (body == null || body.length < 2) {
            return null;
        }
        int idLen = ((body[0] & 0xFF) << 8) | (body[1] & 0xFF);
        if (2 + idLen > body.length) {
            return null;
        }
        return new String(body, 2, idLen, StandardCharsets.UTF_8);
    }

    @Data
    @AllArgsConstructor
    public static class DataPacket {
        private String connectionId;
        private byte[] data;
    }

    public static ProtocolMessage register(String deviceId, String token, int localPort) {
        String jsonBody = String.format(
                "{\"deviceId\":\"%s\",\"token\":\"%s\",\"localPort\":%d}",
                deviceId, token, localPort
        );
        byte[] body = jsonBody.getBytes(StandardCharsets.UTF_8);
        return ProtocolMessage.builder()
                .header(MessageHeader.register(body.length))
                .body(body)
                .build();
    }

    public static ProtocolMessage error(String message) {
        String jsonBody = String.format("{\"message\":\"%s\"}", message);
        byte[] body = jsonBody.getBytes(StandardCharsets.UTF_8);
        return ProtocolMessage.builder()
                .header(MessageHeader.error(body.length))
                .body(body)
                .build();
    }
}
