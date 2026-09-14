package com.outview.integration;

import com.alibaba.fastjson.JSON;
import com.outview.config.OutViewProperties;
import com.outview.controller.DeviceController;
import com.outview.entity.ClientSession;
import com.outview.entity.PortMapping;
import com.outview.netty.ControlChannelInitializer;
import com.outview.netty.DataChannelInitializer;
import com.outview.netty.handler.*;
import com.outview.protocol.*;
import com.outview.protocol.codec.*;
import com.outview.repository.*;
import com.outview.service.*;
import io.netty.bootstrap.Bootstrap;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.*;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.channel.socket.nio.NioSocketChannel;
import org.junit.jupiter.api.*;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.domain.EntityScan;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.annotation.*;
import org.springframework.data.jpa.repository.config.EnableJpaRepositories;

import java.net.*;
import java.util.*;
import java.util.concurrent.*;
import java.util.function.BooleanSupplier;
import static org.junit.jupiter.api.Assertions.*;

/** 完整生产处理器、真实 H2 和回环 TCP；没有 TestServerHandler 或模拟仓库。 */
@SpringBootTest(classes = RealLifecycleIntegrationTest.Config.class, webEnvironment = SpringBootTest.WebEnvironment.NONE,
        properties = {"spring.datasource.url=jdbc:h2:mem:real-lifecycle;DB_CLOSE_DELAY=-1;DB_CLOSE_ON_EXIT=FALSE", "spring.jpa.hibernate.ddl-auto=create-drop",
                "spring.main.allow-circular-references=false", "outview.bind-address=127.0.0.1", "outview.ssl.enabled=false",
                "outview.data-port-start=1024", "outview.data-port-end=65535", "outview.heartbeat-timeout=2",
                "logging.level.com.outview=INFO"})
class RealLifecycleIntegrationTest {
    @Configuration @EnableAutoConfiguration
    @EntityScan(basePackageClasses = PortMapping.class)
    @EnableJpaRepositories(basePackageClasses = PortMappingRepository.class)
    @Import({OutViewProperties.class, SessionStore.class, PortMappingService.class, BanService.class,
            DeviceLifecycleService.class, DataPortService.class, DataChannelInitializer.class, ControlChannelInitializer.class,
            AuthHandler.class, HeartbeatHandler.class, RendezvousHandler.class, ProxyHandler.class, RawDataHandler.class,
            DeviceController.class})
    static class Config {
        @Bean(destroyMethod = "shutdownGracefully") EventLoopGroup sharedWorkerGroup() { return new NioEventLoopGroup(2); }
        @Bean(destroyMethod = "shutdownGracefully") io.netty.util.concurrent.DefaultEventExecutorGroup controlBusinessExecutor() {
            return new io.netty.util.concurrent.DefaultEventExecutorGroup(2);
        }
    }

    @Autowired EventLoopGroup workers;
    @Autowired ControlChannelInitializer initializer;
    @Autowired DeviceLifecycleService lifecycle;
    @Autowired SessionStore sessions;
    @Autowired PortMappingService mappings;
    @Autowired PortMappingRepository repository;
    @Autowired BannedDeviceRepository bans;
    @Autowired DataPortService ports;
    @Autowired DeviceController controller;
    @Autowired RawDataHandler raw;
    @Autowired OutViewProperties properties;
    Channel server;
    int controlPort;
    List<Client> clients = new ArrayList<>();

    @BeforeEach void start() throws Exception {
        server = new ServerBootstrap().group(workers).channel(NioServerSocketChannel.class)
                .childHandler(initializer).bind("127.0.0.1", 0).sync().channel();
        controlPort = ((InetSocketAddress) server.localAddress()).getPort();
    }

    @AfterEach void cleanup() throws Exception {
        for (Client client : clients) client.close();
        for (ClientSession session : new ArrayList<>(sessions.getAllSessions())) lifecycle.disconnect(session.getDeviceId());
        for (PortMapping mapping : mappings.getAllMappings().values()) lifecycle.deleteMapping(mapping.getDeviceId());
        bans.deleteAll();
        server.close().sync();
    }

