package com.outview.webrtc;

import io.netty.channel.ChannelHandlerContext;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;

class WebRTCConnectionRegistryTest {

    private WebRTCConnectionRegistry registry;

    @BeforeEach
    void setUp() {
        registry = new WebRTCConnectionRegistry();
    }

    @Test
    void testInitiallyEmpty() {
        assertEquals(0, registry.size());
        assertTrue(registry.getAll().isEmpty());
    }

    @Test
    void testRegister_AddsConnection() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        registry.register("conn-1", ctx);

        assertEquals(1, registry.size());
        assertSame(ctx, registry.getContext("conn-1"));
    }

    @Test
    void testRegister_MultipleConnections() {
        ChannelHandlerContext ctx1 = mock(ChannelHandlerContext.class);
        ChannelHandlerContext ctx2 = mock(ChannelHandlerContext.class);

        registry.register("conn-1", ctx1);
        registry.register("conn-2", ctx2);

        assertEquals(2, registry.size());
        assertSame(ctx1, registry.getContext("conn-1"));
        assertSame(ctx2, registry.getContext("conn-2"));
    }

    @Test
    void testUnregister_RemovesConnection() {
        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
        registry.register("conn-1", ctx);
        registry.unregister("conn-1");

        assertEquals(0, registry.size());
        assertNull(registry.getContext("conn-1"));
    }

    @Test
    void testUnregister_UnknownId_NoError() {
        // Should not throw
        assertDoesNotThrow(() -> registry.unregister("nonexistent"));
    }

    @Test
    void testGetContext_UnknownId_ReturnsNull() {
        assertNull(registry.getContext("nonexistent"));
    }

    @Test
    void testGetContext_NullId_ReturnsNull() {
        assertNull(registry.getContext(null));
    }

    @Test
    void testRegister_NullId_NoError() {
        // Should not throw and should not add anything
        assertDoesNotThrow(() -> registry.register(null, mock(ChannelHandlerContext.class)));
        assertEquals(0, registry.size());
    }

    @Test
    void testRegister_NullCtx_NoError() {
        assertDoesNotThrow(() -> registry.register("conn-1", null));
        assertEquals(0, registry.size());
    }

    @Test
    void testGetAll_ReturnsAllIds() {
        ChannelHandlerContext ctx1 = mock(ChannelHandlerContext.class);
        ChannelHandlerContext ctx2 = mock(ChannelHandlerContext.class);
        ChannelHandlerContext ctx3 = mock(ChannelHandlerContext.class);

        registry.register("conn-1", ctx1);
        registry.register("conn-2", ctx2);
        registry.register("conn-3", ctx3);

        Set<String> all = registry.getAll();
        assertEquals(3, all.size());
        assertTrue(all.contains("conn-1"));
        assertTrue(all.contains("conn-2"));
        assertTrue(all.contains("conn-3"));
    }

    @Test
    void testGetAll_IsUnmodifiable() {
        registry.register("conn-1", mock(ChannelHandlerContext.class));
        Set<String> all = registry.getAll();
        assertThrows(UnsupportedOperationException.class, () -> all.add("conn-extra"));
    }

    @Test
    void testRegister_ReplacesExisting() {
        ChannelHandlerContext ctx1 = mock(ChannelHandlerContext.class);
        ChannelHandlerContext ctx2 = mock(ChannelHandlerContext.class);

        registry.register("conn-1", ctx1);
        registry.register("conn-1", ctx2);

        // Size stays 1, context is replaced
        assertEquals(1, registry.size());
        assertSame(ctx2, registry.getContext("conn-1"));
    }

    @Test
    void testThreadSafety_ConcurrentRegisterUnregister() throws InterruptedException {
        int threadCount = 20;
        int opsPerThread = 100;
        ExecutorService executor = Executors.newFixedThreadPool(threadCount);
        CountDownLatch latch = new CountDownLatch(threadCount);
        List<Throwable> errors = new ArrayList<>();

        for (int t = 0; t < threadCount; t++) {
            final int threadId = t;
            executor.submit(() -> {
                try {
                    for (int i = 0; i < opsPerThread; i++) {
                        String id = "conn-" + threadId + "-" + i;
                        ChannelHandlerContext ctx = mock(ChannelHandlerContext.class);
                        registry.register(id, ctx);
                        registry.getContext(id);
                        registry.getAll();
                        registry.size();
                        registry.unregister(id);
                    }
                } catch (Throwable e) {
                    synchronized (errors) {
                        errors.add(e);
                    }
                } finally {
                    latch.countDown();
                }
            });
        }

        assertTrue(latch.await(10, TimeUnit.SECONDS), "Threads did not finish in time");
        executor.shutdown();

        if (!errors.isEmpty()) {
            fail("Thread safety errors: " + errors.get(0).getMessage());
        }
        // All registrations were unregistered
        assertEquals(0, registry.size());
    }
}
