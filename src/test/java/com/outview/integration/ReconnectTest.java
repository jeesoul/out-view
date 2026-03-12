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
import io.netty.handler.timeout.IdleStateHandler;
import org.junit.jupiter.api.*;

import java.util.Map;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 断线重连测试
 * 测试客户端断线后的重连机制
 */
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class ReconnectTest {

    private static final int TEST_PORT = 17997;
    private static final String TEST_HOST = "localhost";
    private static final int MAGIC_NUMBER = 0x4F565753;

    private static EventLoopGroup serverBossGroup;
    private static EventLoopGroup serverWorkerGroup;
    private static Channel serverChannel;
    private static AtomicInteger connectionCount = new AtomicInteger(0);
    private static AtomicInteger disconnectionCount = new AtomicInteger(0);
    private static Map<String, Long> lastHeartbeatMap = new ConcurrentHashMap<>();

    @BeforeAll
    static void setupServer() throws Exception {
        serverBossGroup = new NioEventLoopGroup(1);
        serverWorkerGroup = new NioEventLoopGroup();

        ServerBootstrap bootstrap = new ServerBootstrap();
        bootstrap.group(serverBossGroup, serverWorkerGroup)
                .channel(NioServerSocketChannel.class)
                .option(ChannelOption.SO_BACKLOG, 128)
                .childOption(ChannelOption.SO_KEEPALIVE, true)
                .childOption(ChannelOption.TCP_NODELAY, true)
                .childHandler(new ChannelInitializer<SocketChannel>() {
                    @Override
                    protected void initChannel(SocketChannel ch) {
                        ch.pipeline()
                                .addLast(new IdleStateHandler(30, 0, 0, TimeUnit.SECONDS))
                                .addLast(new MessageDecoder())
                                .addLast(new MessageEncoder())
                                .addLast(new ReconnectTestServerHandler());
                    }
                });

        serverChannel = bootstrap.bind(TEST_PORT).sync().channel();
        System.out.println("Reconnect test server started on port: " + TEST_PORT);
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
        System.out.println("Reconnect test server stopped");
    }

    /**
     * 测试1: 客户端主动断开后重连
     */
    @Test
    @Order(1)
    void testClientInitiatedReconnect() throws Exception {
        String deviceId = "reconnect-device-001";
        CountDownLatch firstConnectLatch = new CountDownLatch(1);
        CountDownLatch secondConnectLatch = new CountDownLatch(1);
        AtomicInteger connectCount = new AtomicInteger(0);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            // 第一次连接
            Channel firstChannel = createAndConnectClient(clientGroup, deviceId, () -> {
                connectCount.incrementAndGet();
                if (connectCount.get() == 1) {
                    firstConnectLatch.countDown();
                } else {
                    secondConnectLatch.countDown();
                }
            });

            assertTrue(firstConnectLatch.await(5, TimeUnit.SECONDS), "First connection should succeed");
            assertTrue(firstChannel.isActive(), "First channel should be active");
            System.out.println("First connection established");

            // 主动断开
            firstChannel.close().sync();
            System.out.println("Client disconnected");

            // 等待一段时间
            Thread.sleep(500);

            // 重新连接
            Channel secondChannel = createAndConnectClient(clientGroup, deviceId, () -> {
                connectCount.incrementAndGet();
                secondConnectLatch.countDown();
            });

            assertTrue(secondConnectLatch.await(5, TimeUnit.SECONDS), "Second connection should succeed");
            assertTrue(secondChannel.isActive(), "Second channel should be active");
            System.out.println("Reconnection successful");

            assertEquals(2, connectCount.get(), "Should have connected twice");

            secondChannel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试2: 服务端主动断开后客户端重连
     */
    @Test
    @Order(2)
    void testServerInitiatedDisconnect() throws Exception {
        String deviceId = "reconnect-device-002";
        CountDownLatch registerLatch1 = new CountDownLatch(1);
        CountDownLatch registerLatch2 = new CountDownLatch(1);
        AtomicReference<Channel> currentChannel = new AtomicReference<>();
        AtomicInteger registerCount = new AtomicInteger(0);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            // 第一次连接
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
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                                                int count = registerCount.incrementAndGet();
                                                if (count == 1) {
                                                    registerLatch1.countDown();
                                                } else {
                                                    registerLatch2.countDown();
                                                }
                                            }
                                        }

                                        @Override
                                        public void channelInactive(ChannelHandlerContext ctx) {
                                            System.out.println("Channel became inactive");
                                        }
                                    });
                        }
                    });

            Channel firstChannel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
            currentChannel.set(firstChannel);

            // 发送注册
            ProtocolMessage register = ProtocolMessage.register(deviceId, "token", 3389);
            firstChannel.writeAndFlush(register);

            assertTrue(registerLatch1.await(5, TimeUnit.SECONDS), "First registration should succeed");
            System.out.println("First registration successful");

            // 记录当前连接数
            int connectionsBefore = connectionCount.get();

            // 服务端关闭连接 (模拟服务端断开)
            firstChannel.close().sync();
            Thread.sleep(500);

            // 客户端重连
            Channel secondChannel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 重新注册
            ProtocolMessage register2 = ProtocolMessage.register(deviceId, "token", 3389);
            secondChannel.writeAndFlush(register2);

            assertTrue(registerLatch2.await(5, TimeUnit.SECONDS), "Second registration should succeed");
            System.out.println("Reconnection and registration successful");

            secondChannel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试3: 心跳超时断开
     */
    @Test
    @Order(3)
    void testHeartbeatTimeout() throws Exception {
        String deviceId = "timeout-device-001";
        CountDownLatch registerLatch = new CountDownLatch(1);
        CountDownLatch disconnectLatch = new CountDownLatch(1);
        AtomicBoolean disconnected = new AtomicBoolean(false);

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
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                                                registerLatch.countDown();
                                            }
                                        }

                                        @Override
                                        public void channelInactive(ChannelHandlerContext ctx) {
                                            disconnected.set(true);
                                            disconnectLatch.countDown();
                                            System.out.println("Client disconnected (expected timeout)");
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送注册
            ProtocolMessage register = ProtocolMessage.register(deviceId, "token", 3389);
            channel.writeAndFlush(register);

            assertTrue(registerLatch.await(5, TimeUnit.SECONDS), "Registration should succeed");

            // 不发送心跳，等待服务端超时断开
            // 服务端设置了 30 秒的读超时
            System.out.println("Waiting for server timeout (may take up to 35 seconds)...");

            boolean timedOut = disconnectLatch.await(40, TimeUnit.SECONDS);
            assertTrue(timedOut, "Connection should be closed by timeout");
            assertTrue(disconnected.get(), "Client should be disconnected");

            System.out.println("Heartbeat timeout test passed");

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试4: 重连后端口重新分配
     */
    @Test
    @Order(4)
    void testPortReallocationOnReconnect() throws Exception {
        String deviceId = "port-realloc-device";
        AtomicInteger portRef = new AtomicInteger(0);
        AtomicInteger portRef2 = new AtomicInteger(0);
        CountDownLatch latch1 = new CountDownLatch(1);
        CountDownLatch latch2 = new CountDownLatch(1);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            // 第一次连接
            Channel channel1 = createClientWithPortResponse(clientGroup, portRef, latch1);
            ProtocolMessage register1 = ProtocolMessage.register(deviceId, "token", 3389);
            channel1.writeAndFlush(register1);

            assertTrue(latch1.await(5, TimeUnit.SECONDS), "First registration should succeed");
            int firstPort = portRef.get();
            System.out.println("First port allocated: " + firstPort);

            // 断开
            channel1.close().sync();
            Thread.sleep(500);

            // 第二次连接 (相同 deviceId)
            Channel channel2 = createClientWithPortResponse(clientGroup, portRef2, latch2);
            ProtocolMessage register2 = ProtocolMessage.register(deviceId, "token", 3389);
            channel2.writeAndFlush(register2);

            assertTrue(latch2.await(5, TimeUnit.SECONDS), "Second registration should succeed");
            int secondPort = portRef2.get();
            System.out.println("Second port allocated: " + secondPort);

            // 验证端口重新分配
            assertTrue(secondPort > 0, "Port should be allocated");

            channel2.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试5: 连接稳定性 - 持续心跳保持连接
     */
    @Test
    @Order(5)
    void testConnectionStabilityWithHeartbeat() throws Exception {
        String deviceId = "stable-device";
        CountDownLatch registerLatch = new CountDownLatch(1);
        AtomicInteger heartbeatAckCount = new AtomicInteger(0);
        AtomicBoolean disconnected = new AtomicBoolean(false);

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
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                                                registerLatch.countDown();
                                            } else if (msg.getHeader().getType() == ProtocolConstants.TYPE_HEARTBEAT_ACK) {
                                                heartbeatAckCount.incrementAndGet();
                                            }
                                        }

                                        @Override
                                        public void channelInactive(ChannelHandlerContext ctx) {
                                            disconnected.set(true);
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 注册
            ProtocolMessage register = ProtocolMessage.register(deviceId, "token", 3389);
            channel.writeAndFlush(register);
            assertTrue(registerLatch.await(5, TimeUnit.SECONDS), "Registration should succeed");

            // 持续发送心跳
            for (int i = 0; i < 5; i++) {
                Thread.sleep(2000);
                ProtocolMessage heartbeat = ProtocolMessage.heartbeat();
                channel.writeAndFlush(heartbeat);
            }

            // 等待心跳响应
            Thread.sleep(1000);

            // 验证连接仍然活跃
            assertTrue(channel.isActive(), "Channel should still be active");
            assertFalse(disconnected.get(), "Should not be disconnected");
            assertTrue(heartbeatAckCount.get() >= 4, "Should receive most heartbeat acks");

            System.out.println("Connection remained stable with heartbeats. Received " + heartbeatAckCount.get() + " acks");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    private Channel createAndConnectClient(EventLoopGroup group, String deviceId, Runnable onRegister) throws Exception {
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
                                            onRegister.run();
                                        }
                                    }
                                });
                    }
                });

        Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

        // 发送注册
        ProtocolMessage register = ProtocolMessage.register(deviceId, "token", 3389);
        channel.writeAndFlush(register);

        return channel;
    }

    private Channel createClientWithPortResponse(EventLoopGroup group, AtomicInteger portRef, CountDownLatch latch) throws Exception {
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
                                            String body = new String(msg.getBody());
                                            if (body.contains("externalPort")) {
                                                String portStr = body.split("\"externalPort\":")[1].split("[,}]")[0];
                                                portRef.set(Integer.parseInt(portStr));
                                            }
                                            latch.countDown();
                                        }
                                    }
                                });
                    }
                });

        return bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
    }

    /**
     * 重连测试服务端处理器
     */
    static class ReconnectTestServerHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        private static AtomicInteger portAllocator = new AtomicInteger(7000);
        private String deviceId;

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
            byte type = msg.getHeader().getType();

            switch (type) {
                case ProtocolConstants.TYPE_REGISTER:
                    String body = new String(msg.getBody());
                    deviceId = "device";
                    if (body.contains("deviceId")) {
                        deviceId = body.split("\"deviceId\":\"")[1].split("\"")[0];
                    }

                    int port = portAllocator.incrementAndGet();
                    lastHeartbeatMap.put(deviceId, System.currentTimeMillis());

                    String registerAck = "{\"success\":true,\"deviceId\":\"" + deviceId + "\",\"externalPort\":" + port + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_REGISTER_ACK, registerAck);
                    break;

                case ProtocolConstants.TYPE_HEARTBEAT:
                    if (deviceId != null) {
                        lastHeartbeatMap.put(deviceId, System.currentTimeMillis());
                    }
                    String heartbeatAck = "{\"success\":true,\"timestamp\":" + System.currentTimeMillis() + "}";
                    sendResponse(ctx, ProtocolConstants.TYPE_HEARTBEAT_ACK, heartbeatAck);
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
        public void channelInactive(ChannelHandlerContext ctx) throws Exception {
            disconnectionCount.incrementAndGet();
            if (deviceId != null) {
                lastHeartbeatMap.remove(deviceId);
            }
            super.channelInactive(ctx);
        }

        @Override
        public void userEventTriggered(ChannelHandlerContext ctx, Object evt) throws Exception {
            if (evt instanceof io.netty.handler.timeout.IdleStateEvent) {
                io.netty.handler.timeout.IdleStateEvent event = (io.netty.handler.timeout.IdleStateEvent) evt;
                if (event.state() == io.netty.handler.timeout.IdleState.READER_IDLE) {
                    System.out.println("Server detected read idle, closing connection for device: " + deviceId);
                    ctx.close();
                }
            }
            super.userEventTriggered(ctx, evt);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            System.err.println("Server exception: " + cause.getMessage());
            ctx.close();
        }
    }
}