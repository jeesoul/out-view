package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.controller.DeviceController;
import com.outview.netty.DataChannelInitializer;
import com.outview.netty.handler.AuthHandler;
import com.outview.netty.handler.RawDataHandler;
import com.outview.netty.handler.ProxyHandler;
import io.netty.buffer.Unpooled;
import com.outview.protocol.ProtocolConstants;
import com.outview.protocol.ProtocolMessage;
import com.outview.repository.PortMappingRepository;
import io.netty.channel.DefaultChannelId;
import io.netty.channel.embedded.EmbeddedChannel;
import io.netty.channel.nio.NioEventLoopGroup;
import org.junit.jupiter.api.*;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

import java.net.InetAddress;
import java.net.ServerSocket;
import java.util.Collections;
import java.util.concurrent.*;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;

/** 真实 H2 持久化与实际业务组件回归；不启动应用或接触文件数据库。 */
@DataJpaTest(properties = "spring.jpa.show-sql=false")
@Transactional(propagation = Propagation.NOT_SUPPORTED)
class ConnectionLifecycleTest {
    @Autowired PortMappingRepository repository;
    @Autowired com.outview.repository.BannedDeviceRepository banRepository;
    OutViewProperties properties;
    PortMappingService mappings;
    SessionStore sessions;
    NioEventLoopGroup workers;
    DataPortService ports;

    @BeforeEach void setup() {
        repository.deleteAll();
        banRepository.deleteAll();
        properties = new OutViewProperties();
        properties.setDataPortStart(1024);
        properties.setDataPortEnd(65535);
        properties.setBindAddress("127.0.0.1");
        mappings = new PortMappingService(properties, repository);
        sessions = new SessionStore(properties);
        workers = new NioEventLoopGroup(1);
        ports = new DataPortService(mock(DataChannelInitializer.class), properties, workers);
    }

    @AfterEach void cleanup() {
        ports.shutdown();
        workers.shutdownGracefully(0, 1, java.util.concurrent.TimeUnit.SECONDS).syncUninterruptibly();
    }

    @Test void replacementClosesOldChannelAndOldDisconnectPreservesNewSession() {
        EmbeddedChannel old = new EmbeddedChannel(DefaultChannelId.newInstance());
        EmbeddedChannel current = new EmbeddedChannel(DefaultChannelId.newInstance());
        try {
            sessions.register("device", "token", old, 3389, 25000);
            sessions.register("device", "token", current, 3389, 25000);
            assertFalse(old.isActive(), "替换注册必须关闭旧控制连接");
            sessions.removeSessionByChannel(old);
            assertSame(current, sessions.getSession("device").getChannel());
        } finally { old.finishAndReleaseAll(); current.finishAndReleaseAll(); }
    }

    @Test void failedPersistenceDoesNotReservePortInMemory() {
        String tooLongId = String.join("", Collections.nCopies(65, "x"));
        assertThrows(RuntimeException.class, () -> mappings.setFixedPort(tooLongId, 25001, 3389));
        assertNull(mappings.getPortByDevice(tooLongId));
        assertNull(mappings.getDeviceByPort(25001));
        assertEquals(0, repository.count());
    }

    @Test void presetRejectsInvalidTargetPort() {
        assertNotNull(mappings.setFixedPort("device", 25001, 0));
        assertEquals(0, repository.count());
    }

    @Test void mappingUpdateUsesConfiguredRange() {
        properties.setDataPortStart(25000);
        properties.setDataPortEnd(25010);
        assertNull(mappings.setFixedPort("device", 25001, 3389));
        assertThrows(IllegalArgumentException.class, () -> mappings.updateExternalPort("device", 25011));
        assertEquals(Integer.valueOf(25001), mappings.getPortByDevice("device"));
    }

    @Test void occupiedFixedPortSurvivesFailedRegistration() throws Exception {
        try (ServerSocket occupied = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) {
            int fixedPort = occupied.getLocalPort();
            assertNull(mappings.setFixedPort("device", fixedPort, 3389));
            AuthHandler auth = new AuthHandler(new DeviceLifecycleService(sessions, mappings, ports, mock(BanService.class)));
            EmbeddedChannel channel = new EmbeddedChannel(DefaultChannelId.newInstance(), auth);
            try {
                channel.writeInbound(ProtocolMessage.register("device", "token", 3389));
                ProtocolMessage response = channel.readOutbound();
                assertEquals(ProtocolConstants.TYPE_ERROR, response.getHeader().getType());
                assertEquals(Integer.valueOf(fixedPort), mappings.getPortByDevice("device"));
                assertTrue(repository.findById("device").isPresent());
                assertFalse(mappings.getMapping(fixedPort).isOnline());
            } finally { channel.finishAndReleaseAll(); }
        }
    }

