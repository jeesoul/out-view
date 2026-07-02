package com.outview.webrtc;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.*;

class WebRTCMetricsTest {

    private WebRTCMetrics metrics;

    @BeforeEach
    void setUp() {
        metrics = new WebRTCMetrics();
    }

    @Test
    void testInitialState() {
        assertEquals(0, metrics.getConnectionsTotal());
        assertEquals(0, metrics.getConnectionsActive());
        assertEquals(0, metrics.getSuccessCount());
        assertEquals(0, metrics.getFallbackCount());
        assertEquals(0, metrics.getErrorsTotal());
        assertEquals(0.0, metrics.getSuccessRate());
        assertEquals(0.0, metrics.getFallbackRate());
    }

    @Test
    void testRecordAttemptIncrementsTotalAndActive() {
        metrics.recordConnectionAttempt();
        metrics.recordConnectionAttempt();
        assertEquals(2, metrics.getConnectionsTotal());
        assertEquals(2, metrics.getConnectionsActive());
    }

    @Test
    void testRecordCloseDecrementsActiveOnly() {
        metrics.recordConnectionAttempt();
        metrics.recordConnectionAttempt();
        metrics.recordConnectionClosed();
        assertEquals(2, metrics.getConnectionsTotal());
        assertEquals(1, metrics.getConnectionsActive());
    }

    @Test
    void testSuccessRate() {
        for (int i = 0; i < 10; i++) {
            metrics.recordConnectionAttempt();
        }
        for (int i = 0; i < 7; i++) {
            metrics.recordConnectionSuccess();
        }
        assertEquals(0.7, metrics.getSuccessRate(), 1e-9);
    }

    @Test
    void testFallbackRate() {
        for (int i = 0; i < 5; i++) {
            metrics.recordConnectionAttempt();
        }
        metrics.recordFallback();
        metrics.recordFallback();
        assertEquals(0.4, metrics.getFallbackRate(), 1e-9);
    }

    @Test
    void testErrors() {
        metrics.recordError();
        metrics.recordError();
        metrics.recordError();
        assertEquals(3, metrics.getErrorsTotal());
    }

    @Test
    void testReset() {
        metrics.recordConnectionAttempt();
        metrics.recordConnectionSuccess();
        metrics.recordError();
        metrics.reset();
        assertEquals(0, metrics.getConnectionsTotal());
        assertEquals(0, metrics.getSuccessCount());
        assertEquals(0, metrics.getErrorsTotal());
    }

    @Test
    void testConcurrentAccuracy() throws Exception {
        int threads = 8;
        int perThread = 5_000;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        CountDownLatch latch = new CountDownLatch(threads);
        try {
            for (int t = 0; t < threads; t++) {
                pool.execute(() -> {
                    try {
                        for (int i = 0; i < perThread; i++) {
                            metrics.recordConnectionAttempt();
                            metrics.recordConnectionSuccess();
                            metrics.recordConnectionClosed();
                        }
                    } finally {
                        latch.countDown();
                    }
                });
            }
            assertTrue(latch.await(30, TimeUnit.SECONDS));
        } finally {
            pool.shutdownNow();
        }
        long expected = (long) threads * perThread;
        assertEquals(expected, metrics.getConnectionsTotal());
        assertEquals(expected, metrics.getSuccessCount());
        assertEquals(0, metrics.getConnectionsActive());
        assertEquals(1.0, metrics.getSuccessRate(), 1e-9);
    }
}
