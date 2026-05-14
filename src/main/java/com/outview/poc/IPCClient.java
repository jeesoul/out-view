package com.outview.poc;

import org.newsclub.net.unix.AFUNIXSocket;
import org.newsclub.net.unix.AFUNIXSocketAddress;

import java.io.*;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.logging.Logger;

/**
 * IPCClient - Java IPC client for WebRTC Sidecar POC validation.
 *
 * <p>Protocol: [4 bytes big-endian length][JSON payload]
 * Message format: {"type": "...", "payload": {...}}
 *
 * <p>On Windows, uses TCP localhost as fallback (Unix sockets require Windows 10 1803+).
 * On Linux/macOS, uses Unix Domain Socket via junixsocket.
 *
 * <p>Usage:
 * <pre>
 *   # Start Go server first:
 *   cd webrtc-sidecar && go run cmd/poc/main.go
 *
 *   # Then run Java client:
 *   mvn exec:java -Dexec.mainClass=com.outview.poc.IPCClient
 * </pre>
 */
public class IPCClient {

    private static final Logger log = Logger.getLogger(IPCClient.class.getName());

    /** Default TCP address for Windows fallback. */
    private static final String DEFAULT_TCP_HOST = "127.0.0.1";
    private static final int DEFAULT_TCP_PORT = 9999;

    /** Default Unix socket path for Linux/macOS. */
    private static final String DEFAULT_SOCKET_PATH = "/tmp/outview-ipc.sock";

    /** Number of concurrent connections for stress test. */
    private static final int CONCURRENT_CONNECTIONS = 100;

    public static void main(String[] args) throws Exception {
        String host = System.getProperty("ipc.host", DEFAULT_TCP_HOST);
        int port = Integer.parseInt(System.getProperty("ipc.port", String.valueOf(DEFAULT_TCP_PORT)));
        String socketPath = System.getProperty("ipc.socket", "");

        boolean useUnix = !socketPath.isEmpty() && !isWindows();

        log.info("=== WebRTC Sidecar IPC Client POC ===");
        if (useUnix) {
            log.info("Transport: Unix Domain Socket -> " + socketPath);
        } else {
            log.info("Transport: TCP -> " + host + ":" + port);
        }

        // Test 1: Basic ping-pong
        log.info("\n--- Test 1: Basic Ping-Pong ---");
        testPingPong(host, port, socketPath, useUnix);

        // Test 2: Multiple sequential messages
        log.info("\n--- Test 2: Sequential Messages (10x) ---");
        testSequential(host, port, socketPath, useUnix, 10);

        // Test 3: 100 concurrent connections
        log.info("\n--- Test 3: 100 Concurrent Connections ---");
        testConcurrent(host, port, socketPath, useUnix, CONCURRENT_CONNECTIONS);

        log.info("\n=== All tests completed ===");
    }

    // -------------------------------------------------------------------------
    // Test methods
    // -------------------------------------------------------------------------

    private static void testPingPong(String host, int port, String socketPath, boolean useUnix)
            throws Exception {
        try (Socket socket = connect(host, port, socketPath, useUnix)) {
            long ts = System.currentTimeMillis();
            String pingJson = buildPingJson(ts, "hello from Java");
            writeMessage(socket.getOutputStream(), pingJson);

            String response = readMessage(socket.getInputStream());
            log.info("Sent:     " + pingJson);
            log.info("Received: " + response);

            if (!response.contains("\"pong\"")) {
                throw new AssertionError("Expected pong response, got: " + response);
            }
            log.info("PASS: Ping-pong OK");
        }
    }

    private static void testSequential(String host, int port, String socketPath, boolean useUnix,
            int count) throws Exception {
        try (Socket socket = connect(host, port, socketPath, useUnix)) {
            for (int i = 0; i < count; i++) {
                long ts = System.currentTimeMillis();
                String pingJson = buildPingJson(ts, "sequential-" + i);
                writeMessage(socket.getOutputStream(), pingJson);

                String response = readMessage(socket.getInputStream());
                if (!response.contains("\"pong\"")) {
                    throw new AssertionError("Message " + i + ": expected pong, got: " + response);
                }
            }
            log.info("PASS: " + count + " sequential messages OK");
        }
    }

