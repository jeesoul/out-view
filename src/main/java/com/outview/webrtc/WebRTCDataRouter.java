package com.outview.webrtc;

import com.fasterxml.jackson.databind.JsonNode;
import io.netty.channel.ChannelHandlerContext;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.util.function.Consumer;

/**
 * Routes data between the Java server and the Go sidecar over the WebRTC DataChannel.
 *
 * <p>Outbound path (server → client): {@link #routeToWebRTC(String, byte[])} sends data
 * to the sidecar via {@link WebRTCProxyService#sendData(String, byte[])}, which forwards
 * it over the DataChannel to the Go client.
 *
 * <p>Inbound path (client → server): the sidecar emits {@code event} messages with
 * {@code event=data}. This router registers itself as an IPC listener for each active
 * WebRTC connection and calls {@link #routeFromWebRTC(String, byte[])} to deliver the
 * data to the appropriate Netty channel via {@link WebRTCConnectionRegistry}.
 */
@Component
public class WebRTCDataRouter {

    private static final Logger log = LoggerFactory.getLogger(WebRTCDataRouter.class);

    private final WebRTCProxyService proxyService;
    private final WebRTCConnectionRegistry registry;

    public WebRTCDataRouter(WebRTCProxyService proxyService, WebRTCConnectionRegistry registry) {
        this.proxyService = proxyService;
        this.registry = registry;
    }

    /**
     * Register an IPC data listener for the given connection.
     *
     * <p>Call this after the WebRTC connection is established so that inbound
     * {@code data} events from the sidecar are delivered to the Netty channel.
     *
     * @param connectionId the WebRTC connection identifier
     */
    public void registerDataListener(String connectionId) {
        proxyService.addConnectionListener(connectionId, new Consumer<IPCMessage>() {
            @Override
            public void accept(IPCMessage msg) {
                handleSidecarEvent(connectionId, msg);
            }
        });
        log.debug("[WebRTCDataRouter] Registered data listener for connectionId={}", connectionId);
    }

    /**
     * Remove the IPC data listener for the given connection.
     *
     * @param connectionId the WebRTC connection identifier
     */
    public void unregisterDataListener(String connectionId) {
        proxyService.removeConnectionListener(connectionId);
        log.debug("[WebRTCDataRouter] Unregistered data listener for connectionId={}", connectionId);
    }

    /**
     * Send data to the Go client via the WebRTC DataChannel.
     *
     * @param connectionId the WebRTC connection identifier
     * @param data         raw bytes to send
     */
    public void routeToWebRTC(String connectionId, byte[] data) {
        if (!proxyService.isConnected()) {
            log.warn("[WebRTCDataRouter] Sidecar not connected, dropping outbound data for connectionId={}", connectionId);
            return;
        }
        try {
            proxyService.sendData(connectionId, data);
            log.debug("[WebRTCDataRouter] Sent {} bytes to sidecar for connectionId={}", data.length, connectionId);
        } catch (IOException e) {
            log.error("[WebRTCDataRouter] Failed to send data to sidecar for connectionId={}: {}", connectionId, e.getMessage());
        }
    }

    /**
     * Deliver data received from the DataChannel to the appropriate Netty channel.
     *
     * @param connectionId the WebRTC connection identifier
     * @param data         raw bytes received from the DataChannel
     */
    public void routeFromWebRTC(String connectionId, byte[] data) {
        ChannelHandlerContext ctx = registry.getContext(connectionId);
        if (ctx == null) {
            log.warn("[WebRTCDataRouter] No Netty context for connectionId={}, dropping {} bytes", connectionId, data.length);
            return;
        }
        if (!ctx.channel().isActive()) {
            log.warn("[WebRTCDataRouter] Netty channel inactive for connectionId={}, dropping {} bytes", connectionId, data.length);
            return;
        }
        // Wrap the raw payload in a TYPE_DATA ProtocolMessage so downstream
        // handlers (e.g. ProxyHandler / RawDataHandler) can process it normally.
        ctx.fireChannelRead(com.outview.protocol.ProtocolMessage.dataWithConnectionId(connectionId, data));
        log.debug("[WebRTCDataRouter] Delivered {} bytes from DataChannel to Netty for connectionId={}", data.length, connectionId);
    }

    // -------------------------------------------------------------------------
    // Internal
    // -------------------------------------------------------------------------

    private void handleSidecarEvent(String connectionId, IPCMessage msg) {
        JsonNode payload = msg.getPayload();
        if (payload == null) {
            return;
        }
        String event = payload.path("event").asText("");
        if (!"data".equals(event)) {
            // Not a data event — ignore (other events are handled by WebRTCHandler).
            return;
        }

        // Jackson deserialises base64-encoded JSON byte arrays automatically.
        JsonNode dataNode = payload.path("data");
        if (dataNode.isMissingNode() || dataNode.isNull()) {
            log.warn("[WebRTCDataRouter] Data event missing 'data' field for connectionId={}", connectionId);
            return;
        }

        byte[] data;
        try {
            data = dataNode.binaryValue();
            if (data == null) {
                // Fallback: treat as plain text bytes (should not happen with correct sidecar).
                data = dataNode.asText().getBytes(java.nio.charset.StandardCharsets.UTF_8);
            }
        } catch (IOException e) {
            log.error("[WebRTCDataRouter] Failed to decode data for connectionId={}: {}", connectionId, e.getMessage());
            return;
        }

        routeFromWebRTC(connectionId, data);
    }
}
