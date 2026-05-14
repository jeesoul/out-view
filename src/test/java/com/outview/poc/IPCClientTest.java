package com.outview.poc;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

import java.io.*;
import java.net.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Unit tests for IPCClient protocol helpers.
 * Uses an embedded Java TCP server to avoid requiring the Go server.
 */
class IPCClientTest {

    private ServerSocket serverSocket;
    private Thread serverThread;
    private int serverPort;

    @BeforeEach
    void startEchoServer() throws Exception {
        serverSocket = new ServerSocket(0); // bind to random port
        serverPort = serverSocket.getLocalPort();

        serverThread = new Thread(() -> {
            while (!serverSocket.isClosed()) {
                try {
                    Socket client = serverSocket.accept();
                    new Thread(() -> handleClient(client)).start();
                } catch (IOException e) {
                    if (!serverSocket.isClosed()) {
                        e.printStackTrace();
                    }
                }
            }
        });
        serverThread.setDaemon(true);
        serverThread.start();
    }

    @AfterEach
    void stopEchoServer() throws Exception {
        serverSocket.close();
    }

    /** Echo server: reads a ping, responds with pong. */
    private void handleClient(Socket client) {
        try (Socket c = client) {
            IPCClient.readMessage(c.getInputStream()); // consume request
            // Build a simple pong response
            String pong = "{\"type\":\"pong\",\"payload\":{\"timestamp\":0,"
                    + "\"echo_message\":\"test\",\"server_time\":0}}";
            IPCClient.writeMessage(c.getOutputStream(), pong);
        } catch (IOException e) {
            // ignore
        }
    }

    @Test
    void testWriteReadRoundtrip() throws Exception {
        // Write a message to a pipe and read it back
        PipedOutputStream pos = new PipedOutputStream();
        PipedInputStream pis = new PipedInputStream(pos);

        String original = "{\"type\":\"ping\",\"payload\":{\"timestamp\":12345,\"message\":\"hello\"}}";
        IPCClient.writeMessage(pos, original);
        pos.close();

        String result = IPCClient.readMessage(pis);
        assertEquals(original, result);
    }

    @Test
    void testPingPongWithEchoServer() throws Exception {
        try (Socket socket = new Socket("127.0.0.1", serverPort)) {
            socket.setSoTimeout(5000);
            String ping = "{\"type\":\"ping\",\"payload\":{\"timestamp\":1000,\"message\":\"test\"}}";
            IPCClient.writeMessage(socket.getOutputStream(), ping);
            String response = IPCClient.readMessage(socket.getInputStream());
            assertTrue(response.contains("\"pong\""), "Expected pong in response, got: " + response);
        }
    }

    @Test
    void testConcurrent100WithEchoServer() throws Exception {
        int numConns = 100;
        ExecutorService executor = Executors.newFixedThreadPool(numConns);
        CountDownLatch startLatch = new CountDownLatch(1);
        CountDownLatch doneLatch = new CountDownLatch(numConns);
        AtomicInteger successCount = new AtomicInteger(0);
        AtomicInteger errorCount = new AtomicInteger(0);

        for (int i = 0; i < numConns; i++) {
            final int id = i;
            executor.submit(() -> {
                try {
                    startLatch.await();
                    try (Socket socket = new Socket("127.0.0.1", serverPort)) {
                        socket.setSoTimeout(5000);
                        String ping = "{\"type\":\"ping\",\"payload\":{\"timestamp\":"
                                + System.currentTimeMillis() + ",\"message\":\"concurrent-" + id + "\"}}";
                        IPCClient.writeMessage(socket.getOutputStream(), ping);
                        String response = IPCClient.readMessage(socket.getInputStream());
                        if (response.contains("\"pong\"")) {
                            successCount.incrementAndGet();
                        } else {
                            errorCount.incrementAndGet();
                        }
                    }
                } catch (Exception e) {
                    errorCount.incrementAndGet();
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        startLatch.countDown();
        boolean finished = doneLatch.await(30, TimeUnit.SECONDS);
        executor.shutdown();

        assertTrue(finished, "Test timed out after 30s");
        assertEquals(numConns, successCount.get(), "All connections should succeed");
        assertEquals(0, errorCount.get(), "No errors expected");
    }

    @Test
    void testLargeMessage() throws Exception {
        // Test with a large message (10KB payload)
        StringBuilder sb = new StringBuilder("{\"type\":\"ping\",\"payload\":{\"timestamp\":0,\"message\":\"");
        for (int i = 0; i < 1000; i++) {
            sb.append("abcdefghij");
        }
        sb.append("\"}}");
        String largeMsg = sb.toString();

        PipedOutputStream pos = new PipedOutputStream();
        PipedInputStream pis = new PipedInputStream(pos, largeMsg.length() + 8);

        IPCClient.writeMessage(pos, largeMsg);
        pos.close();

        String result = IPCClient.readMessage(pis);
        assertEquals(largeMsg, result);
    }
}
