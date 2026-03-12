package com.outview.protocol;

import com.alibaba.fastjson.JSON;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.HashMap;
import java.util.Map;

/**
 * 协议消息
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ProtocolMessage {

    /**
     * 消息头
     */
    private MessageHeader header;

    /**
     * 消息体 (二进制数据)
     */
    private byte[] body;

    /**
     * 创建心跳消息
     */
    public static ProtocolMessage heartbeat() {
        String jsonBody = "{\"timestamp\":" + System.currentTimeMillis() + "}";
        byte[] body = jsonBody.getBytes();
        return ProtocolMessage.builder()
                .header(MessageHeader.heartbeat(body.length))
                .body(body)
                .build();
    }

    /**
     * 创建数据转发消息（无连接ID）
     */
    public static ProtocolMessage data(byte[] payload) {
        return ProtocolMessage.builder()
                .header(MessageHeader.data(payload.length))
                .body(payload)
                .build();
    }

    /**
     * 创建数据转发消息（带连接ID）
     * 消息体格式: JSON {"connectionId":"xxx","data":"base64编码的数据"}
     *
     * @param connectionId 连接ID（用于区分多个用户连接）
     * @param payload      数据负载
     */
    public static ProtocolMessage dataWithConnectionId(String connectionId, byte[] payload) {
        Map<String, Object> dataMap = new HashMap<>();
        dataMap.put("connectionId", connectionId);
        dataMap.put("data", java.util.Base64.getEncoder().encodeToString(payload));
        byte[] body = JSON.toJSONString(dataMap).getBytes();
        return ProtocolMessage.builder()
                .header(MessageHeader.data(body.length))
                .body(body)
                .build();
    }

    /**
     * 解析带连接ID的数据消息
     *
     * @return DataPacket 包含连接ID和原始数据
     */
    public DataPacket parseDataPacket() {
        if (header.getType() != ProtocolConstants.TYPE_DATA) {
            return null;
        }
        try {
            String json = new String(body);
            Map<String, Object> dataMap = JSON.parseObject(json, Map.class);
            String connectionId = (String) dataMap.get("connectionId");
            String dataBase64 = (String) dataMap.get("data");
            byte[] data = java.util.Base64.getDecoder().decode(dataBase64);
            return new DataPacket(connectionId, data);
        } catch (Exception e) {
            // 如果解析失败，可能是不带连接ID的原始数据
            return new DataPacket(null, body);
        }
    }

    /**
     * 数据包
     */
    @Data
    @AllArgsConstructor
    public static class DataPacket {
        private String connectionId;
        private byte[] data;
    }

    /**
     * 创建注册消息
     */
    public static ProtocolMessage register(String deviceId, String token, int localPort) {
        String jsonBody = String.format(
                "{\"deviceId\":\"%s\",\"token\":\"%s\",\"localPort\":%d}",
                deviceId, token, localPort
        );
        byte[] body = jsonBody.getBytes();
        return ProtocolMessage.builder()
                .header(MessageHeader.register(body.length))
                .body(body)
                .build();
    }

    /**
     * 创建错误消息
     */
    public static ProtocolMessage error(String message) {
        String jsonBody = String.format("{\"message\":\"%s\"}", message);
        byte[] body = jsonBody.getBytes();
        return ProtocolMessage.builder()
                .header(MessageHeader.error(body.length))
                .body(body)
                .build();
    }
}