    @Test void failedOnlinePortChangePreservesMappingAndSession() throws Exception {
        try (ServerSocket occupied = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) {
            int newPort = occupied.getLocalPort();
            int oldPort = newPort == 25001 ? 25002 : 25001;
            assertNull(mappings.setFixedPort("device", oldPort, 3389));
            EmbeddedChannel channel = new EmbeddedChannel(DefaultChannelId.newInstance());
            try {
                sessions.register("device", "token", channel, 3389, oldPort);
                DeviceController controller = new DeviceController(sessions, mappings, new DeviceLifecycleService(sessions, mappings, ports, mock(BanService.class)), mock(BanService.class));
                assertEquals(false, controller.updateMapping("device", Collections.singletonMap("externalPort", newPort)).get("success"));
                assertEquals(Integer.valueOf(oldPort), mappings.getPortByDevice("device"));
                assertEquals(oldPort, repository.findById("device").get().getExternalPort());
                assertEquals(oldPort, sessions.getSession("device").getExternalPort());
            } finally { channel.finishAndReleaseAll(); }
        }
    }

    @Test void cachedPortLookupDoesNotWaitForDatabaseWrite() throws Exception {
        PortMappingRepository delayed = org.mockito.Mockito.mock(PortMappingRepository.class);
        java.util.concurrent.CountDownLatch writing = new java.util.concurrent.CountDownLatch(1);
        java.util.concurrent.CountDownLatch release = new java.util.concurrent.CountDownLatch(1);
        org.mockito.Mockito.when(delayed.saveAndFlush(org.mockito.ArgumentMatchers.any())).thenAnswer(call -> {
            com.outview.entity.PortMapping mapping = call.getArgument(0);
            if (mapping.getDeviceId().equals("slow")) {
                writing.countDown();
                assertTrue(release.await(5, java.util.concurrent.TimeUnit.SECONDS));
            }
            return repository.saveAndFlush(mapping);
        });
        PortMappingService cache = new PortMappingService(properties, delayed);
        assertNull(cache.setFixedPort("existing", 25001, 3389));
        java.util.concurrent.ExecutorService executor = java.util.concurrent.Executors.newFixedThreadPool(2);
        java.util.concurrent.Future<?> writer = executor.submit(() -> cache.setFixedPort("slow", 25002, 3389));
        try {
            assertTrue(writing.await(2, java.util.concurrent.TimeUnit.SECONDS));
            java.util.concurrent.Future<String> reader = executor.submit(() -> cache.getDeviceByPort(25001));
            assertEquals("existing", assertDoesNotThrow(() -> reader.get(300, java.util.concurrent.TimeUnit.MILLISECONDS),
                    "Netty读取已有映射不应等待另一个设备的数据库写入"));
        } finally {
            release.countDown();
            writer.get(2, java.util.concurrent.TimeUnit.SECONDS);
            executor.shutdownNow();
        }
    }

    @Test void occupiedControlPortFailsServerStartup() throws Exception {
        try (ServerSocket occupied = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) {
            properties.setControlPort(occupied.getLocalPort());
            properties.setBindAddress("127.0.0.1");
            com.outview.netty.NettyServer server = new com.outview.netty.NettyServer(properties,
                    mock(com.outview.netty.ControlChannelInitializer.class), workers);
            try {
                assertThrows(IllegalStateException.class, server::run,
                        "控制端口启动失败必须使应用失败，不能只有HTTP存活却没有隧道");
            } finally { server.shutdown(); }
        }
    }

