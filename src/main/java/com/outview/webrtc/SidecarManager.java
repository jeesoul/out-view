package com.outview.webrtc;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.DisposableBean;
import org.springframework.stereotype.Component;

import javax.annotation.PreDestroy;
import java.io.IOException;
import java.io.InputStream;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Manages the lifecycle of the WebRTC Sidecar process.
 * Starts the sidecar binary, monitors health via IPC ping, and auto-restarts on failure.
 */
@Component
public class SidecarManager implements DisposableBean {
    private static final Logger log = LoggerFactory.getLogger(SidecarManager.class);
    private static final int MAX_RESTARTS = 3;
    private static final long BASE_RESTART_DELAY_MS = 1000; // 1s, doubles each retry

    private final WebRTCConfig config;
    private final WebRTCProxyService proxyService;

    private Process sidecarProcess;
    private Thread stdoutThread;
    private Thread stderrThread;
    private ScheduledExecutorService scheduler;
    private ScheduledFuture<?> healthCheckFuture;

    private final AtomicBoolean started = new AtomicBoolean(false);
    private final AtomicBoolean stopping = new AtomicBoolean(false);
    private final AtomicInteger restartCount = new AtomicInteger(0);

    public SidecarManager(WebRTCConfig config, WebRTCProxyService proxyService) {
        this.config = config;
        this.proxyService = proxyService;
    }

    /**
     * Start the sidecar process and connect to it.
     */
    public synchronized void start() throws IOException {
        if (started.get()) {
            log.warn("Sidecar already started");
            return;
        }
        if (!config.isEnabled()) {
            log.info("WebRTC disabled, not starting sidecar");
            return;
        }

        doStart();
        started.set(true);

        // Start health check every 10 seconds
        scheduler = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread t = new Thread(r, "sidecar-health");
            t.setDaemon(true);
            return t;
        });
        healthCheckFuture = scheduler.scheduleAtFixedRate(
            this::healthCheck, 10, 10, TimeUnit.SECONDS);
    }

    private void doStart() throws IOException {
        String binaryPath = config.getSidecarBinaryPath();
        String socketPath = config.getSidecarSocketPath();

        log.info("Starting sidecar: {} -socket {}", binaryPath, socketPath);

        ProcessBuilder pb = new ProcessBuilder(binaryPath, "-socket", socketPath);
        pb.environment().put("GOGC", "100");
        sidecarProcess = pb.start();

        // Redirect stdout/stderr to SLF4J
        stdoutThread = new Thread(
            new StreamLogger(sidecarProcess.getInputStream(), "sidecar-stdout"),
            "sidecar-stdout-logger");
        stdoutThread.setDaemon(true);
        stdoutThread.start();

        stderrThread = new Thread(
            new StreamLogger(sidecarProcess.getErrorStream(), "sidecar-stderr"),
            "sidecar-stderr-logger");
        stderrThread.setDaemon(true);
        stderrThread.start();

        // Wait briefly for socket to be ready, then connect
        try { Thread.sleep(500); } catch (InterruptedException e) { Thread.currentThread().interrupt(); }

        connectWithRetry();
    }

    private void connectWithRetry() throws IOException {
        int attempts = 3;
        IOException lastEx = null;
        for (int i = 0; i < attempts; i++) {
            try {
                proxyService.connect();
                log.info("Connected to sidecar IPC");
                return;
            } catch (IOException e) {
                lastEx = e;
                log.warn("IPC connect attempt {} failed: {}", i + 1, e.getMessage());
                try { Thread.sleep(500); } catch (InterruptedException ie) { Thread.currentThread().interrupt(); }
            }
        }
        throw new IOException("Failed to connect to sidecar after " + attempts + " attempts", lastEx);
    }

    private void healthCheck() {
        if (stopping.get()) return;

        // Check if process is still alive
        if (sidecarProcess != null && !sidecarProcess.isAlive()) {
            log.error("Sidecar process died unexpectedly");
            handleProcessDeath();
            return;
        }

        // Check IPC connectivity
        if (!proxyService.isConnected()) {
            log.warn("Sidecar IPC connection lost");
            handleProcessDeath();
        }
    }

    private void handleProcessDeath() {
        if (stopping.get()) return;

        int count = restartCount.incrementAndGet();
        if (count > MAX_RESTARTS) {
            log.error("Sidecar failed {} times, giving up", count);
            return;
        }

        long delayMs = BASE_RESTART_DELAY_MS * (1L << (count - 1)); // exponential backoff
        log.info("Restarting sidecar (attempt {}/{}) in {}ms", count, MAX_RESTARTS, delayMs);

        scheduler.schedule(() -> {
            try {
                killProcess();
                proxyService.disconnect();
                doStart();
                log.info("Sidecar restarted successfully");
            } catch (IOException e) {
                log.error("Failed to restart sidecar", e);
                handleProcessDeath(); // recurse for next retry
            }
        }, delayMs, TimeUnit.MILLISECONDS);
    }

    private void killProcess() {
        if (sidecarProcess != null && sidecarProcess.isAlive()) {
            sidecarProcess.destroy();
            try {
                sidecarProcess.waitFor(5, TimeUnit.SECONDS);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
            if (sidecarProcess.isAlive()) {
                sidecarProcess.destroyForcibly();
            }
        }
    }

    /**
     * Stop the sidecar process gracefully.
     */
    @PreDestroy
    @Override
    public void destroy() {
        if (!stopping.compareAndSet(false, true)) return;

        log.info("Stopping sidecar...");

        if (healthCheckFuture != null) {
            healthCheckFuture.cancel(false);
        }
        if (scheduler != null) {
            scheduler.shutdown();
            try {
                scheduler.awaitTermination(5, TimeUnit.SECONDS);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }

        proxyService.disconnect();
        killProcess();

        log.info("Sidecar stopped");
    }

    public boolean isRunning() {
        return started.get() && !stopping.get()
            && sidecarProcess != null && sidecarProcess.isAlive();
    }

    public int getRestartCount() {
        return restartCount.get();
    }

    /**
     * Reads an InputStream line by line and logs each line.
     */
    private static class StreamLogger implements Runnable {
        private static final Logger log = LoggerFactory.getLogger(SidecarManager.class);
        private final InputStream stream;
        private final String name;

        StreamLogger(InputStream stream, String name) {
            this.stream = stream;
            this.name = name;
        }

        @Override
        public void run() {
            try (java.io.BufferedReader reader = new java.io.BufferedReader(
                    new java.io.InputStreamReader(stream))) {
                String line;
                while ((line = reader.readLine()) != null) {
                    log.info("[{}] {}", name, line);
                }
            } catch (IOException e) {
                log.debug("{} reader closed: {}", name, e.getMessage());
            }
        }
    }
}