    @Test void bindCannotBeSharedByDifferentDevices() throws Exception {
        int port = freePort();
        try {
            assertTrue(ports.startDataPort(port, "first"));
            assertFalse(ports.startDataPort(port, "second"), "已存在的监听必须核验设备归属");
            assertTrue(ports.isPortActive(port));
        } finally { ports.stopDataPort(port); }
    }

    @Test void blockingRegistrationRunsOutsideNettyIoThread() throws Exception {
        Client client = registered("executor");
        Channel channel = sessions.getSession("executor").getChannel();
        assertNotSame(channel.eventLoop(), channel.pipeline().context("auth").executor());
        client.send(ProtocolMessage.heartbeat());
        assertEquals(ProtocolConstants.TYPE_HEARTBEAT_ACK, client.read().getHeader().getType());
    }

    @Test void onlinePortChangeUpdatesAckQueryDatabaseAndActualForwarding() throws Exception {
        Client client = registered("moving");
        int oldPort = mappings.getPortByDevice("moving");
        int newPort = freePort();
        Map<String, Object> result = controller.updateMapping("moving", Collections.singletonMap("externalPort", newPort));
        assertEquals(true, result.get("success"));
        assertEquals(newPort, JSON.parseObject(client.read().getBody()).getIntValue("externalPort"));
        assertEquals(newPort, sessions.getSession("moving").getExternalPort());
        assertEquals(newPort, repository.findById("moving").get().getExternalPort());
        assertFalse(ports.isPortActive(oldPort));
        assertTrue(ports.isPortActive(newPort));
        Client query = connect();
        ProtocolMessage request = ProtocolMessage.register("ignored", "ignored", 3389);
        request.setBody("{\"deviceCode\":\"moving\"}".getBytes(java.nio.charset.StandardCharsets.UTF_8));
        request.getHeader().setLength(request.getBody().length);
        request.getHeader().setType(ProtocolConstants.TYPE_DEVICE_QUERY);
        query.send(request);
        assertEquals(newPort, JSON.parseObject(query.read().getBody()).getIntValue("externalPort"));
        try (Socket external = new Socket("127.0.0.1", newPort)) {
            external.setSoTimeout(2000);
            external.getOutputStream().write(new byte[]{1, 2, 3});
            ProtocolMessage.DataPacket packet = client.read().parseDataPacket();
            assertArrayEquals(new byte[]{1, 2, 3}, packet.getData());
            client.send(ProtocolMessage.dataWithConnectionId(packet.getConnectionId(), new byte[]{4, 5}));
            assertEquals(4, external.getInputStream().read());
            assertEquals(5, external.getInputStream().read());
        }
        assertEquals(ProtocolConstants.TYPE_CLOSE_CONNECTION, client.read().getHeader().getType());
    }

    @Test void registeredTrafficBypassesBlockedRegistrationExecutor() throws Exception {
        Client client = registered("bypass");
        int port = mappings.getPortByDevice("bypass");
        try (Socket external = new Socket("127.0.0.1", port)) {
            external.setSoTimeout(700);
            external.getOutputStream().write(new byte[]{1});
            String id = client.read().parseDataPacket().getConnectionId();
            CountDownLatch busy = new CountDownLatch(1);
            CountDownLatch release = new CountDownLatch(1);
            io.netty.util.concurrent.Future<?> blocker = sessions.getSession("bypass").getChannel().pipeline()
                    .context("auth").executor().submit(() -> {
                        busy.countDown();
                        try { release.await(5, TimeUnit.SECONDS); }
                        catch (InterruptedException e) { Thread.currentThread().interrupt(); }
                    });
            try {
                assertTrue(busy.await(2, TimeUnit.SECONDS));
                client.send(ProtocolMessage.dataWithConnectionId(id, new byte[]{9}));
                assertEquals(9, assertDoesNotThrow(() -> external.getInputStream().read(),
                        "已注册设备的数据不应排在阻塞注册任务之后"));
                ProtocolMessage query = ProtocolMessage.register("ignored", "ignored", 3389);
                query.setBody("{\"deviceCode\":\"bypass\"}".getBytes(java.nio.charset.StandardCharsets.UTF_8));
                query.getHeader().setLength(query.getBody().length);
                query.getHeader().setType(ProtocolConstants.TYPE_DEVICE_QUERY);
                client.send(query);
                assertEquals(port, JSON.parseObject(client.read().getBody()).getIntValue("externalPort"));
                ProtocolMessage close = ProtocolMessage.dataWithConnectionId(id, new byte[0]);
                close.getHeader().setType(ProtocolConstants.TYPE_CLOSE_CONNECTION);
                client.send(close);
                assertEquals(-1, assertDoesNotThrow(() -> external.getInputStream().read(),
                        "单连接关闭通知不应等待注册线程"));
            } finally {
                release.countDown();
                blocker.syncUninterruptibly();
            }
        }
    }

