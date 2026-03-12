package com.outview.client;

import io.netty.bootstrap.Bootstrap;
import io.netty.buffer.ByteBuf;
import io.netty.buffer.Unpooled;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioSocketChannel;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.ssl.SslContextBuilder;
import io.netty.handler.ssl.util.InsecureTrustManagerFactory;
import io.netty.handler.timeout.IdleState;
import io.netty.handler.timeout.IdleStateEvent;
import io.netty.handler.timeout.IdleStateHandler;

import java.nio.charset.StandardCharsets;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/**
 * outView 客户端测试
 * 支持 SSL/TLS 加密连接
 */
public class OutViewClientTest {

    private static final int MAGIC_NUMBER = 0x4F565753;
    private static final byte VERSION = 1;
    private static final byte TYPE_REGISTER = 1;
    private static final byte TYPE_HEARTBEAT = 2;

    private Channel channel;
    private EventLoopGroup group;
    private final CountDownLatch registerLatch = new CountDownLatch(1);
    private volatile String registerResponse;

    // SSL 配置
    private final boolean sslEnabled;
    private final String serverHost;
    private final int serverPort;

    public OutViewClientTest() {
        this(false, "localhost", 7000);
    }

    public OutViewClientTest(boolean sslEnabled, String serverHost, int serverPort) {
        this.sslEnabled = sslEnabled;
        this.serverHost = serverHost;
        this.serverPort = serverPort;
    }

    public static void main(String[] args) throws Exception {
        String serverHost = args.length > 0 ? args[0] : "localhost";
        int serverPort = args.length > 1 ? Integer.parseInt(args[1]) : 7000;
        String deviceId = args.length > 2 ? args[2] : "test-device-001";
        String token = args.length > 3 ? args[3] : "test-token-001";
        int localPort = args.length > 4 ? Integer.parseInt(args[4]) : 3389;
        boolean sslEnabled = args.length > 5 ? Boolean.parseBoolean(args[5]) : false;

        System.out.println("====================================");
        System.out.println("outView Client Test");
        System.out.println("====================================");
        System.out.println("Server: " + serverHost + ":" + serverPort);
        System.out.println("DeviceId: " + deviceId);
        System.out.println("Token: " + maskToken(token));
        System.out.println("LocalPort: " + localPort);
        System.out.println("SSL Enabled: " + sslEnabled);
        System.out.println("====================================\n");

        OutViewClientTest client = new OutViewClientTest(sslEnabled, serverHost, serverPort);
        try {
            client.connect(serverHost, serverPort, deviceId, token, localPort);

            // 等待注册响应
            if (client.registerLatch.await(5, TimeUnit.SECONDS)) {
                System.out.println("\n✅ Register response: " + client.registerResponse);
            } else {
                System.out.println("\n❌ No register response received (timeout)");
            }

            // 发送几次心跳
            for (int i = 0; i < 3; i++) {
                Thread.sleep(2000);
                client.sendHeartbeat();
                System.out.println("Sent heartbeat #" + (i + 1));
            }

            System.out.println("\n✅ Test completed successfully!");

        } finally {
            client.disconnect();
        }
    }

    public void connect(String host, int port, String deviceId, String token, int localPort) throws Exception {
        group = new NioEventLoopGroup();
        Bootstrap bootstrap = new Bootstrap();
        bootstrap.group(group)
                .channel(NioSocketChannel.class)
                .option(ChannelOption.TCP_NODELAY, true)
                .option(ChannelOption.SO_KEEPALIVE, true)
                .handler(new ChannelInitializer<SocketChannel>() {
                    @Override
                    protected void initChannel(SocketChannel ch) throws Exception {
                        ChannelPipeline pipeline = ch.pipeline();

                        // SSL/TLS 支持
                        if (sslEnabled) {
                            SslContext sslContext = SslContextBuilder.forClient()
                                    .trustManager(InsecureTrustManagerFactory.INSTANCE) // 仅用于测试
                                    .protocols("TLSv1.2", "TLSv1.3")
                                    .build();
                            pipeline.addLast("ssl", sslContext.newHandler(ch.alloc(), host, port));
                            System.out.println("SSL/TLS handler added");
                        }

                        pipeline.addLast(new IdleStateHandler(0, 30, 0, TimeUnit.SECONDS));
                        pipeline.addLast(new ClientHandler());
                    }
                });

        ChannelFuture future = bootstrap.connect(host, port).sync();
        channel = future.channel();
        System.out.println("✅ Connected to server: " + host + ":" + port);

        // 发送注册消息
        sendRegister(deviceId, token, localPort);
    }

