package com.outview.webrtc;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Consumer;

import static org.junit.jupiter.api.Assertions.*;

class WebRTCProxyServiceTest {
    private WebRTCConfig config;
    private WebRTCProxyService service;
    private final ObjectMapper mapper = new ObjectMapper();

    @BeforeEach
    void setUp() {
        config = new WebRTCConfig();
        service = new WebRTCProxyService(config);
    }

    @Test
    void testIsConnected_InitiallyFalse() {
        assertFalse(service.isConnected());
    }

    @Test
    void testSend_WhenNotConnected_ThrowsIOException() {
        assertThrows(java.io.IOException.class, () -> {
            service.createPC("test-conn");
        });
    }

    @Test
    void testAddConnectionListener_AndRemove() {
        AtomicReference<IPCMessage> received = new AtomicReference<>();
        Consumer<IPCMessage> listener = new Consumer<IPCMessage>() {
            @Override
            public void accept(IPCMessage msg) {
                received.set(msg);
            }
        };

        service.addConnectionListener("conn-1", listener);
        service.removeConnectionListener("conn-1");

        // After removal, listener should not be called.
        // No exception expected.
    }

    @Test
    void testConfig_Defaults() {
        assertEquals("/tmp/outview-webrtc.sock", config.getSidecarSocketPath());
        assertEquals(5000, config.getConnectTimeoutMs());
        assertTrue(config.isEnabled());
    }

    @Test
    void testIPCMessage_Serialization() throws Exception {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", "test-123");
        IPCMessage msg = new IPCMessage("create_pc", payload);

        String json = mapper.writeValueAsString(msg);
        assertTrue(json.contains("create_pc"));
        assertTrue(json.contains("test-123"));
    }
}
