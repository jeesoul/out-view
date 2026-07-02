package com.outview.protocol;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
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

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private MessageHeader header;
    private byte[] body;

    public static ProtocolMessage heartbeat() {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("timestamp", System.currentTimeMillis());
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.heartbeat(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create heartbeat message", e);
        }
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
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("deviceId", deviceId);
            node.put("token", token);
            node.put("localPort", localPort);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.register(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create register message", e);
        }
    }

    public static ProtocolMessage error(String message) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("message", message);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.error(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create error message", e);
        }
    }

    public static ProtocolMessage webrtcOffer(String connectionId, String sdp) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            node.put("sdp", sdp);
            node.put("sdpType", "offer");
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcOffer(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc offer message", e);
        }
    }

    public static ProtocolMessage webrtcAnswer(String connectionId, String sdp) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            node.put("sdp", sdp);
            node.put("sdpType", "answer");
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcAnswer(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc answer message", e);
        }
    }

    public static ProtocolMessage webrtcICECandidate(String connectionId, String candidate, String sdpMid, Integer sdpMLineIndex) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            node.put("candidate", candidate);
            if (sdpMid != null) node.put("sdpMid", sdpMid);
            if (sdpMLineIndex != null) node.put("sdpMLineIndex", sdpMLineIndex);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcICECandidate(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc ice candidate message", e);
        }
    }

    public static ProtocolMessage webrtcICEComplete(String connectionId) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcICEComplete(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc ice complete message", e);
        }
    }

    public static ProtocolMessage webrtcEstablished(String connectionId) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcEstablished(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc established message", e);
        }
    }

    public static ProtocolMessage webrtcFailed(String connectionId, String reason) {
        try {
            ObjectNode node = MAPPER.createObjectNode();
            node.put("connectionId", connectionId);
            if (reason != null) node.put("reason", reason);
            byte[] body = MAPPER.writeValueAsBytes(node);
            return ProtocolMessage.builder()
                    .header(MessageHeader.webrtcFailed(body.length))
                    .body(body)
                    .build();
        } catch (Exception e) {
            throw new RuntimeException("Failed to create webrtc failed message", e);
        }
    }
}
