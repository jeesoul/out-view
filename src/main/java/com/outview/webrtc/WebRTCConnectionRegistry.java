package com.outview.webrtc;

import io.netty.channel.ChannelHandlerContext;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.Collections;
import java.util.HashSet;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Thread-safe registry that tracks active WebRTC connections.
 *
 * <p>Each entry maps a connectionId (string) to the Netty {@link ChannelHandlerContext}
 * that owns the connection. The registry is used by {@code WebRTCHandler} to:
 * <ul>
 *   <li>Record a connection when the sidecar reports it as established.</li>
 *   <li>Remove a connection when it fails or the underlying channel closes.</li>
 *   <li>Look up the channel context for a given connectionId.</li>
 * </ul>
 */
@Slf4j
@Component
public class WebRTCConnectionRegistry {

    private final ConcurrentHashMap<String, ChannelHandlerContext> connections =
            new ConcurrentHashMap<>();

    /**
     * Register a new WebRTC connection.
     *
     * @param connectionId unique connection identifier
     * @param ctx          Netty channel context for the connection
     */
    public void register(String connectionId, ChannelHandlerContext ctx) {
        if (connectionId == null || ctx == null) {
            log.warn("[WebRTCConnectionRegistry] Attempted to register with null connectionId or ctx");
            return;
        }
        ChannelHandlerContext previous = connections.put(connectionId, ctx);
        if (previous != null) {
            log.warn("[WebRTCConnectionRegistry] Replaced existing registration for connectionId={}", connectionId);
        } else {
            log.info("[WebRTCConnectionRegistry] Registered connectionId={}, total={}", connectionId, connections.size());
        }
    }

    /**
     * Remove a connection from the registry.
     *
     * @param connectionId unique connection identifier
     */
    public void unregister(String connectionId) {
        if (connectionId == null) {
            return;
        }
        ChannelHandlerContext removed = connections.remove(connectionId);
        if (removed != null) {
            log.info("[WebRTCConnectionRegistry] Unregistered connectionId={}, total={}", connectionId, connections.size());
        } else {
            log.debug("[WebRTCConnectionRegistry] Unregister called for unknown connectionId={}", connectionId);
        }
    }

    /**
     * Get the Netty channel context for a connection.
     *
     * @param connectionId unique connection identifier
     * @return the {@link ChannelHandlerContext}, or {@code null} if not found
     */
    public ChannelHandlerContext getContext(String connectionId) {
        if (connectionId == null) {
            return null;
        }
        return connections.get(connectionId);
    }

    /**
     * Return an unmodifiable snapshot of all active connection IDs at the time of the call.
     *
     * <p>The returned set is a true snapshot — it is not backed by the live map, so
     * callers may safely iterate it while concurrently mutating the registry (e.g. calling
     * {@link #unregister(String)} inside the loop).
     *
     * @return snapshot of active connection IDs
     */
    public Set<String> getAll() {
        return Collections.unmodifiableSet(new HashSet<>(connections.keySet()));
    }

    /**
     * Return the number of active connections.
     *
     * @return active connection count
     */
    public int size() {
        return connections.size();
    }
}