    @Test void heartbeatsKeepSessionAliveBeyondIdleTimeoutAndSilenceRemovesIt() throws Exception {
        Client client = registered("heartbeat");
        long until = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(2600);
        do {
            client.send(ProtocolMessage.heartbeat());
            assertEquals(ProtocolConstants.TYPE_HEARTBEAT_ACK, client.read().getHeader().getType());
            Thread.sleep(200);
        } while (System.nanoTime() < until);
        assertTrue(sessions.getSession("heartbeat").isActive());
        int fixed = mappings.getPortByDevice("heartbeat");
        await(() -> sessions.getSession("heartbeat") == null, 4000);
        assertFalse(ports.isPortActive(fixed));
        assertEquals(Integer.valueOf(fixed), mappings.getPortByDevice("heartbeat"));
    }

    @Test void duplicateRegistrationAndDisconnectReuseFixedPort() throws Exception {
        Client first = registered("reconnect");
        int fixed = mappings.getPortByDevice("reconnect");
        Client replacement = connect();
        replacement.send(ProtocolMessage.register("reconnect", "token", 3389));
        assertEquals(fixed, JSON.parseObject(replacement.read().getBody()).getIntValue("externalPort"));
        await(() -> !first.channel.isActive(), 2000);
        replacement.send(ProtocolMessage.heartbeat());
        assertEquals(ProtocolConstants.TYPE_HEARTBEAT_ACK, replacement.read().getHeader().getType());
        assertTrue(ports.isPortActive(fixed));
        replacement.close();
        await(() -> sessions.getSession("reconnect") == null && !ports.isPortActive(fixed), 2000);
        PortMappingService reloaded = new PortMappingService(properties, repository);
        reloaded.loadPersistedMappings();
        assertEquals(Integer.valueOf(fixed), reloaded.getPortByDevice("reconnect"));
        Client again = connect();
        again.send(ProtocolMessage.register("reconnect", "token", 3389));
        assertEquals(fixed, JSON.parseObject(again.read().getBody()).getIntValue("externalPort"));
    }

    @Test void occupiedPortChangeKeepsOldListenerAndOfflineChangeDoesNotListen() throws Exception {
        Client client = registered("conflict");
        int original = mappings.getPortByDevice("conflict");
        try (ServerSocket occupied = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) {
            assertEquals(false, controller.updateMapping("conflict", Collections.singletonMap("externalPort", occupied.getLocalPort())).get("success"));
            assertTrue(ports.isPortActive(original));
            assertEquals(original, repository.findById("conflict").get().getExternalPort());
        }
        client.close();
        await(() -> sessions.getSession("conflict") == null && !ports.isPortActive(original), 2000);
        int next = freePort();
        assertEquals(true, controller.updateMapping("conflict", Collections.singletonMap("externalPort", next)).get("success"));
        assertFalse(ports.isPortActive(next));
        Client again = connect();
        again.send(ProtocolMessage.register("conflict", "token", 3389));
        assertEquals(next, JSON.parseObject(again.read().getBody()).getIntValue("externalPort"));
    }

    @Test void banRejectsReconnectAndRetainsReservation() throws Exception {
        registered("banned");
        int fixed = mappings.getPortByDevice("banned");
        lifecycle.ban("banned", "test", "test");
        Client rejected = connect();
        rejected.send(ProtocolMessage.register("banned", "token", 3389));
        assertEquals(ProtocolConstants.TYPE_ERROR, rejected.read().getHeader().getType());
        assertNull(sessions.getSession("banned"));
        assertFalse(ports.isPortActive(fixed));
        assertEquals(Integer.valueOf(fixed), mappings.getPortByDevice("banned"));
    }