    @Test void controllerBanHoldsDeviceLockUntilBanIsPersisted() throws Exception {
        CountDownLatch savingBan = new CountDownLatch(1);
        CountDownLatch releaseBan = new CountDownLatch(1);
        BanService slowBans = new BanService(banRepository) {
            @Override public void ban(String deviceId, String operator, String reason) {
                savingBan.countDown();
                try { assertTrue(releaseBan.await(3, TimeUnit.SECONDS)); }
                catch (InterruptedException e) { Thread.currentThread().interrupt(); throw new IllegalStateException(e); }
                super.ban(deviceId, operator, reason);
            }
        };
        DeviceLifecycleService lifecycle = new DeviceLifecycleService(sessions, mappings, ports, slowBans);
        DeviceController controller = new DeviceController(sessions, mappings, lifecycle, slowBans);
        EmbeddedChannel original = new EmbeddedChannel(DefaultChannelId.newInstance());
        EmbeddedChannel reconnect = new EmbeddedChannel(DefaultChannelId.newInstance());
        int port;
        try (ServerSocket reservation = new ServerSocket(0, 1, InetAddress.getLoopbackAddress())) { port = reservation.getLocalPort(); }
        assertNull(mappings.setFixedPort("ban-race", port, 3389));
        sessions.register("ban-race", "token", original, 3389, port);
        ExecutorService executor = Executors.newFixedThreadPool(2);
        Future<?> banning = executor.submit(() -> controller.banDevice("ban-race", Collections.singletonMap("reason", "test")));
        Future<?> registering = null;
        try {
            assertTrue(savingBan.await(2, TimeUnit.SECONDS));
            CountDownLatch attempting = new CountDownLatch(1);
            registering = executor.submit(() -> {
                attempting.countDown();
                return lifecycle.register("ban-race", "token", reconnect, 3389);
            });
            assertTrue(attempting.await(1, TimeUnit.SECONDS));
            Future<?> pending = registering;
            assertThrows(TimeoutException.class, () -> pending.get(200, TimeUnit.MILLISECONDS),
                    "封禁尚未落库时重连必须等待同设备生命周期锁");
            releaseBan.countDown();
            banning.get(2, TimeUnit.SECONDS);
            ExecutionException rejected = assertThrows(ExecutionException.class, () -> pending.get(2, TimeUnit.SECONDS));
            assertTrue(rejected.getCause().getMessage().contains("banned"));
            assertTrue(banRepository.existsByDeviceId("ban-race"));
            assertNull(sessions.getSession("ban-race"));
            assertEquals(Integer.valueOf(port), mappings.getPortByDevice("ban-race"));
        } finally {
            releaseBan.countDown();
            banning.get(2, TimeUnit.SECONDS);
            if (registering != null) { try { registering.get(2, TimeUnit.SECONDS); } catch (ExecutionException expected) { } }
            executor.shutdownNow();
            original.finishAndReleaseAll(); reconnect.finishAndReleaseAll();
        }
    }

    @Test void externalCloseNotifiesClient() {
        withDataConnection((owner, external, intruder, id) -> {
            external.close();
            ProtocolMessage close = owner.readOutbound();
            assertNotNull(close, "外部断线必须通知客户端关闭对应本地连接");
            assertEquals(ProtocolConstants.TYPE_CLOSE_CONNECTION, close.getHeader().getType());
            assertEquals(id, close.parseCloseConnectionId());
        });
    }

    @Test void concurrentCleanupKeepsNotificationOwnershipTogether() {
        withDataConnection((owner, external, intruder, id) -> {
            RawDataHandler raw = external.pipeline().get(RawDataHandler.class);
            CountDownLatch firstRemovedSession = new CountDownLatch(1);
            CountDownLatch releaseFirst = new CountDownLatch(1);
            @SuppressWarnings("unchecked")
            java.util.Map<io.netty.channel.Channel, com.outview.entity.ClientSession> original =
                    (java.util.Map<io.netty.channel.Channel, com.outview.entity.ClientSession>)
                            org.springframework.test.util.ReflectionTestUtils.getField(raw, "userToSessionMap");
            // 精确冻结在首次移除 session 后，让另一路清理尝试抢走 connectionId。
            ConcurrentHashMap<io.netty.channel.Channel, com.outview.entity.ClientSession> controlled =
                    new ConcurrentHashMap<io.netty.channel.Channel, com.outview.entity.ClientSession>(original) {
                        @Override public com.outview.entity.ClientSession remove(Object key) {
                            com.outview.entity.ClientSession removed = super.remove(key);
                            if (removed != null) {
                                firstRemovedSession.countDown();
                                try { assertTrue(releaseFirst.await(3, TimeUnit.SECONDS)); }
                                catch (InterruptedException e) { Thread.currentThread().interrupt(); throw new IllegalStateException(e); }
                            }
                            return removed;
                        }
                    };
            org.springframework.test.util.ReflectionTestUtils.setField(raw, "userToSessionMap", controlled);
            ExecutorService executor = Executors.newFixedThreadPool(2);
            Future<?> first = executor.submit(() -> org.springframework.test.util.ReflectionTestUtils.invokeMethod(raw, "cleanupConnection", external, true));
            try {
                assertTrue(firstRemovedSession.await(2, TimeUnit.SECONDS));
                Future<?> second = executor.submit(() -> org.springframework.test.util.ReflectionTestUtils.invokeMethod(raw, "cleanupConnection", external, true));
                second.get(1, TimeUnit.SECONDS);
                releaseFirst.countDown();
                first.get(1, TimeUnit.SECONDS);
                ProtocolMessage close = owner.readOutbound();
                assertNotNull(close, "并发清理必须由同一个拥有者发送唯一关闭通知");
                assertEquals(ProtocolConstants.TYPE_CLOSE_CONNECTION, close.getHeader().getType());
                assertEquals(id, close.parseCloseConnectionId());
                assertNull(owner.readOutbound(), "同一条连接只能发送一次关闭通知");
                assertNull(raw.getUserChannelByConnectionId(id));
                assertNull(raw.getSession(external));
            } catch (Exception e) { throw new AssertionError(e); }
            finally {
                releaseFirst.countDown();
                executor.shutdownNow();
            }
        });
    }

