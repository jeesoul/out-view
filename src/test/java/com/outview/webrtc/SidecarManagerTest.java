package com.outview.webrtc;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

import java.io.IOException;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class SidecarManagerTest {
    private WebRTCConfig config;
    private WebRTCProxyService proxyService;
    private SidecarManager manager;

    @BeforeEach
    void setUp() {
        config = new WebRTCConfig();
        proxyService = Mockito.mock(WebRTCProxyService.class);
        manager = new SidecarManager(config, proxyService);
    }

    @Test
    void testStart_WhenDisabled_DoesNotStart() throws IOException {
        config.setEnabled(false);
        manager.start();
        assertFalse(manager.isRunning());
    }

    @Test
    void testStart_WithNonExistentBinary_ThrowsException() {
        config.setEnabled(true);
        config.setSidecarBinaryPath("/nonexistent/binary");
        assertThrows(IOException.class, () -> manager.start());
    }

    @Test
    void testDestroy_CalledTwice_NoPanic() throws Exception {
        config.setEnabled(false); // don't actually start
        manager.destroy();
        manager.destroy(); // should not throw
    }

    @Test
    void testIsRunning_InitiallyFalse() {
        assertFalse(manager.isRunning());
    }

    @Test
    void testGetRestartCount_InitiallyZero() {
        assertEquals(0, manager.getRestartCount());
    }
}