    private void sendRegister(String deviceId, String token, int localPort) {
        String json = String.format(
                "{\"deviceId\":\"%s\",\"token\":\"%s\",\"localPort\":%d}",
                deviceId, token, localPort
        );
        byte[] body = json.getBytes(StandardCharsets.UTF_8);

        ByteBuf buf = Unpooled.buffer(12 + body.length);
        buf.writeInt(MAGIC_NUMBER);
        buf.writeByte(VERSION);
        buf.writeByte(TYPE_REGISTER);
        buf.writeInt(body.length);
        buf.writeShort((short) 0);
        buf.writeBytes(body);

        channel.writeAndFlush(buf);
        System.out.println("Sent register request: " + json);
    }

    private void sendHeartbeat() {
        String json = String.format("{\"timestamp\":%d}", System.currentTimeMillis());
        byte[] body = json.getBytes(StandardCharsets.UTF_8);

        ByteBuf buf = Unpooled.buffer(12 + body.length);
        buf.writeInt(MAGIC_NUMBER);
        buf.writeByte(VERSION);
        buf.writeByte(TYPE_HEARTBEAT);
        buf.writeInt(body.length);
        buf.writeShort((short) 0);
        buf.writeBytes(body);

        channel.writeAndFlush(buf);
    }

    public void disconnect() {
        if (channel != null) {
            channel.close();
        }
        if (group != null) {
            group.shutdownGracefully();
        }
        System.out.println("Disconnected from server");
    }

    class ClientHandler extends SimpleChannelInboundHandler<ByteBuf> {
        @Override
        protected void channelRead0(ChannelHandlerContext ctx, ByteBuf msg) {
            if (msg.readableBytes() < 12) {
                return;
            }

            int magic = msg.readInt();
            byte version = msg.readByte();
            byte type = msg.readByte();
            int length = msg.readInt();
            msg.readShort(); // reserved

            if (magic != MAGIC_NUMBER) {
                System.err.println("Invalid magic number: " + Integer.toHexString(magic));
                return;
            }

            if (msg.readableBytes() >= length) {
                byte[] body = new byte[length];
                msg.readBytes(body);
                String bodyStr = new String(body, StandardCharsets.UTF_8);

                if (type == 5) { // REGISTER_ACK
                    registerResponse = bodyStr;
                    registerLatch.countDown();
                }

                System.out.println("Received: type=" + type + ", body=" + bodyStr);
            }
        }

        @Override
        public void userEventTriggered(ChannelHandlerContext ctx, Object evt) throws Exception {
            if (evt instanceof IdleStateEvent) {
                IdleStateEvent event = (IdleStateEvent) evt;
                if (event.state() == IdleState.WRITER_IDLE) {
                    sendHeartbeat();
                }
            }
            super.userEventTriggered(ctx, evt);
        }

        @Override
        public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
            System.err.println("Exception: " + cause.getMessage());
            ctx.close();
        }
    }

    /**
     * Token 脱敏处理
     */
    private static String maskToken(String token) {
        if (token == null || token.length() <= 8) {
            return "****";
        }
        return token.substring(0, 4) + "****" + token.substring(token.length() - 4);
    }
}