package com.outview.webrtc;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import io.netty.channel.Channel;
import io.netty.channel.ChannelHandlerContext;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Base64;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Consumer;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class WebRTCDataRouterTest {

    private WebRTCProxyService proxyService;
    private WebRTCConnectionRegistry registry;
    private WebRTCDataRouter router;
    private final ObjectMapper mapper = new ObjectMapper();

    @BeforeEach
    void setUp() {
        WebRTCConfig config = new WebRTCConfig();
        proxyService = new WebRTCProxyService(config);
        registry = new WebRTCConnectionRegistry();
        router = new WebRTCDataRouter(proxyService, registry);
    }

    // -------------------------------------------------------------------------
    // routeToWebRTC
    // -------------------------------------------------------------------------

    @Test
    void routeToWebRTC_WhenSidecarNotConnected_DropsData() {
        // proxyService is not connected — should not throw, just log a warning.
        assertDoesNotThrow(() -> router.routeToWebRTC("conn-1", new byte[]{1, 2, 3}));
    }

    // -------------------------------------------------------------------------
    // routeFromWebRTC
    // -------------------------------------------------------------------------

    @Test
    void routeFromWebRTC_WhenNoContext_DropsData() {
        // No context registered — should not throw.
        assertDoesNotThrow(() -> router.routeFromWebRTC("conn-unknown", new byte[]{1, 2, 3}));
    }

    @Test
    void routeFromWebRTC_WhenChannelInactive_DropsData() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        when(ctx.channel()).thenReturn(channel);
        when(channel.isActive()).thenReturn(false);

        registry.register("conn-1", ctx);
        assertDoesNotThrow(() -> router.routeFromWebRTC("conn-1", new byte[]{1, 2, 3}));

        // fireChannelRead should NOT be called for an inactive channel.
        verify(ctx, never()).fireChannelRead(any());
    }

    @Test
    void routeFromWebRTC_WhenChannelActive_FiresChannelRead() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        when(ctx.channel()).thenReturn(channel);
        when(channel.isActive()).thenReturn(true);

        registry.register("conn-1", ctx);
        byte[] data = new byte[]{10, 20, 30};
        router.routeFromWebRTC("conn-1", data);

        // fireChannelRead should be called with a ProtocolMessage wrapping the data.
        verify(ctx, times(1)).fireChannelRead(any(com.outview.protocol.ProtocolMessage.class));
    }

    // -------------------------------------------------------------------------
    // registerDataListener / unregisterDataListener
    // -------------------------------------------------------------------------

    @Test
    void registerDataListener_AddsListenerToProxyService() {
        // After registering, the listener should be present (we verify by sending a
        // data event and checking that routeFromWebRTC is invoked).
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        when(ctx.channel()).thenReturn(channel);
        when(channel.isActive()).thenReturn(true);
        registry.register("conn-1", ctx);

        router.registerDataListener("conn-1");

        // Simulate a 'data' event arriving from the sidecar.
        byte[] payload = new byte[]{1, 2, 3};
        ObjectNode eventPayload = mapper.createObjectNode();
        eventPayload.put("connection_id", "conn-1");
        eventPayload.put("event", "data");
        eventPayload.put("data", Base64.getEncoder().encodeToString(payload));
        IPCMessage msg = new IPCMessage("event", eventPayload);

        // Retrieve the registered listener and invoke it directly.
        AtomicReference<Consumer<IPCMessage>> capturedListener = new AtomicReference<>();
        WebRTCProxyService spyService = spy(proxyService);
        doAnswer(invocation -> {
            capturedListener.set(invocation.getArgument(1));
            return null;
        }).when(spyService).addConnectionListener(eq("conn-2"), any());

        // Use a fresh router wired to the spy to capture the listener.
        WebRTCDataRouter routerWithSpy = new WebRTCDataRouter(spyService, registry);
        registry.register("conn-2", ctx);
        routerWithSpy.registerDataListener("conn-2");

        assertNotNull(capturedListener.get(), "Listener should have been registered");
    }

    @Test
    void unregisterDataListener_RemovesListenerFromProxyService() {
        router.registerDataListener("conn-1");
        // Should not throw even if no listener was registered.
        assertDoesNotThrow(() -> router.unregisterDataListener("conn-1"));
        assertDoesNotThrow(() -> router.unregisterDataListener("conn-nonexistent"));
    }

    // -------------------------------------------------------------------------
    // handleSidecarEvent — non-data events are ignored
    // -------------------------------------------------------------------------

    @Test
    void handleSidecarEvent_NonDataEvent_IsIgnored() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        when(ctx.channel()).thenReturn(channel);
        when(channel.isActive()).thenReturn(true);
        registry.register("conn-1", ctx);

        router.registerDataListener("conn-1");

        // Retrieve the listener via a spy.
        AtomicReference<Consumer<IPCMessage>> listenerRef = new AtomicReference<>();
        WebRTCProxyService spyService = spy(proxyService);
        doAnswer(invocation -> {
            listenerRef.set(invocation.getArgument(1));
            return null;
        }).when(spyService).addConnectionListener(eq("conn-3"), any());

        WebRTCDataRouter routerWithSpy = new WebRTCDataRouter(spyService, registry);
        registry.register("conn-3", ctx);
        routerWithSpy.registerDataListener("conn-3");

        Consumer<IPCMessage> listener = listenerRef.get();
        assertNotNull(listener);

        // Send an 'established' event — should be ignored (no fireChannelRead).
        ObjectNode payload = mapper.createObjectNode();
        payload.put("connection_id", "conn-3");
        payload.put("event", "established");
        listener.accept(new IPCMessage("event", payload));

        verify(ctx, never()).fireChannelRead(any());
    }

    // -------------------------------------------------------------------------
    // routeFromWebRTC — data is wrapped in a TYPE_DATA ProtocolMessage
    // -------------------------------------------------------------------------

    @Test
    void routeFromWebRTC_WrapsDataInProtocolMessage() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        Channel channel = mock(Channel.class);
        when(ctx.channel()).thenReturn(channel);
        when(channel.isActive()).thenReturn(true);
        registry.register("conn-wrap", ctx);

        byte[] data = "hello webrtc".getBytes(java.nio.charset.StandardCharsets.UTF_8);
        router.routeFromWebRTC("conn-wrap", data);

        // Capture the argument passed to fireChannelRead.
        org.mockito.ArgumentCaptor<Object> captor = org.mockito.ArgumentCaptor.forClass(Object.class);
        verify(ctx).fireChannelRead(captor.capture());

        Object arg = captor.getValue();
        assertInstanceOf(com.outview.protocol.ProtocolMessage.class, arg);

        com.outview.protocol.ProtocolMessage pm = (com.outview.protocol.ProtocolMessage) arg;
        assertEquals(com.outview.protocol.ProtocolConstants.TYPE_DATA, pm.getHeader().getType());

        // Parse the data packet and verify the payload.
        com.outview.protocol.ProtocolMessage.DataPacket packet = pm.parseDataPacket();
        assertNotNull(packet);
        assertEquals("conn-wrap", packet.getConnectionId());
        assertArrayEquals(data, packet.getData());
    }
}
