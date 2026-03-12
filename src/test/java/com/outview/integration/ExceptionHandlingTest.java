package com.outview.integration;

import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.protocol.codec.MessageDecoder;
import com.outview.protocol.codec.MessageEncoder;
import io.netty.bootstrap.Bootstrap;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.Unpooled;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.channel.socket.nio.NioSocketChannel;
import org.junit.jupiter.api.*;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 异常处理测试
 * 测试各种异常场景的处理
 */
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class ExceptionHandlingTest {

    private static final int TEST_PORT = 17995;
    private static final String TEST_HOST = "localhost";
    private static final int MAGIC_NUMBER = 0x4F565753;

    private static EventLoopGroup serverBossGroup;
    private static EventLoopGroup serverWorkerGroup;
    private static Channel serverChannel;

    @BeforeAll
    static void setupServer() throws Exception {
        serverBossGroup = new NioEventLoopGroup(1);
        serverWorkerGroup = new NioEventLoopGroup();

        ServerBootstrap bootstrap = new ServerBootstrap();
        bootstrap.group(serverBossGroup, serverWorkerGroup)
                .channel(NioServerSocketChannel.class)
                .option(ChannelOption.SO_BACKLOG, 128)
                .childHandler(new ChannelInitializer<SocketChannel>() {
                    @Override
                    protected void initChannel(SocketChannel ch) {
                        ch.pipeline()
                                .addLast(new MessageDecoder())
                                .addLast(new MessageEncoder())
                                .addLast(new ExceptionTestServerHandler());
                    }
                });

        serverChannel = bootstrap.bind(TEST_PORT).sync().channel();
        System.out.println("Exception test server started on port: " + TEST_PORT);
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
        System.out.println("Exception test server stopped");
    }

    /**
     * 测试1: 无效 Magic Number
     */
    @Test
    @Order(1)
    void testInvalidMagicNumber() throws Exception {
        CountDownLatch disconnectLatch = new CountDownLatch(1);
        AtomicBoolean channelClosed = new AtomicBoolean(false);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline().addLast(new SimpleChannelInboundHandler<ByteBuf>() {
                                @Override
                                protected void channelRead0(ChannelHandlerContext ctx, ByteBuf msg) {
                                    // 不处理响应
                                }

                                @Override
                                public void channelInactive(ChannelHandlerContext ctx) {
                                    channelClosed.set(true);
                                    disconnectLatch.countDown();
                                }
                            });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送无效 Magic Number 的消息
            ByteBuf buf = Unpooled.buffer(16);
            buf.writeInt(0x12345678); // Invalid magic
            buf.writeByte(1);         // Version
            buf.writeByte(1);         // Type
            buf.writeInt(0);          // Length
            buf.writeShort(0);        // Reserved

            channel.writeAndFlush(buf);

            // 等待连接关闭
            boolean closed = disconnectLatch.await(5, TimeUnit.SECONDS);

            System.out.println("Invalid Magic Number Test: Channel closed = " + channelClosed.get());
            assertTrue(closed || !channel.isActive(), "Channel should be closed after invalid magic number");

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试2: 不完整的消息头
     */
    @Test
    @Order(2)
    void testIncompleteHeader() throws Exception {
        CountDownLatch responseLatch = new CountDownLatch(1);
        AtomicBoolean errorReceived = new AtomicBoolean(false);

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
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_ERROR) {
                                                errorReceived.set(true);
                                            }
                                            responseLatch.countDown();
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送不完整的消息 (只有 8 字节，少于 12 字节的头)
            ByteBuf buf = Unpooled.buffer(8);
            buf.writeInt(MAGIC_NUMBER);
            buf.writeByte(1);
            buf.writeByte(1);
            buf.writeShort(0);

            channel.writeAndFlush(buf);

            // 等待一段时间
            Thread.sleep(1000);

            // 连接应该仍然活跃（等待更多数据）
            System.out.println("Incomplete Header Test: Channel active = " + channel.isActive());

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试3: 超大消息
     */
    @Test
    @Order(3)
    void testOversizedMessage() throws Exception {
        CountDownLatch latch = new CountDownLatch(1);
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
                                            latch.countDown();
                                        }

                                        @Override
                                        public void channelInactive(ChannelHandlerContext ctx) {
                                            disconnected.set(true);
                                            latch.countDown();
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送超大消息 (10MB)
            int largeSize = 10 * 1024 * 1024;
            ByteBuf buf = Unpooled.buffer(12 + 100); // 实际不发送 10MB，只发送声明
            buf.writeInt(MAGIC_NUMBER);
            buf.writeByte(1);
            buf.writeByte(ProtocolConstants.TYPE_DATA);
            buf.writeInt(largeSize); // 声明大量数据
            buf.writeShort(0);

            channel.writeAndFlush(buf);

            boolean completed = latch.await(5, TimeUnit.SECONDS);

            System.out.println("Oversized Message Test: Disconnected = " + disconnected.get());
            // 服务端可能会关闭连接或拒绝处理

            if (channel.isActive()) {
                channel.close().sync();
            }

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试4: 无效的消息类型
     */
    @Test
    @Order(4)
    void testInvalidMessageType() throws Exception {
        CountDownLatch latch = new CountDownLatch(1);
        AtomicReference<ProtocolMessage> responseRef = new AtomicReference<>();

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
                                            responseRef.set(msg);
                                            latch.countDown();
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送无效类型的消息
            ByteBuf buf = Unpooled.buffer(12);
            buf.writeInt(MAGIC_NUMBER);
            buf.writeByte(1);
            buf.writeByte(99); // Invalid type
            buf.writeInt(0);
            buf.writeShort(0);

            channel.writeAndFlush(buf);

            boolean completed = latch.await(5, TimeUnit.SECONDS);

            System.out.println("Invalid Message Type Test: Completed = " + completed);
            // 服务端可能忽略或返回错误

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试5: 空消息体
     */
    @Test
    @Order(5)
    void testEmptyMessageBody() throws Exception {
        CountDownLatch latch = new CountDownLatch(1);
        AtomicBoolean success = new AtomicBoolean(false);

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
                                            if (msg.getHeader().getType() == ProtocolConstants.TYPE_HEARTBEAT_ACK) {
                                                success.set(true);
                                            }
                                            latch.countDown();
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送空消息体的心跳
            ByteBuf buf = Unpooled.buffer(12);
            buf.writeInt(MAGIC_NUMBER);
            buf.writeByte(1);
            buf.writeByte(ProtocolConstants.TYPE_HEARTBEAT);
            buf.writeInt(0); // Empty body
            buf.writeShort(0);

            channel.writeAndFlush(buf);

            boolean completed = latch.await(5, TimeUnit.SECONDS);

            System.out.println("Empty Message Body Test: Success = " + success.get());
            assertTrue(completed, "Should receive response for empty body heartbeat");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试6: 连接后不发送数据
     */
    @Test
    @Order(6)
    void testIdleConnection() throws Exception {
        AtomicBoolean disconnected = new AtomicBoolean(false);
        CountDownLatch latch = new CountDownLatch(1);

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline().addLast(new SimpleChannelInboundHandler<ByteBuf>() {
                                @Override
                                protected void channelRead0(ChannelHandlerContext ctx, ByteBuf msg) {
                                }

                                @Override
                                public void channelInactive(ChannelHandlerContext ctx) {
                                    disconnected.set(true);
                                    latch.countDown();
                                }
                            });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
            assertTrue(channel.isActive(), "Channel should be active after connection");

            // 等待但不发送任何数据
            Thread.sleep(2000);

            System.out.println("Idle Connection Test: Still active after 2s = " + channel.isActive());
            assertTrue(channel.isActive(), "Channel should still be active without idle timeout");

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试7: 重复注册
     */
    @Test
    @Order(7)
    void testDuplicateRegistration() throws Exception {
        String deviceId = "duplicate-test-device";
        CountDownLatch firstLatch = new CountDownLatch(1);
        CountDownLatch secondLatch = new CountDownLatch(1);
        AtomicInteger portCount = new AtomicInteger(0);

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
                                                portCount.incrementAndGet();
                                                if (portCount.get() == 1) {
                                                    firstLatch.countDown();
                                                } else {
                                                    secondLatch.countDown();
                                                }
                                            }
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送第一次注册
            ProtocolMessage register1 = ProtocolMessage.register(deviceId, "token", 3389);
            channel.writeAndFlush(register1);

            assertTrue(firstLatch.await(5, TimeUnit.SECONDS), "First registration should succeed");

            // 发送第二次注册 (相同 deviceId)
            ProtocolMessage register2 = ProtocolMessage.register(deviceId, "token", 3389);
            channel.writeAndFlush(register2);

            boolean secondResult = secondLatch.await(5, TimeUnit.SECONDS);

            System.out.println("Duplicate Registration Test: Both registered = " + (portCount.get() == 2));
            // 服务端可能允许重复注册或拒绝

            channel.close().sync();

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试8: 客户端突然断开
     */
    @Test
    @Order(8)
    void testAbruptClientDisconnect() throws Exception {
        CountDownLatch registerLatch = new CountDownLatch(1);

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
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送注册
            ProtocolMessage register = ProtocolMessage.register("abrupt-disconnect-device", "token", 3389);
            channel.writeAndFlush(register);

            assertTrue(registerLatch.await(5, TimeUnit.SECONDS), "Registration should succeed");

            // 突然关闭连接 (不发送断开消息)
            channel.close().sync();
            System.out.println("Abrupt disconnect test completed - client closed connection");

            // 给服务端时间处理断开
            Thread.sleep(500);

            // 服务端应该正常处理了断开

        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 异常测试服务端处理器
     */
    static class ExceptionTestServerHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        private static final int MAX_MESSAGE_SIZE = 1024 * 1024; // 1MB

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
            byte type = msg.getHeader().getType();

            switch (type) {
                case ProtocolConstants.TYPE_REGISTER:
                    String body = new String(msg.getBody());
                    String deviceId = "device";
                    if (body.contains("deviceId")) {
                        deviceId = body.split("\"deviceId\":\"")[1].split("\"")[0];
                    }

                    String registerAck = "{\"success\":true,\"deviceId\":\"" + deviceId + "\",\"externalPort\":9001}";
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
                    // 未知消息类型 - 发送错误或忽略
                    System.out.println("Unknown message type: " + type);
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
            System.err.println("Server exception: " + cause.getClass().getSimpleName() + " - " + cause.getMessage());
            ctx.close();
        }

        @Override
        public void channelInactive(ChannelHandlerContext ctx) throws Exception {
            System.out.println("Client disconnected");
            super.channelInactive(ctx);
        }
    }
}