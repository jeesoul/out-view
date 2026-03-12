package com.outview.integration;

import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.protocol.codec.MessageDecoder;
import com.outview.protocol.codec.MessageEncoder;
import io.netty.bootstrap.Bootstrap;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.channel.socket.nio.NioSocketChannel;
import org.junit.jupiter.api.*;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 性能测试
 * 测试系统在高负载下的表现
 */
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class PerformanceTest {

    private static final int TEST_PORT = 17996;
    private static final String TEST_HOST = "localhost";

    private static EventLoopGroup serverBossGroup;
    private static EventLoopGroup serverWorkerGroup;
    private static Channel serverChannel;

    @BeforeAll
    static void setupServer() throws Exception {
        serverBossGroup = new NioEventLoopGroup(1);
        serverWorkerGroup = new NioEventLoopGroup(8);

        ServerBootstrap bootstrap = new ServerBootstrap();
        bootstrap.group(serverBossGroup, serverWorkerGroup)
                .channel(NioServerSocketChannel.class)
                .option(ChannelOption.SO_BACKLOG, 1024)
                .childOption(ChannelOption.SO_KEEPALIVE, true)
                .childOption(ChannelOption.TCP_NODELAY, true)
                .childHandler(new ChannelInitializer<SocketChannel>() {
                    @Override
                    protected void initChannel(SocketChannel ch) {
                        ch.pipeline()
                                .addLast(new MessageDecoder())
                                .addLast(new MessageEncoder())
                                .addLast(new PerformanceTestServerHandler());
                    }
                });

        serverChannel = bootstrap.bind(TEST_PORT).sync().channel();
        System.out.println("Performance test server started on port: " + TEST_PORT);
    }

    @AfterAll
    static void teardownServer() {
        if (serverChannel != null) {
            serverChannel.close();
        }
        if (serverBossGroup != null) {
            serverBossGroup.shutdownGracefully();
        }
        if (serverWorkerGroup != null) {
            serverWorkerGroup.shutdownGracefully();
        }
        System.out.println("Performance test server stopped");
    }

    /**
     * 测试1: 吞吐量测试 - 消息处理速度
     */
    @Test
    @Order(1)
    void testMessageThroughput() throws Exception {
        int messageCount = 1000;
        CountDownLatch allResponses = new CountDownLatch(messageCount);
        AtomicInteger successCount = new AtomicInteger(0);
        AtomicLong totalLatency = new AtomicLong(0);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline()
                                    .addLast(new MessageDecoder())
                                    .addLast(new MessageEncoder())
                                    .addLast(new SimpleChannelInboundHandler<ProtocolMessage>() {
                                        private long sendTime;

                                        @Override
                                        public void channelActive(ChannelHandlerContext ctx) throws Exception {
                                            super.channelActive(ctx);
                                        }

                                        @Override
                                        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_HEARTBEAT_ACK) {
                                                long latency = System.nanoTime() - sendTime;
                                                totalLatency.addAndGet(latency);
                                                successCount.incrementAndGet();
                                                allResponses.countDown();
                                            }
                                        }

                                        public void setSendTime(long time) {
                                            this.sendTime = time;
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送大量心跳消息
            long startTime = System.nanoTime();
            for (int i = 0; i < messageCount; i++) {
                ProtocolMessage heartbeat = ProtocolMessage.heartbeat();
                channel.writeAndFlush(heartbeat);
            }

            boolean completed = allResponses.await(30, TimeUnit.SECONDS);
            long duration = System.nanoTime() - startTime;

            assertTrue(completed, "All messages should be processed within 30 seconds");

            double throughput = messageCount * 1_000_000_000.0 / duration;
            double avgLatencyMs = totalLatency.get() / (double) messageCount / 1_000_000.0;

            System.out.println("Throughput Test Results:");
            System.out.println("  Messages: " + messageCount);
            System.out.println("  Duration: " + duration / 1_000_000 + " ms");
            System.out.println("  Throughput: " + String.format("%.2f", throughput) + " msg/s");
            System.out.println("  Avg Latency: " + String.format("%.2f", avgLatencyMs) + " ms");

            assertTrue(throughput > 100, "Throughput should be > 100 msg/s");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试2: 数据传输吞吐量
     */
    @Test
    @Order(2)
    void testDataTransferThroughput() throws Exception {
        int dataSize = 1024; // 1KB
        int packetCount = 100;
        CountDownLatch allReceived = new CountDownLatch(packetCount);
        AtomicLong totalBytes = new AtomicLong(0);
        AtomicInteger receivedCount = new AtomicInteger(0);

        byte[] testData = new byte[dataSize];
        for (int i = 0; i < dataSize; i++) {
            testData[i] = (byte) (i % 256);
        }

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline()
                                    .addLast(new MessageDecoder())
                                    .addLast(new MessageEncoder())
                                    .addLast(new SimpleChannelInboundHandler<ProtocolMessage>() {
                                        @Override
                                        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_DATA) {
                                                totalBytes.addAndGet(msg.getBody().length);
                                                receivedCount.incrementAndGet();
                                                allReceived.countDown();
                                            }
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            long startTime = System.nanoTime();
            for (int i = 0; i < packetCount; i++) {
                ProtocolMessage dataMsg = ProtocolMessage.data(testData);
                channel.writeAndFlush(dataMsg);
            }

            boolean completed = allReceived.await(30, TimeUnit.SECONDS);
            long duration = System.nanoTime() - startTime;

            assertTrue(completed, "All data should be transferred within 30 seconds");

            double throughputMBps = (totalBytes.get() / 1024.0 / 1024.0) / (duration / 1_000_000_000.0);

            System.out.println("Data Transfer Test Results:");
            System.out.println("  Packets: " + packetCount);
            System.out.println("  Packet Size: " + dataSize + " bytes");
            System.out.println("  Total Data: " + totalBytes.get() / 1024 + " KB");
            System.out.println("  Duration: " + duration / 1_000_000 + " ms");
            System.out.println("  Throughput: " + String.format("%.2f", throughputMBps) + " MB/s");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试3: 连接建立速度
     */
    @Test
    @Order(3)
    void testConnectionEstablishmentSpeed() throws Exception {
        int connectionCount = 50;
        List<Long> connectionTimes = new CopyOnWriteArrayList<>();
        ExecutorService executor = Executors.newFixedThreadPool(10);
        CountDownLatch allDone = new CountDownLatch(connectionCount);
        List<EventLoopGroup> groups = new CopyOnWriteArrayList<>();

        try {
            long testStart = System.nanoTime();

            for (int i = 0; i < connectionCount; i++) {
                executor.submit(() -> {
                    long startTime = System.nanoTime();
                    EventLoopGroup group = new NioEventLoopGroup();
                    groups.add(group);

                    try {
                        Bootstrap bootstrap = new Bootstrap();
                        bootstrap.group(group)
                                .channel(NioSocketChannel.class)
                                .handler(new ChannelInitializer<SocketChannel>() {
                                    @Override
                                    protected void initChannel(SocketChannel ch) {
                                        ch.pipeline()
                                                .addLast(new MessageDecoder())
                                                .addLast(new MessageEncoder());
                                    }
                                });

                        Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

                        long connectionTime = System.nanoTime() - startTime;
                        connectionTimes.add(connectionTime);

                        // 发送注册并等待响应
                        ProtocolMessage register = ProtocolMessage.register("speed-test-device", "token", 3389);
                        channel.writeAndFlush(register);

                        Thread.sleep(100);
                        channel.close();

                    } catch (Exception e) {
                        e.printStackTrace();
                    } finally {
                        allDone.countDown();
                    }
                });
            }

            boolean completed = allDone.await(60, TimeUnit.SECONDS);
            long totalTestTime = System.nanoTime() - testStart;

            assertTrue(completed, "All connections should complete within 60 seconds");

            // 计算统计数据
            double avgConnectionTime = connectionTimes.stream()
                    .mapToLong(Long::longValue)
                    .average()
                    .orElse(0) / 1_000_000.0;

            double maxConnectionTime = connectionTimes.stream()
                    .mapToLong(Long::longValue)
                    .max()
                    .orElse(0) / 1_000_000.0;

            double minConnectionTime = connectionTimes.stream()
                    .mapToLong(Long::longValue)
                    .min()
                    .orElse(0) / 1_000_000.0;

            System.out.println("Connection Speed Test Results:");
            System.out.println("  Total Connections: " + connectionCount);
            System.out.println("  Total Time: " + totalTestTime / 1_000_000 + " ms");
            System.out.println("  Avg Connection Time: " + String.format("%.2f", avgConnectionTime) + " ms");
            System.out.println("  Min Connection Time: " + String.format("%.2f", minConnectionTime) + " ms");
            System.out.println("  Max Connection Time: " + String.format("%.2f", maxConnectionTime) + " ms");

            assertTrue(avgConnectionTime < 1000, "Average connection time should be < 1 second");

        } finally {
            for (EventLoopGroup group : groups) {
                group.shutdownGracefully();
            }
            executor.shutdown();
        }
    }

    /**
     * 测试4: 并发用户容量测试
     */
    @Test
    @Order(4)
    @Disabled("Run manually for capacity testing")
    void testConcurrentUserCapacity() throws Exception {
        int targetConnections = 100;
        AtomicInteger activeConnections = new AtomicInteger(0);
        AtomicInteger failedConnections = new AtomicInteger(0);
        List<Channel> channels = new CopyOnWriteArrayList<>();
        List<EventLoopGroup> groups = new CopyOnWriteArrayList<>();
        ExecutorService executor = Executors.newFixedThreadPool(20);

        try {
            CountDownLatch allDone = new CountDownLatch(targetConnections);

            for (int i = 0; i < targetConnections; i++) {
                final int index = i;
                executor.submit(() -> {
                    EventLoopGroup group = new NioEventLoopGroup();
                    groups.add(group);

                    try {
                        Bootstrap bootstrap = new Bootstrap();
                        bootstrap.group(group)
                                .channel(NioSocketChannel.class)
                                .handler(new ChannelInitializer<SocketChannel>() {
                                    @Override
                                    protected void initChannel(SocketChannel ch) {
                                        ch.pipeline()
                                                .addLast(new MessageDecoder())
                                                .addLast(new MessageEncoder())
                                                .addLast(new SimpleChannelInboundHandler<ProtocolMessage>() {
                                                    @Override
                                                    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
                                                        // 处理消息
                                                    }
                                                });
                                    }
                                });

                        Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
                        channels.add(channel);
                        activeConnections.incrementAndGet();

                        // 发送注册
                        String deviceId = "capacity-device-" + index;
                        ProtocolMessage register = ProtocolMessage.register(deviceId, "token", 3389);
                        channel.writeAndFlush(register);

                    } catch (Exception e) {
                        failedConnections.incrementAndGet();
                    } finally {
                        allDone.countDown();
                    }
                });
            }

            boolean completed = allDone.await(120, TimeUnit.SECONDS);

            System.out.println("Capacity Test Results:");
            System.out.println("  Target Connections: " + targetConnections);
            System.out.println("  Active Connections: " + activeConnections.get());
            System.out.println("  Failed Connections: " + failedConnections.get());
            System.out.println("  Success Rate: " + (activeConnections.get() * 100.0 / targetConnections) + "%");

            // 保持连接一段时间
            Thread.sleep(5000);

            // 验证连接仍然活跃
            int stillActive = 0;
            for (Channel ch : channels) {
                if (ch.isActive()) {
                    stillActive++;
                }
            }
            System.out.println("  Still Active After 5s: " + stillActive);

            assertTrue(completed, "All connection attempts should complete");
            assertTrue(activeConnections.get() >= targetConnections * 0.9, "At least 90% connections should succeed");

        } finally {
            for (Channel ch : channels) {
                if (ch.isActive()) {
                    ch.close();
                }
            }
            for (EventLoopGroup group : groups) {
                group.shutdownGracefully();
            }
            executor.shutdown();
        }
    }

    /**
     * 测试5: 内存稳定性测试
     */
    @Test
    @Order(5)
    void testMemoryStability() throws Exception {
        Runtime runtime = Runtime.getRuntime();
        long initialMemory = runtime.totalMemory() - runtime.freeMemory();

        int iterations = 1000;
        CountDownLatch latch = new CountDownLatch(iterations);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline()
                                    .addLast(new MessageDecoder())
                                    .addLast(new MessageEncoder())
                                    .addLast(new SimpleChannelInboundHandler<ProtocolMessage>() {
                                        @Override
                                        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
                                            latch.countDown();
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送大量消息
            for (int i = 0; i < iterations; i++) {
                byte[] data = new byte[1024];
                ProtocolMessage dataMsg = ProtocolMessage.data(data);
                channel.writeAndFlush(dataMsg);
            }

            boolean completed = latch.await(30, TimeUnit.SECONDS);
            assertTrue(completed, "All messages should be processed");

            // 强制 GC
            System.gc();
            Thread.sleep(1000);

            long finalMemory = runtime.totalMemory() - runtime.freeMemory();
            long memoryIncrease = finalMemory - initialMemory;

            System.out.println("Memory Stability Test Results:");
            System.out.println("  Initial Memory: " + initialMemory / 1024 / 1024 + " MB");
            System.out.println("  Final Memory: " + finalMemory / 1024 / 1024 + " MB");
            System.out.println("  Memory Increase: " + memoryIncrease / 1024 / 1024 + " MB");
            System.out.println("  Messages Processed: " + iterations);

            // 内存增长不应过大 (< 50MB)
            assertTrue(memoryIncrease < 50 * 1024 * 1024, "Memory increase should be reasonable");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 性能测试服务端处理器
     */
    static class PerformanceTestServerHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        private static AtomicInteger portAllocator = new AtomicInteger(8000);
        private static AtomicInteger connectionCounter = new AtomicInteger(0);
        private static AtomicLong totalMessages = new AtomicLong(0);
        private static AtomicLong totalBytes = new AtomicLong(0);

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
            totalMessages.incrementAndGet();
            byte type = msg.getHeader().getType();

            switch (type) {
                case ProtocolConstants.TYPE_REGISTER:
                    String deviceId = "perf-device-" + connectionCounter.incrementAndGet();
                    int port = portAllocator.incrementAndGet();

                    String registerAck = "{\"success\":true,\"deviceId\":\"" + deviceId + "\",\"externalPort\":" + port + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_REGISTER_ACK, registerAck);
                    break;

                case ProtocolConstants.TYPE_HEARTBEAT:
                    String heartbeatAck = "{\"success\":true,\"timestamp\":" + System.currentTimeMillis() + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_HEARTBEAT_ACK, heartbeatAck);
                    break;

                case ProtocolConstants.TYPE_DATA:
                    totalBytes.addAndGet(msg.getBody().length);
                    // 回显数据
                    ProtocolMessage dataAck = ProtocolMessage.data(msg.getBody());
                    ctx.writeAndFlush(dataAck);
                    break;

                default:
                    break;
            }
        }

        private void sendResponse(ChannelHandlerContext ctx, byte type, String body) {
            ProtocolMessage response = ProtocolMessage.builder()
                    .header(com.outview.protocol.MessageHeader.builder()
                            .magic(ProtocolConstants.MAGIC_NUMBER)
                            .version(ProtocolConstants.VERSION)
                            .type(type)
                            .length(body.length())
                            .build())
                    .body(body.getBytes())
                    .build();
            ctx.writeAndFlush(response);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            ctx.close();
        }
    }
}