    @Test void anotherDeviceCannotWriteToOwnedConnection() {
        withDataConnection((owner, external, intruder, id) -> {
            intruder.writeInbound(ProtocolMessage.dataWithConnectionId(id, new byte[]{9}));
            assertNull(external.readOutbound(), "其他设备不得向该连接注入数据");
            owner.writeInbound(ProtocolMessage.dataWithConnectionId(id, new byte[]{7}));
            io.netty.buffer.ByteBuf received = external.readOutbound();
            assertNotNull(received);
            assertEquals(7, received.readByte());
            received.release();
        });
    }

    @Test void anotherDeviceCannotCloseOwnedConnection() {
        withDataConnection((owner, external, intruder, id) -> {
            ProtocolMessage close = ProtocolMessage.dataWithConnectionId(id, new byte[0]);
            close.getHeader().setType(ProtocolConstants.TYPE_CLOSE_CONNECTION);
            intruder.writeInbound(close);
            assertTrue(external.isActive(), "其他设备不得关闭该连接");
            owner.writeInbound(close);
            assertFalse(external.isActive());
        });
    }

    @Test void slowExternalConsumerClosesOnlyItsConnectionAndNotifiesClient() {
        withDataConnection((owner, external, intruder, id) -> {
            java.util.List<io.netty.channel.ChannelPromise> pending = new java.util.ArrayList<>();
            external.pipeline().addFirst(new io.netty.channel.ChannelOutboundHandlerAdapter() {
                @Override public void write(io.netty.channel.ChannelHandlerContext ctx, Object message,
                                            io.netty.channel.ChannelPromise promise) {
                    // 模拟真实 socket 写队列不完成；释放测试捕获的缓冲区，保留未完成写操作。
                    io.netty.util.ReferenceCountUtil.release(message);
                    pending.add(promise);
                }
            });
            try {
                byte[] payload = new byte[1024 * 1024];
                for (int i = 0; i < 6; i++) owner.writeInbound(ProtocolMessage.dataWithConnectionId(id, payload));
                assertFalse(external.isActive(), "慢接收连接必须有待写内存上限");
                assertTrue(owner.isActive(), "单条转发拥塞不得关闭控制连接");
                ProtocolMessage close = owner.readOutbound();
                assertNotNull(close);
                assertEquals(ProtocolConstants.TYPE_CLOSE_CONNECTION, close.getHeader().getType());
            } finally {
                for (io.netty.channel.ChannelPromise promise : pending) promise.tryFailure(new java.io.IOException("test closed"));
            }
        });
    }

    private interface DataCheck { void check(EmbeddedChannel owner, EmbeddedChannel external, EmbeddedChannel intruder, String id); }

    private void withDataConnection(DataCheck check) {
        // 此组仅隔离 TCP acceptor；实际网络与监听索引由 RealLifecycleIntegrationTest 覆盖。
        RawDataHandler raw = new RawDataHandler(sessions, mappings, mock(DataPortService.class));
        ProxyHandler proxy = new ProxyHandler(sessions, mappings, raw);
        EmbeddedChannel owner = new EmbeddedChannel(DefaultChannelId.newInstance(), proxy);
        EmbeddedChannel intruder = new EmbeddedChannel(DefaultChannelId.newInstance(), proxy);
        mappings.setFixedPort("owner", 25001, 3389);
        sessions.register("owner", "token", owner, 3389, 25001);
        sessions.register("intruder", "token", intruder, 3389, 25002);
        EmbeddedChannel external = new EmbeddedChannel(DefaultChannelId.newInstance(), raw) {
            @Override public java.net.SocketAddress localAddress() {
                return new java.net.InetSocketAddress("127.0.0.1", 25001);
            }
        };
        try {
            external.writeInbound(Unpooled.wrappedBuffer(new byte[]{1}));
            ProtocolMessage data = owner.readOutbound();
            assertNotNull(data);
            check.check(owner, external, intruder, data.parseDataPacket().getConnectionId());
        } finally {
            external.finishAndReleaseAll(); owner.finishAndReleaseAll(); intruder.finishAndReleaseAll();
        }
    }
}
