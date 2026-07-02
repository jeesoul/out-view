package com.outview.netty.handler;

import com.outview.webrtc.WebRTCConnectionRegistry;
import com.outview.webrtc.WebRTCDataRouter;
import com.outview.webrtc.WebRTCProxyService;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelId;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link WebRTCHandler#channelInactive(ChannelHandlerContext)}.
 *
 * <p>Verifies that when a Netty channel closes, all WebRTC connections that were
 * using that channel are cleaned up: the data listener is unregistered, the
 * registry entry is removed, and {@code closePC} is called on the proxy service.
 */
class WebRTCHandlerTest {

    private WebRTCProxyService proxyService;
    private WebRTCConnectionRegistry registry;
    private WebRTCDataRouter dataRouter;
    private WebRTCHandler handler;

    @BeforeEach
    void setUp() {
        proxyService = mock(WebRTCProxyService.class);
        registry = new WebRTCConnectionRegistry();
        dataRouter = mock(WebRTCDataRouter.class);
        handler = new WebRTCHandler(proxyService, registry, dataRouter);
    }

    // -------------------------------------------------------------------------
    // channelInactive — single connection on the closing channel
    // -------------------------------------------------------------------------

    @Test
    void channelInactive_SingleConnection_CleanedUp() throws Exception {
        ChannelHandlerContext ctx = mockCtx();
        registry.register("conn-1", ctx);

        handler.channelInactive(ctx);

        // Registry entry removed.
        assertNull(registry.getContext("conn-1"), "registry entry should be removed");
        assertEquals(0, registry.size());

        // Data listener unregistered.
        verify(dataRouter).unregisterDataListener("conn-1");

        // PeerConnection closed on sidecar.
        verify(proxyService).closePC("conn-1");

        // channelInactive propagated up the pipeline.
        verify(ctx).fireChannelInactive();
    }

    // -------------------------------------------------------------------------
    // channelInactive — multiple connections, only those on the closing channel
    // -------------------------------------------------------------------------

    @Test
    void channelInactive_MultipleConnections_OnlyMatchingChannelCleaned() throws Exception {
        ChannelHandlerContext closingCtx = mockCtx();
        ChannelHandlerContext otherCtx = mockCtx();

        registry.register("conn-closing", closingCtx);
        registry.register("conn-other", otherCtx);

        handler.channelInactive(closingCtx);

        // Only the connection on the closing channel is removed.
        assertNull(registry.getContext("conn-closing"), "conn-closing should be removed");
        assertNotNull(registry.getContext("conn-other"), "conn-other should remain");

        verify(dataRouter).unregisterDataListener("conn-closing");
        verify(proxyService).closePC("conn-closing");

        // The other connection must NOT be touched.
        verify(dataRouter, never()).unregisterDataListener("conn-other");
        verify(proxyService, never()).closePC("conn-other");

        verify(closingCtx).fireChannelInactive();
    }

    // -------------------------------------------------------------------------
    // channelInactive — no connections registered for this channel
    // -------------------------------------------------------------------------

    @Test
    void channelInactive_NoConnectionsForChannel_NoCleanupCalled() throws Exception {
        ChannelHandlerContext ctx = mockCtx();
        // Registry is empty — nothing to clean up.

        handler.channelInactive(ctx);

        verify(dataRouter, never()).unregisterDataListener(any());
        verify(proxyService, never()).closePC(any());
        verify(ctx).fireChannelInactive();
    }

    // -------------------------------------------------------------------------
    // channelInactive — closePC throws; cleanup continues and pipeline fires
    // -------------------------------------------------------------------------

    @Test
    void channelInactive_ClosePCThrows_CleanupContinues() throws Exception {
        ChannelHandlerContext ctx = mockCtx();
        registry.register("conn-err", ctx);

        doThrow(new RuntimeException("sidecar unavailable"))
                .when(proxyService).closePC("conn-err");

        // Should not propagate the exception.
        assertDoesNotThrow(() -> handler.channelInactive(ctx));

        // Registry and data listener still cleaned up.
        assertNull(registry.getContext("conn-err"));
        verify(dataRouter).unregisterDataListener("conn-err");

        // Pipeline still fires.
        verify(ctx).fireChannelInactive();
    }

    // -------------------------------------------------------------------------
    // channelInactive — multiple connections on the same channel, all cleaned up
    // -------------------------------------------------------------------------

    @Test
    void channelInactive_MultipleConnectionsSameChannel_AllCleaned() throws Exception {
        ChannelHandlerContext ctx = mockCtx();

        registry.register("conn-a", ctx);
        registry.register("conn-b", ctx);
        registry.register("conn-c", ctx);

        handler.channelInactive(ctx);

        assertEquals(0, registry.size(), "all connections should be removed");

        verify(dataRouter).unregisterDataListener("conn-a");
        verify(dataRouter).unregisterDataListener("conn-b");
        verify(dataRouter).unregisterDataListener("conn-c");

        verify(proxyService).closePC("conn-a");
        verify(proxyService).closePC("conn-b");
        verify(proxyService).closePC("conn-c");

        verify(ctx).fireChannelInactive();
    }

    // -------------------------------------------------------------------------
    // Helper
    // -------------------------------------------------------------------------

    /**
     * Creates a mock {@link ChannelHandlerContext} with a unique {@link Channel}
     * so that {@code ctx.equals(other)} returns false for different mocks.
     */
    private static ChannelHandlerContext mockCtx() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        ChannelId channelId = mock(ChannelId.class);
        when(channel.id()).thenReturn(channelId);
        when(channelId.asShortText()).thenReturn("test-channel-" + System.nanoTime());
        when(ctx.channel()).thenReturn(channel);
        return ctx;
    }
}
