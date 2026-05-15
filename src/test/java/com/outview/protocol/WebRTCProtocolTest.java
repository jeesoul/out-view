package com.outview.protocol;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import java.util.HashSet;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

class WebRTCProtocolTest {
    private final ObjectMapper mapper = new ObjectMapper();

    @Test
    void testWebRTCOffer() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcOffer("conn-1", "v=0\r\n...");
        assertEquals(ProtocolConstants.TYPE_WEBRTC_OFFER, msg.getHeader().getType());
        JsonNode body = mapper.readTree(msg.getBody());
        assertEquals("conn-1", body.get("connectionId").asText());
        assertEquals("offer", body.get("sdpType").asText());
    }

    @Test
    void testWebRTCAnswer() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcAnswer("conn-2", "v=0\r\n...");
        assertEquals(ProtocolConstants.TYPE_WEBRTC_ANSWER, msg.getHeader().getType());
        JsonNode body = mapper.readTree(msg.getBody());
        assertEquals("answer", body.get("sdpType").asText());
    }

    @Test
    void testWebRTCICECandidate() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcICECandidate("conn-3", "candidate:1 ...", "0", 0);
        assertEquals(ProtocolConstants.TYPE_WEBRTC_ICE_CANDIDATE, msg.getHeader().getType());
        JsonNode body = mapper.readTree(msg.getBody());
        assertEquals("candidate:1 ...", body.get("candidate").asText());
        assertEquals(0, body.get("sdpMLineIndex").asInt());
    }

    @Test
    void testWebRTCICEComplete() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcICEComplete("conn-4");
        assertEquals(ProtocolConstants.TYPE_WEBRTC_ICE_COMPLETE, msg.getHeader().getType());
        JsonNode body = mapper.readTree(msg.getBody());
        assertEquals("conn-4", body.get("connectionId").asText());
    }

    @Test
    void testWebRTCEstablished() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcEstablished("conn-5");
        assertEquals(ProtocolConstants.TYPE_WEBRTC_ESTABLISHED, msg.getHeader().getType());
    }

    @Test
    void testWebRTCFailed() throws Exception {
        ProtocolMessage msg = ProtocolMessage.webrtcFailed("conn-6", "ICE failed");
        assertEquals(ProtocolConstants.TYPE_WEBRTC_FAILED, msg.getHeader().getType());
        JsonNode body = mapper.readTree(msg.getBody());
        assertEquals("ICE failed", body.get("reason").asText());
    }

    @Test
    void testTypeConstants_Unique() {
        byte[] types = {
            ProtocolConstants.TYPE_REGISTER,
            ProtocolConstants.TYPE_HEARTBEAT,
            ProtocolConstants.TYPE_DATA,
            ProtocolConstants.TYPE_ERROR,
            ProtocolConstants.TYPE_REGISTER_ACK,
            ProtocolConstants.TYPE_HEARTBEAT_ACK,
            ProtocolConstants.TYPE_CLOSE_CONNECTION,
            ProtocolConstants.TYPE_WEBRTC_OFFER,
            ProtocolConstants.TYPE_WEBRTC_ANSWER,
            ProtocolConstants.TYPE_WEBRTC_ICE_CANDIDATE,
            ProtocolConstants.TYPE_WEBRTC_ICE_COMPLETE,
            ProtocolConstants.TYPE_WEBRTC_ESTABLISHED,
            ProtocolConstants.TYPE_WEBRTC_FAILED,
        };
        Set<Byte> seen = new HashSet<>();
        for (byte t : types) {
            assertTrue(seen.add(t), "Duplicate type value: " + t);
        }
    }
}
