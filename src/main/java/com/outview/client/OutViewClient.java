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
import java.util.Scanner;
import java.util.concurrent.TimeUnit;

/**
 * outView 客户端
 * 支持 SSL/TLS 加密连接
 */
public class OutViewClient {

    private static final int MAGIC_NUMBER = 0x4F565753;
    private static final byte VERSION = 1;
    private static final byte TYPE_REGISTER = 1;
    private static final byte TYPE_HEARTBEAT = 2;
    private static final byte TYPE_DATA = 3;

    private final String serverHost;
    private final int serverPort;
    private final String deviceId;
    private final String token;
    private final int localPort;
    private final boolean sslEnabled;

    private Channel channel;
    private EventLoopGroup group;

    public OutViewClient(String serverHost, int serverPort, String deviceId, String token, int localPort) {
        this(serverHost, serverPort, deviceId, token, localPort, false);
    }

    public OutViewClient(String serverHost, int serverPort, String deviceId, String token, int localPort, boolean sslEnabled) {
        this.serverHost = serverHost;
        this.serverPort = serverPort;
        this.deviceId = deviceId;
        this.token = token;
        this.localPort = localPort;
        this.sslEnabled = sslEnabled;
    }

    public void start() throws Exception {
        group = new NioEventLoopGroup();
        try {
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
                                pipeline.addLast("ssl", sslContext.newHandler(ch.alloc(), serverHost, serverPort));
                                System.out.println("SSL/TLS enabled for connection");
                            }

                            pipeline.addLast(new IdleStateHandler(0, 30, 0, TimeUnit.SECONDS));
                            pipeline.addLast(new ClientHandler());
                        }
                    });

            ChannelFuture future = bootstrap.connect(serverHost, serverPort).sync();
            channel = future.channel();
            System.out.println("Connected to server: " + serverHost + ":" + serverPort);

            // 发送注册消息
            sendRegister();

            // 等待用户输入
            System.out.println("\nPress ENTER to send heartbeat, 'q' to quit:");
            Scanner scanner = new Scanner(System.in);
            String line;
            while ((line = scanner.nextLine()) != null) {
                if ("q".equalsIgnoreCase(line)) {
                    break;
                }
                sendHeartbeat();
            }

            channel.closeFuture().sync();
        } finally {
            group.shutdownGracefully();
        }
    }

    private void sendRegister() {
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
        System.out.println("Sent heartbeat");
    }

    public void stop() {
        if (group != null) {
            group.shutdownGracefully();
        }
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
            short reserved = msg.readShort();

            if (magic != MAGIC_NUMBER) {
                System.err.println("Invalid magic number: " + Integer.toHexString(magic));
                return;
            }

            if (msg.readableBytes() >= length) {
                byte[] body = new byte[length];
                msg.readBytes(body);
                String bodyStr = new String(body, StandardCharsets.UTF_8);
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

    public static void main(String[] args) throws Exception {
        // 默认配置
        String serverHost = "localhost";
        int serverPort = 7000;
        String deviceId = "test-device-001";
        String token = "test-token-001";
        int localPort = 3389;
        boolean sslEnabled = false;

        // 解析命令行参数
        if (args.length > 0) {
            serverHost = args[0];
        }
        if (args.length > 1) {
            serverPort = Integer.parseInt(args[1]);
        }
        if (args.length > 2) {
            deviceId = args[2];
        }
        if (args.length > 3) {
            token = args[3];
        }
        if (args.length > 4) {
            localPort = Integer.parseInt(args[4]);
        }
        if (args.length > 5) {
            sslEnabled = Boolean.parseBoolean(args[5]);
        }

        System.out.println("====================================");
        System.out.println("outView Client");
        System.out.println("====================================");
        System.out.println("Server: " + serverHost + ":" + serverPort);
        System.out.println("DeviceId: " + deviceId);
        System.out.println("Token: " + maskToken(token));
        System.out.println("LocalPort: " + localPort);
        System.out.println("SSL Enabled: " + sslEnabled);
        System.out.println("====================================\n");

        OutViewClient client = new OutViewClient(serverHost, serverPort, deviceId, token, localPort, sslEnabled);
        client.start();
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