package com.outview.netty.handler;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.webrtc.WebRTCConnectionRegistry;
import com.outview.webrtc.WebRTCProxyService;
import io.netty.channel.ChannelHandler;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.util.Set;

/**
 * WebRTC signaling handler.
 *
 * <p>Handles WebRTC signaling messages (types 8-13) arriving from the Go client over the
 * binary protocol. For each Offer it creates a PeerConnection via the sidecar, obtains an
 * Answer, and writes it back to the channel. ICE candidates are forwarded to the sidecar.
 * ICE-complete, Established, and Failed messages are logged (and Failed triggers cleanup).
 */
@Slf4j
@Component
@ChannelHandler.Sharable
public class WebRTCHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private final WebRTCProxyService webRTCProxyService;
    private final WebRTCConnectionRegistry registry;

    public WebRTCHandler(WebRTCProxyService webRTCProxyService, WebRTCConnectionRegistry registry) {
        this.webRTCProxyService = webRTCProxyService;
        this.registry = registry;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
        byte type = msg.getHeader().getType();

        switch (type) {
            case ProtocolConstants.TYPE_WEBRTC_OFFER:
                handleOffer(ctx, msg);
                break;
            case ProtocolConstants.TYPE_WEBRTC_ICE_CANDIDATE:
                handleICECandidate(msg);
                break;
            case ProtocolConstants.TYPE_WEBRTC_ICE_COMPLETE:
                handleICEComplete(msg);
                break;
            case ProtocolConstants.TYPE_WEBRTC_ESTABLISHED:
                handleEstablished(ctx, msg);
                break;
            case ProtocolConstants.TYPE_WEBRTC_FAILED:
                handleFailed(msg);
                break;
            default:
                // Not a WebRTC message — pass to the next handler in the pipeline
                ctx.fireChannelRead(msg);
                break;
        }
    }

    // -------------------------------------------------------------------------
    // Offer: create PC on sidecar, set remote SDP, wait for answer event
    // -------------------------------------------------------------------------

    private void handleOffer(ChannelHandlerContext ctx, ProtocolMessage msg) {
        String body = new String(msg.getBody(), StandardCharsets.UTF_8);
        try {
            JsonNode json = MAPPER.readTree(body);
            String connectionId = json.path("connectionId").asText(null);
            String sdp = json.path("sdp").asText(null);
            String sdpType = json.path("sdpType").asText("offer");

            if (connectionId == null || sdp == null) {
                log.warn("[WebRTCHandler] Offer missing connectionId or sdp");
                return;
            }

            log.info("[WebRTCHandler] Received offer: connectionId={}, sdpType={}", connectionId, sdpType);

            // Register a one-shot listener that will send the answer back when the sidecar
            // emits an "answer" event for this connection.
            webRTCProxyService.addConnectionListener(connectionId, ipcMsg -> {
                try {
                    JsonNode payload = ipcMsg.getPayload();
                    String event = payload.path("event").asText("");
                    if ("answer".equals(event) || "sdp_answer".equals(event)) {
                        String answerSdp = payload.path("sdp").asText(null);
                        if (answerSdp == null) {
                            log.warn("[WebRTCHandler] Answer event missing sdp for connectionId={}", connectionId);
                            return;
                        }
                        log.info("[WebRTCHandler] Sending answer to client: connectionId={}", connectionId);
                        ctx.writeAndFlush(ProtocolMessage.webrtcAnswer(connectionId, answerSdp));
                        // Keep the listener alive for subsequent ICE / established events
                    } else if ("ice_candidate".equals(event)) {
                        // Field names match sidecar EventPayload JSON tags: sdp_mid (snake_case).
                        // sdp_mline_index is not emitted by the sidecar; null is handled gracefully.
                        String candidate = payload.path("candidate").asText(null);
                        String sdpMid = payload.path("sdp_mid").asText(null);
                        Integer sdpMLineIndex = payload.has("sdp_mline_index")
                                ? payload.path("sdp_mline_index").asInt() : null;
                        if (candidate != null) {
                            log.debug("[WebRTCHandler] Forwarding ICE candidate to client: connectionId={}", connectionId);
                            ctx.writeAndFlush(ProtocolMessage.webrtcICECandidate(connectionId, candidate, sdpMid, sdpMLineIndex));
                        }
                    } else if ("ice_complete".equals(event)) {
                        log.info("[WebRTCHandler] ICE gathering complete on sidecar: connectionId={}", connectionId);
                        ctx.writeAndFlush(ProtocolMessage.webrtcICEComplete(connectionId));
                    } else if ("established".equals(event)) {
                        log.info("[WebRTCHandler] WebRTC established (sidecar event): connectionId={}", connectionId);
                        ctx.writeAndFlush(ProtocolMessage.webrtcEstablished(connectionId));
                        webRTCProxyService.removeConnectionListener(connectionId);
                    } else if ("failed".equals(event)) {
                        String reason = payload.path("reason").asText("unknown");
                        log.warn("[WebRTCHandler] WebRTC failed (sidecar event): connectionId={}, reason={}", connectionId, reason);
                        ctx.writeAndFlush(ProtocolMessage.webrtcFailed(connectionId, reason));
                        webRTCProxyService.removeConnectionListener(connectionId);
                    }
                } catch (Exception e) {
                    log.error("[WebRTCHandler] Error processing sidecar event for connectionId={}", connectionId, e);
                }
            });

            // Tell the sidecar to create a PeerConnection and set the remote SDP (offer).
            // If either call throws, remove the listener to avoid a permanent leak.
            try {
                webRTCProxyService.createPC(connectionId);
                webRTCProxyService.setRemoteSDP(connectionId, sdp, sdpType);
            } catch (Exception e) {
                webRTCProxyService.removeConnectionListener(connectionId);
                log.error("[WebRTCHandler] Failed to set up WebRTC for {}: {}", connectionId, e.getMessage());
            }

        } catch (Exception e) {
            log.error("[WebRTCHandler] Failed to handle offer", e);
        }
    }

    // -------------------------------------------------------------------------
    // ICE candidate from client → forward to sidecar
    // -------------------------------------------------------------------------

    private void handleICECandidate(ProtocolMessage msg) {
        String body = new String(msg.getBody(), StandardCharsets.UTF_8);
        try {
            JsonNode json = MAPPER.readTree(body);
            String connectionId = json.path("connectionId").asText(null);
            String candidate = json.path("candidate").asText(null);
            String sdpMid = json.path("sdpMid").asText(null);
            Integer sdpMLineIndex = json.has("sdpMLineIndex") ? json.path("sdpMLineIndex").asInt() : null;

            if (connectionId == null || candidate == null) {
                log.warn("[WebRTCHandler] ICE candidate missing connectionId or candidate");
                return;
            }

            log.debug("[WebRTCHandler] Received ICE candidate: connectionId={}", connectionId);
            webRTCProxyService.addICECandidate(connectionId, candidate, sdpMid, sdpMLineIndex);

        } catch (Exception e) {
            log.error("[WebRTCHandler] Failed to handle ICE candidate", e);
        }
    }

    // -------------------------------------------------------------------------
    // ICE complete — client finished sending candidates; just log it
    // -------------------------------------------------------------------------

    private void handleICEComplete(ProtocolMessage msg) {
        String connectionId = parseConnectionId(msg);
        log.info("[WebRTCHandler] Client ICE gathering complete: connectionId={}", connectionId);
    }

    // -------------------------------------------------------------------------
    // Established — WebRTC data channel is open
    // -------------------------------------------------------------------------

    private void handleEstablished(ChannelHandlerContext ctx, ProtocolMessage msg) {
        String connectionId = parseConnectionId(msg);
        log.info("[WebRTCHandler] WebRTC established: connectionId={}", connectionId);
        if (connectionId != null) {
            registry.register(connectionId, ctx);
        }
    }

    // -------------------------------------------------------------------------
    // Failed — WebRTC negotiation failed; clean up sidecar resources
    // -------------------------------------------------------------------------

    private void handleFailed(ProtocolMessage msg) {
        String body = new String(msg.getBody(), StandardCharsets.UTF_8);
        try {
            JsonNode json = MAPPER.readTree(body);
            String connectionId = json.path("connectionId").asText(null);
            String reason = json.path("reason").asText("unknown");

            log.warn("[WebRTCHandler] WebRTC failed: connectionId={}, reason={}", connectionId, reason);

            if (connectionId != null) {
                webRTCProxyService.removeConnectionListener(connectionId);
                registry.unregister(connectionId);
                try {
                    webRTCProxyService.closePC(connectionId);
                } catch (Exception e) {
                    log.warn("[WebRTCHandler] closePC failed for connectionId={}: {}", connectionId, e.getMessage());
                }
            }
        } catch (Exception e) {
            log.error("[WebRTCHandler] Failed to handle WebRTC failed message", e);
        }
    }

    // -------------------------------------------------------------------------
    // Helpers
    // -------------------------------------------------------------------------

    private String parseConnectionId(ProtocolMessage msg) {
        try {
            JsonNode json = MAPPER.readTree(msg.getBody());
            return json.path("connectionId").asText(null);
        } catch (Exception e) {
            log.warn("[WebRTCHandler] Failed to parse connectionId from message", e);
            return null;
        }
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) throws Exception {
        // When the channel closes, clean up all WebRTC connections that were using it.
        Set<String> allIds = registry.getAll();
        for (String connectionId : allIds) {
            if (ctx.equals(registry.getContext(connectionId))) {
                log.info("[WebRTCHandler] Channel closed, cleaning up connectionId={}", connectionId);
                registry.unregister(connectionId);
                try {
                    webRTCProxyService.closePC(connectionId);
                } catch (Exception e) {
                    log.warn("[WebRTCHandler] closePC on channel inactive failed for connectionId={}: {}",
                            connectionId, e.getMessage());
                }
            }
        }
        ctx.fireChannelInactive();
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) throws Exception {
        log.error("[WebRTCHandler] Exception: channel={}", ctx.channel().id().asShortText(), cause);
        ctx.close();
    }
}
