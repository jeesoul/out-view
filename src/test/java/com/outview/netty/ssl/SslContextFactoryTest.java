package com.outview.netty.ssl;

import com.outview.config.OutViewProperties;
import io.netty.handler.ssl.SslContext;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * SSL Context 工厂测试
 */
class SslContextFactoryTest {

    @Test
    void testCreateSelfSignedServerSslContext() throws Exception {
        OutViewProperties.SslConfig config = new OutViewProperties.SslConfig();
        config.setEnabled(true);
        config.setUseSelfSigned(true);
        config.setSelfSignedCn("test-server");

        SslContext sslContext = SslContextFactory.createServerSslContext(config);

        assertNotNull(sslContext, "SSL Context should not be null");
        assertTrue(sslContext.isServer(), "Should be a server SSL context");
    }

    @Test
    void testCreateClientSslContext() throws Exception {
        OutViewProperties.SslConfig config = new OutViewProperties.SslConfig();
        config.setEnabled(true);

        SslContext sslContext = SslContextFactory.createClientSslContext(config);

        assertNotNull(sslContext, "Client SSL Context should not be null");
        assertFalse(sslContext.isServer(), "Should be a client SSL context");
    }

    @Test
    void testDisabledSslReturnsNull() throws Exception {
        OutViewProperties.SslConfig config = new OutViewProperties.SslConfig();
        config.setEnabled(false);

        SslContext serverContext = SslContextFactory.createServerSslContext(config);
        SslContext clientContext = SslContextFactory.createClientSslContext(config);

        assertNull(serverContext, "Server SSL Context should be null when disabled");
        assertNull(clientContext, "Client SSL Context should be null when disabled");
    }
}