    private static void testConcurrent(String host, int port, String socketPath, boolean useUnix,
            int numConnections) throws Exception {
        ExecutorService executor = Executors.newFixedThreadPool(numConnections);
        CountDownLatch startLatch = new CountDownLatch(1);
        CountDownLatch doneLatch = new CountDownLatch(numConnections);
        AtomicInteger successCount = new AtomicInteger(0);
        AtomicInteger errorCount = new AtomicInteger(0);
        List<String> errors = new CopyOnWriteArrayList<>();

        for (int i = 0; i < numConnections; i++) {
            final int id = i;
            executor.submit(() -> {
                try {
                    startLatch.await(); // wait for all threads to be ready
                    try (Socket socket = connect(host, port, socketPath, useUnix)) {
                        long ts = System.currentTimeMillis();
                        String pingJson = buildPingJson(ts, "concurrent-" + id);
                        writeMessage(socket.getOutputStream(), pingJson);

                        String response = readMessage(socket.getInputStream());
                        if (!response.contains("\"pong\"")) {
                            errors.add("Thread " + id + ": expected pong, got: " + response);
                            errorCount.incrementAndGet();
                        } else {
                            successCount.incrementAndGet();
                        }
                    }
                } catch (Exception e) {
                    errors.add("Thread " + id + ": " + e.getMessage());
                    errorCount.incrementAndGet();
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        long startTime = System.currentTimeMillis();
        startLatch.countDown(); // release all threads simultaneously
        boolean finished = doneLatch.await(30, TimeUnit.SECONDS);
        long elapsed = System.currentTimeMillis() - startTime;

        executor.shutdown();

        if (!finished) {
            throw new AssertionError("Concurrent test timed out after 30s");
        }

        log.info(String.format("Results: %d OK, %d errors, elapsed %dms",
                successCount.get(), errorCount.get(), elapsed));

        if (!errors.isEmpty()) {
            for (String err : errors.subList(0, Math.min(5, errors.size()))) {
                log.warning("  Error: " + err);
            }
        }

        if (errorCount.get() > 0) {
            throw new AssertionError(
                    "Concurrent test: " + errorCount.get() + " errors out of " + numConnections);
        }
        log.info("PASS: " + numConnections + " concurrent connections OK");
    }

    // -------------------------------------------------------------------------
    // IPC protocol helpers
    // -------------------------------------------------------------------------

    /**
     * Writes a length-prefixed message to the output stream.
     * Format: [4 bytes big-endian length][UTF-8 JSON payload]
     */
    static void writeMessage(OutputStream out, String json) throws IOException {
        byte[] payload = json.getBytes(StandardCharsets.UTF_8);
        ByteBuffer header = ByteBuffer.allocate(4);
        header.putInt(payload.length);
        out.write(header.array());
        out.write(payload);
        out.flush();
    }

    /**
     * Reads a length-prefixed message from the input stream.
     * Format: [4 bytes big-endian length][UTF-8 JSON payload]
     */
    static String readMessage(InputStream in) throws IOException {
        byte[] header = readFully(in, 4);
        int length = ByteBuffer.wrap(header).getInt();
        if (length <= 0 || length > 4 * 1024 * 1024) {
            throw new IOException("Invalid message length: " + length);
        }
        byte[] payload = readFully(in, length);
        return new String(payload, StandardCharsets.UTF_8);
    }

    private static byte[] readFully(InputStream in, int length) throws IOException {
        byte[] buf = new byte[length];
        int offset = 0;
        while (offset < length) {
            int read = in.read(buf, offset, length - offset);
            if (read == -1) {
                throw new EOFException("Stream closed after " + offset + " of " + length + " bytes");
            }
            offset += read;
        }
        return buf;
    }

    // -------------------------------------------------------------------------
    // Connection helpers
    // -------------------------------------------------------------------------

    /**
     * Creates a socket connection. Uses Unix socket on Linux/macOS, TCP on Windows.
     */
    private static Socket connect(String host, int port, String socketPath, boolean useUnix)
            throws IOException {
        if (useUnix) {
            return connectUnix(socketPath);
        } else {
            return connectTCP(host, port);
        }
    }

    private static Socket connectTCP(String host, int port) throws IOException {
        Socket socket = new Socket();
        socket.connect(new InetSocketAddress(host, port), 5000);
        socket.setSoTimeout(10000);
        return socket;
    }

    /**
     * Connects via Unix Domain Socket using junixsocket (direct import, not reflection).
     * Use {@link AFUNIXSocket#connectTo(AFUNIXSocketAddress)} as the single-call factory.
     */
    static Socket connectUnix(String socketPath) throws IOException {
        AFUNIXSocketAddress address = AFUNIXSocketAddress.of(new File(socketPath));
        AFUNIXSocket socket = AFUNIXSocket.connectTo(address);
        socket.setSoTimeout(10000);
        return socket;
    }

    // -------------------------------------------------------------------------
    // JSON helpers (no external dependency - hand-rolled for POC)
    // -------------------------------------------------------------------------

    private static String buildPingJson(long timestamp, String message) {
        return String.format(
                "{\"type\":\"ping\",\"payload\":{\"timestamp\":%d,\"message\":\"%s\"}}",
                timestamp, escapeJson(message));
    }

    private static String escapeJson(String s) {
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static boolean isWindows() {
        return System.getProperty("os.name", "").toLowerCase().contains("win");
    }
}