    @Test void clientCloseRemovesRawAndPortConnectionIndexes() throws Exception {
        Client client = registered("close");
        int port = mappings.getPortByDevice("close");
        try (Socket external = new Socket("127.0.0.1", port)) {
            external.setSoTimeout(2000);
            external.getOutputStream().write(1);
            String id = client.read().parseDataPacket().getConnectionId();
            Channel user = raw.getUserChannelByConnectionId(id);
            assertNotNull(ports.getClientChannel(port, user));
            ProtocolMessage close = ProtocolMessage.dataWithConnectionId(id, new byte[0]);
            close.getHeader().setType(ProtocolConstants.TYPE_CLOSE_CONNECTION);
            client.send(close);
            assertEquals(-1, external.getInputStream().read());
            await(() -> raw.getUserChannelByConnectionId(id) == null && ports.getClientChannel(port, user) == null, 2000);
        }
    }

    @Test void concurrentPortChangesHaveOneWinnerWithoutClosingItsListener() throws Exception {
        registered("race-a");
        registered("race-b");
        int oldA = mappings.getPortByDevice("race-a");
        int oldB = mappings.getPortByDevice("race-b");
        int target = freePort();
        ExecutorService executor = Executors.newFixedThreadPool(2);
        CountDownLatch go = new CountDownLatch(1);
        try {
            Future<Map<String, Object>> a = executor.submit(() -> { go.await(); return controller.updateMapping("race-a", Collections.singletonMap("externalPort", target)); });
            Future<Map<String, Object>> b = executor.submit(() -> { go.await(); return controller.updateMapping("race-b", Collections.singletonMap("externalPort", target)); });
            go.countDown();
            boolean aWon = Boolean.TRUE.equals(a.get(3, TimeUnit.SECONDS).get("success"));
            boolean bWon = Boolean.TRUE.equals(b.get(3, TimeUnit.SECONDS).get("success"));
            assertNotEquals(aWon, bWon);
            assertTrue(ports.isPortActive(target));
            assertEquals(aWon ? "race-a" : "race-b", mappings.getDeviceByPort(target));
            assertTrue(ports.isPortActive(aWon ? oldB : oldA));
            assertEquals(target, repository.findById(aWon ? "race-a" : "race-b").get().getExternalPort());
        } finally { executor.shutdownNow(); }
    }

    private Client registered(String deviceId) throws Exception {
        int port = freePort();
        assertNull(lifecycle.preset(deviceId, port, 3389));
        Client client = connect();
        client.send(ProtocolMessage.register(deviceId, "token", 3389));
        assertEquals(ProtocolConstants.TYPE_REGISTER_ACK, client.read().getHeader().getType());
        return client;
    }
    private Client connect() throws Exception {
        Client client = new Client();
        client.channel = new Bootstrap().group(workers).channel(NioSocketChannel.class)
                .handler(new ChannelInitializer<SocketChannel>() {
                    @Override protected void initChannel(SocketChannel ch) {
                        ch.pipeline().addLast(new MessageDecoder(), new MessageEncoder(), new SimpleChannelInboundHandler<ProtocolMessage>() {
                            @Override protected void channelRead0(ChannelHandlerContext ctx, ProtocolMessage msg) { client.inbox.add(msg); }
                        });
                    }
                }).connect("127.0.0.1", controlPort).sync().channel();
        clients.add(client);
        return client;
    }
    private static int freePort() throws Exception {
        try (ServerSocket socket = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) { return socket.getLocalPort(); }
    }
    private static void await(BooleanSupplier condition, long timeout) throws Exception {
        long end = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeout);
        while (!condition.getAsBoolean() && System.nanoTime() < end) Thread.sleep(10);
        assertTrue(condition.getAsBoolean(), "等待真实网络状态超时");
    }
    private static class Client implements AutoCloseable {
        Channel channel;
        BlockingQueue<ProtocolMessage> inbox = new LinkedBlockingQueue<>();
        void send(ProtocolMessage message) { channel.writeAndFlush(message).syncUninterruptibly(); }
        ProtocolMessage read() throws Exception {
            ProtocolMessage msg = inbox.poll(3, TimeUnit.SECONDS);
            assertNotNull(msg, "没有收到协议响应");
            return msg;
        }
        public void close() { if (channel != null) channel.close().syncUninterruptibly(); }
    }
}
