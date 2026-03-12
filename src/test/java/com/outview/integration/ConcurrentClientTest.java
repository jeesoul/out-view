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
import java.util.Map;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 多客户端并发测试
 * 测试服务端处理多个客户端同时连接的能力
 */
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class ConcurrentClientTest {

    private static final int TEST_PORT = 17998;
    private static final String TEST_HOST = "localhost";
    private static final int MAGIC_NUMBER = 0x4F565753;

    private static EventLoopGroup serverBossGroup;
    private static EventLoopGroup serverWorkerGroup;
    private static Channel serverChannel;

    private static AtomicInteger connectionCount = new AtomicInteger(0);
    private static AtomicInteger registerCount = new AtomicInteger(0);
    private static Map<String, Integer> devicePortMap = new ConcurrentHashMap<>();

    @BeforeAll
    static void setupServer() throws Exception {
        serverBossGroup = new NioEventLoopGroup(1);
        serverWorkerGroup = new NioEventLoopGroup(4);

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
                                .addLast(new ConcurrentTestServerHandler());
                    }
                });

        serverChannel = bootstrap.bind(TEST_PORT).sync().channel();
        System.out.println("Concurrent test server started on port: " + TEST_PORT);
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
        System.out.println("Concurrent test server stopped");
    }

    /**
     * 测试1: 10个客户端并发连接
     */
    @Test
    @Order(1)
    void testTenConcurrentConnections() throws Exception {
        int clientCount = 10;
        CountDownLatch allConnected = new CountDownLatch(clientCount);
        CountDownLatch allRegistered = new CountDownLatch(clientCount);
        ExecutorService executor = Executors.newFixedThreadPool(clientCount);
        List<Channel> channels = new ArrayList<>();
        List<EventLoopGroup> groups = new ArrayList<>();

        try {
            // 并发启动客户端
            for (int i = 0; i < clientCount; i++) {
                final int index = i;
                executor.submit(() -> {
                    try {
                        EventLoopGroup group = new NioEventLoopGroup();
                        groups.add(group);

                        Bootstrap bootstrap = new Bootstrap();
                        bootstrap.group(group)
                                .channel(NioSocketChannel.class)
                                .option(ChannelOption.TCP_NODELAY, true)
                                .handler(new ChannelInitializer<SocketChannel>() {
                                    @Override
                                    protected void initChannel(SocketChannel ch) {
                                        ch.pipeline()
                                                .addLast(new MessageDecoder())
                                                .addLast(new MessageEncoder())
                                                .addLast(new SimpleChannelInboundHandler<ProtocolMessage>() {
                                                    @Override
                                                    protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
                                                        if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                                                            allRegistered.countDown();
                                                        }
                                                    }
                                                });
                                    }
                                });

                        ChannelFuture future = bootstrap.connect(TEST_HOST, TEST_PORT).sync();
                        Channel channel = future.channel();
                        channels.add(channel);
                        allConnected.countDown();

                        // 发送注册消息
                        String deviceId = "concurrent-device-" + index;
                        ProtocolMessage register = ProtocolMessage.register(deviceId, "token-" + index, 3389);
                        channel.writeAndFlush(register);

                    } catch (Exception e) {
                        e.printStackTrace();
                    }
                });
            }

            // 等待所有客户端连接
            boolean connected = allConnected.await(10, TimeUnit.SECONDS);
            assertTrue(connected, "All clients should connect within 10 seconds");

            // 等待所有注册完成
            boolean registered = allRegistered.await(15, TimeUnit.SECONDS);
            assertTrue(registered, "All clients should register within 15 seconds");

            System.out.println("Successfully connected and registered " + clientCount + " clients");

        } finally {
            // 关闭所有客户端
            for (Channel channel : channels) {
                if (channel != null && channel.isActive()) {
                    channel.close();
                }
            }
            for (EventLoopGroup group : groups) {
                group.shutdownGracefully();
            }
            executor.shutdown();
        }
    }

    /**
     * 测试2: 50个客户端并发连接（压力测试）
     */
    @Test
    @Order(2)
    @Disabled("Run manually for stress testing")
    void testFiftyConcurrentConnections() throws Exception {
        int clientCount = 50;
        CountDownLatch allRegistered = new CountDownLatch(clientCount);
        AtomicInteger successCount = new AtomicInteger(0);
        AtomicInteger failCount = new AtomicInteger(0);
        ExecutorService executor = Executors.newFixedThreadPool(20);
        List<EventLoopGroup> groups = new CopyOnWriteArrayList<>();

        try {
            long startTime = System.currentTimeMillis();

            for (int i = 0; i < clientCount; i++) {
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
                                                        if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                                                            successCount.incrementAndGet();
                                                            allRegistered.countDown();
                                                        }
                                                    }

                                                    @Override
                                                    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
                                                        failCount.incrementAndGet();
                                                        allRegistered.countDown();
                                                    }
                                                });
                                    }
                                });

                        Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

                        String deviceId = "stress-device-" + index;
                        ProtocolMessage register = ProtocolMessage.register(deviceId, "token-" + index, 3389);
                        channel.writeAndFlush(register);

                        // 保持连接一段时间
                        Thread.sleep(100);

                        channel.close();

                    } catch (Exception e) {
                        failCount.incrementAndGet();
                        allRegistered.countDown();
                    }
                });
            }

            boolean completed = allRegistered.await(60, TimeUnit.SECONDS);
            long duration = System.currentTimeMillis() - startTime;

            System.out.println("Stress test completed in " + duration + "ms");
            System.out.println("Success: " + successCount.get() + ", Failed: " + failCount.get());

            assertTrue(completed, "All operations should complete within 60 seconds");
            assertTrue(successCount.get() >= clientCount * 0.9, "At least 90% should succeed");

        } finally {
            for (EventLoopGroup group : groups) {
                group.shutdownGracefully();
            }
            executor.shutdown();
        }
    }

    /**
     * 测试3: 并发心跳测试
     */
    @Test
    @Order(3)
    void testConcurrentHeartbeats() throws Exception {
        int clientCount = 5;
        int heartbeatPerClient = 3;
        CountDownLatch allHeartbeats = new CountDownLatch(clientCount * heartbeatPerClient);
        ExecutorService executor = Executors.newFixedThreadPool(clientCount);
        List<Channel> channels = new ArrayList<>();
        List<EventLoopGroup> groups = new ArrayList<>();

        try {
            // 启动客户端
            for (int i = 0; i < clientCount; i++) {
                final int index = i;
                EventLoopGroup group = new NioEventLoopGroup();
                groups.add(group);

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
                                                if (msg.getHeader().getType() == ProtocolConstants.TYPE_HEARTBEAT_ACK) {
                                                    allHeartbeats.countDown();
                                                }
                                            }
                                        });
                            }
                        });

                Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
                channels.add(channel);
            }

            // 并发发送心跳
            for (int i = 0; i < clientCount; i++) {
                final Channel channel = channels.get(i);
                executor.submit(() -> {
                    for (int j = 0; j < heartbeatPerClient; j++) {
                        ProtocolMessage heartbeat = ProtocolMessage.heartbeat();
                        channel.writeAndFlush(heartbeat);
                        try {
                            Thread.sleep(100);
                        } catch (InterruptedException e) {
                            Thread.currentThread().interrupt();
                        }
                    }
                });
            }

            boolean completed = allHeartbeats.await(10, TimeUnit.SECONDS);
            assertTrue(completed, "All heartbeats should be processed within 10 seconds");

            System.out.println("All " + (clientCount * heartbeatPerClient) + " heartbeats processed successfully");

        } finally {
            for (Channel channel : channels) {
                if (channel != null && channel.isActive()) {
                    channel.close();
                }
            }
            for (EventLoopGroup group : groups) {
                group.shutdownGracefully();
            }
            executor.shutdown();
        }
    }

    /**
     * 并发测试服务端处理器
     */
    static class ConcurrentTestServerHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        private static AtomicInteger portAllocator = new AtomicInteger(6000);

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
            byte type = msg.getHeader().getType();

            switch (type) {
                case ProtocolConstants.TYPE_REGISTER:
                    connectionCount.incrementAndGet();
                    int registerNum = registerCount.incrementAndGet();

                    // 解析 deviceId
                    String body = new String(msg.getBody());
                    String deviceId = "device-" + registerNum;
                    if (body.contains("deviceId")) {
                        deviceId = body.split("\"deviceId\":\"")[1].split("\"")[0];
                    }

                    int port = portAllocator.incrementAndGet();
                    devicePortMap.put(deviceId, port);

                    // 发送注册响应
                    String registerAck = "{\"success\":true,\"deviceId\":\"" + deviceId + "\",\"externalPort\":" + port + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_REGISTER_ACK, registerAck);
                    break;

                case ProtocolConstants.TYPE_HEARTBEAT:
                    String heartbeatAck = "{\"success\":true,\"timestamp\":" + System.currentTimeMillis() + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_HEARTBEAT_ACK, heartbeatAck);
                    break;

                case ProtocolConstants.TYPE_DATA:
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
        public void channelActive(ChannelHandlerContext ctx) throws Exception {
            connectionCount.incrementAndGet();
            super.channelActive(ctx);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            System.err.println("Server exception: " + cause.getMessage());
            ctx.close();
        }
    }
}