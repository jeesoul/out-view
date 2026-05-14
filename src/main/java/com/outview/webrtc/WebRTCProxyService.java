package com.outview.webrtc;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.io.IOException;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Consumer;

/**
 * Main service for interacting with the WebRTC Sidecar over IPC.
 * Manages the IPCConnection, provides per-command methods, and routes
 * incoming events to per-connection listeners.
 */
@Service
public class WebRTCProxyService {
    private static final Logger log = LoggerFactory.getLogger(WebRTCProxyService.class);

    private final WebRTCConfig config;
    private final ObjectMapper mapper = new ObjectMapper();
    private final Map<String, Consumer<IPCMessage>> connectionListeners = new ConcurrentHashMap<>();

    private IPCConnection connection;
    private volatile boolean connected = false;

    public WebRTCProxyService(WebRTCConfig config) {
        this.config = config;
    }

    /**
     * Connect to the Sidecar IPC server. Must be called before any other methods.
     */
    public synchronized void connect() throws IOException {
        if (connected) return;
        connection = new IPCConnection();
        connection.connect(config.getSidecarSocketPath(), config.getConnectTimeoutMs());
        connection.addListener(new Consumer<IPCMessage>() {
            @Override
            public void accept(IPCMessage msg) {
                handleIncomingMessage(msg);
            }
        });
        connected = true;
        log.info("WebRTCProxyService connected to sidecar");
    }

    /**
     * Disconnect from the Sidecar.
     */
    public synchronized void disconnect() {
        if (!connected) return;
        connected = false;
        if (connection != null) {
            connection.close();
            connection = null;
        }
        connectionListeners.clear();
        log.info("WebRTCProxyService disconnected");
    }

    /** Register a listener for events on a specific connectionID. */
    public void addConnectionListener(String connectionID, Consumer<IPCMessage> listener) {
        connectionListeners.put(connectionID, listener);
    }

    /** Remove the listener for a connectionID. */
    public void removeConnectionListener(String connectionID) {
        connectionListeners.remove(connectionID);
    }

    /** Send create_pc command to the Sidecar. */
    public void createPC(String connectionID) throws IOException {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", connectionID);
        send("create_pc", payload);
    }

    /** Send set_remote_sdp command. */
    public void setRemoteSDP(String connectionID, String sdp, String type) throws IOException {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", connectionID);
        payload.put("sdp", sdp);
        payload.put("type", type);
        send("set_remote_sdp", payload);
    }

    /** Send add_ice_candidate command. */
    public void addICECandidate(String connectionID, String candidate,
                                String sdpMid, Integer sdpMLineIndex) throws IOException {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", connectionID);
        payload.put("candidate", candidate);
        if (sdpMid != null) payload.put("sdp_mid", sdpMid);
        if (sdpMLineIndex != null) payload.put("sdp_mline_index", sdpMLineIndex);
        send("add_ice_candidate", payload);
    }

    /** Send data over WebRTC DataChannel. Jackson serializes byte[] as base64. */
    public void sendData(String connectionID, byte[] data) throws IOException {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", connectionID);
        payload.put("data", data);
        send("send_data", payload);
    }

    /** Send close_pc command. */
    public void closePC(String connectionID) throws IOException {
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", connectionID);
        send("close_pc", payload);
        connectionListeners.remove(connectionID);
    }

    public boolean isConnected() {
        return connected && connection != null && !connection.isClosed();
    }

    private void send(String type, ObjectNode payload) throws IOException {
        if (!connected || connection == null) {
            throw new IOException("Not connected to sidecar");
        }
        connection.send(new IPCMessage(type, payload));
    }

    private void handleIncomingMessage(IPCMessage msg) {
        if (!"event".equals(msg.getType())) {
            log.warn("Unexpected message type from sidecar: {}", msg.getType());
            return;
        }
        if (msg.getPayload() == null) {
            log.warn("Event message has no payload");
            return;
        }
        String connectionID = msg.getPayload().path("connection_id").asText(null);
        if (connectionID == null) {
            log.warn("Event message missing connection_id");
            return;
        }
        Consumer<IPCMessage> listener = connectionListeners.get(connectionID);
        if (listener != null) {
            try {
                listener.accept(msg);
            } catch (Exception e) {
                log.error("Connection listener error for {}", connectionID, e);
            }
        } else {
            log.debug("No listener for connectionID: {}", connectionID);
        }
    }
}
