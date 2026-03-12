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

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 端到端集成测试
 * 测试完整的客户端连接流程
 */
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class EndToEndIntegrationTest {

    private static final int TEST_PORT = 17999;
    private static final String TEST_HOST = "localhost";
    private static final int MAGIC_NUMBER = 0x4F565753;

    private static EventLoopGroup serverBossGroup;
    private static EventLoopGroup serverWorkerGroup;
    private static Channel serverChannel;

    private static AtomicInteger registerCount = new AtomicInteger(0);
    private static AtomicInteger heartbeatCount = new AtomicInteger(0);

    @BeforeAll
    static void setupServer() throws Exception {
        serverBossGroup = new NioEventLoopGroup(1);
        serverWorkerGroup = new NioEventLoopGroup();

        ServerBootstrap bootstrap = new ServerBootstrap();
        bootstrap.group(serverBossGroup, serverWorkerGroup)
                .channel(NioServerSocketChannel.class)
                .option(ChannelOption.SO_BACKLOG, 128)
                .childOption(ChannelOption.SO_KEEPALIVE, true)
                .childHandler(new ChannelInitializer<SocketChannel>() {
                    @Override
                    protected void initChannel(SocketChannel ch) {
                        ch.pipeline()
                                .addLast(new MessageDecoder())
                                .addLast(new MessageEncoder())
                                .addLast(new TestServerHandler());
                    }
                });

        serverChannel = bootstrap.bind(TEST_PORT).sync().channel();
        System.out.println("Test server started on port: " + TEST_PORT);
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
        System.out.println("Test server stopped");
    }

    /**
     * 测试1: 单客户端连接和注册
     */
    @Test
    @Order(1)
    void testSingleClientConnection() throws Exception {
        CountDownLatch registerLatch = new CountDownLatch(1);
        AtomicReference<String> responseRef = new AtomicReference<>();

        EventLoopGroup clientGroup = new NioEventLoopGroup();
        try {
            Bootstrap bootstrap = new Bootstrap();
            bootstrap.group(clientGroup)
                    .channel(NioSocketChannel.class)
                    .option(ChannelOption.TCP_NODELAY, true)
                    .handler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline()
                                    .addLast(new MessageDecoder())
                                    .addLast(new MessageEncoder())
                                    .addLast(new TestClientHandler(registerLatch, responseRef));
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();
            assertTrue(channel.isActive(), "Client should be connected");

            // 发送注册消息
            ProtocolMessage register = ProtocolMessage.register("test-device-001", "test-token", 3389);
            channel.writeAndFlush(register);

            // 等待响应
            boolean received = registerLatch.await(5, TimeUnit.SECONDS);
            assertTrue(received, "Should receive register response");
            assertNotNull(responseRef.get(), "Response should not be null");

            channel.close().sync();
        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试2: 心跳消息
     */
    @Test
    @Order(2)
    void testHeartbeatMessage() throws Exception {
        CountDownLatch heartbeatLatch = new CountDownLatch(1);
        AtomicReference<String> responseRef = new AtomicReference<>();

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
                                                responseRef.set(new String(msg.getBody()));
                                                heartbeatLatch.countDown();
                                            }
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送心跳消息
            ProtocolMessage heartbeat = ProtocolMessage.heartbeat();
            channel.writeAndFlush(heartbeat);

            boolean received = heartbeatLatch.await(5, TimeUnit.SECONDS);
            assertTrue(received, "Should receive heartbeat response");

            channel.close().sync();
        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试3: 数据消息传输
     */
    @Test
    @Order(3)
    void testDataMessageTransmission() throws Exception {
        CountDownLatch dataLatch = new CountDownLatch(1);
        AtomicReference<byte[]> dataRef = new AtomicReference<>();
        byte[] testData = "Hello, outView!".getBytes(StandardCharsets.UTF_8);

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
                                                dataRef.set(msg.getBody());
                                                dataLatch.countDown();
                                            }
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送数据消息
            ProtocolMessage dataMsg = ProtocolMessage.data(testData);
            channel.writeAndFlush(dataMsg);

            boolean received = dataLatch.await(5, TimeUnit.SECONDS);
            assertTrue(received, "Should receive data echo");

            channel.close().sync();
        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试4: 错误消息处理
     */
    @Test
    @Order(4)
    void testErrorMessage() throws Exception {
        CountDownLatch errorLatch = new CountDownLatch(1);
        AtomicReference<String> errorRef = new AtomicReference<>();

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
                                                errorRef.set(new String(msg.getBody()));
                                                errorLatch.countDown();
                                            }
                                        }
                                    });
                        }
                    });

            Channel channel = bootstrap.connect(TEST_HOST, TEST_PORT).sync().channel();

            // 发送错误消息
            ProtocolMessage errorMsg = ProtocolMessage.error("Test error message");
            channel.writeAndFlush(errorMsg);

            boolean received = errorLatch.await(5, TimeUnit.SECONDS);
            assertTrue(received, "Should receive error response");

            channel.close().sync();
        } finally {
            clientGroup.shutdownGracefully();
        }
    }

    /**
     * 测试服务端处理器
     */
    static class TestServerHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) throws Exception {
            byte type = msg.getHeader().getType();

            switch (type) {
                case ProtocolConstants.TYPE_REGISTER:
                    registerCount.incrementAndGet();
                    // 发送注册响应
                    String registerAck = "{\"success\":true,\"deviceId\":\"test-device-001\",\"externalPort\":6001}";
                    ProtocolMessage ack = ProtocolMessage.builder()
                            .header(com.outview.protocol.MessageHeader.builder()
                                    .magic(ProtocolConstants.MAGIC_NUMBER)
                                    .version(ProtocolConstants.VERSION)
                                    .type(ProtocolConstants.TYPE_REGISTER_ACK)
                                    .length(registerAck.length())
                                    .build())
                            .body(registerAck.getBytes())
                            .build();
                    ctx.writeAndFlush(ack);
                    break;

                case ProtocolConstants.TYPE_HEARTBEAT:
                    heartbeatCount.incrementAndGet();
                    // 发送心跳响应
                    String heartbeatAck = "{\"success\":true,\"timestamp\":" + System.currentTimeMillis() + "}";
                    ProtocolMessage hack = ProtocolMessage.builder()
                            .header(com.outview.protocol.MessageHeader.builder()
                                    .magic(ProtocolConstants.MAGIC_NUMBER)
                                    .version(ProtocolConstants.VERSION)
                                    .type(ProtocolConstants.TYPE_HEARTBEAT_ACK)
                                    .length(heartbeatAck.length())
                                    .build())
                            .body(heartbeatAck.getBytes())
                            .build();
                    ctx.writeAndFlush(hack);
                    break;

                case ProtocolConstants.TYPE_DATA:
                    // 回显数据
                    ProtocolMessage dataAck = ProtocolMessage.data(msg.getBody());
                    ctx.writeAndFlush(dataAck);
                    break;

                case ProtocolConstants.TYPE_ERROR:
                    // 回显错误
                    ProtocolMessage errorAck = ProtocolMessage.error(new String(msg.getBody()));
                    ctx.writeAndFlush(errorAck);
                    break;

                default:
                    // 未知消息类型
                    break;
            }
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            System.err.println("Server exception: " + cause.getMessage());
            ctx.close();
        }
    }

    /**
     * 测试客户端处理器
     */
    static class TestClientHandler extends SimpleChannelInboundHandler<ProtocolMessage> {

        private final CountDownLatch latch;
        private final AtomicReference<String> responseRef;

        public TestClientHandler(CountDownLatch latch, AtomicReference<String> responseRef) {
            this.latch = latch;
            this.responseRef = responseRef;
        }

        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) {
            if (msg.getHeader().getType() == ProtocolConstants.TYPE_REGISTER_ACK) {
                responseRef.set(new String(msg.getBody()));
                latch.countDown();
            }
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            System.err.println("Client exception: " + cause.getMessage());
            ctx.close();
        }
    }
}