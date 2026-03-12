package com.outview.netty.ssl;

import com.outview.config.OutViewProperties;
import io.netty.handler.ssl.ClientAuth;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.ssl.SslContextBuilder;
import io.netty.handler.ssl.SslProvider;
import io.netty.handler.ssl.util.InsecureTrustManagerFactory;
import io.netty.handler.ssl.util.SelfSignedCertificate;
import lombok.extern.slf4j.Slf4j;
import org.springframework.core.io.DefaultResourceLoader;
import org.springframework.core.io.Resource;
import org.springframework.core.io.ResourceLoader;
import org.springframework.util.StringUtils;

import javax.net.ssl.KeyManagerFactory;
import javax.net.ssl.SSLException;
import javax.net.ssl.TrustManagerFactory;
import java.io.InputStream;
import java.security.KeyStore;
import java.security.KeyStoreException;
import java.security.NoSuchAlgorithmException;
import java.security.PrivateKey;
import java.security.cert.Certificate;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.ArrayList;
import java.util.List;

/**
 * SSL 上下文工厂
 * 用于创建服务端和客户端的 SSL Context
 */
@Slf4j
public class SslContextFactory {

    private static final String CERTIFICATE_TYPE = "X.509";
    private static final String KEYSTORE_TYPE = "PKCS12";
    private static final ResourceLoader resourceLoader = new DefaultResourceLoader();

    /**
     * 创建服务端 SSL Context
     *
     * @param config SSL 配置
     * @return SslContext
     */
    public static SslContext createServerSslContext(OutViewProperties.SslConfig config) throws SSLException {
        if (!config.isEnabled()) {
            return null;
        }

        log.info("Creating server SSL context...");

        try {
            SslContextBuilder builder;

            if (config.isUseSelfSigned()) {
                // 使用自签名证书
                builder = createSelfSignedContextBuilder(config);
            } else if (StringUtils.hasText(config.getCertPath()) && StringUtils.hasText(config.getKeyPath())) {
                // 使用指定证书
                builder = createCertContextBuilder(config);
            } else {
                log.warn("SSL enabled but no certificate configured, using self-signed certificate");
                builder = createSelfSignedContextBuilder(config);
            }

            // 配置客户端认证
            if (config.isNeedClientAuth()) {
                builder.clientAuth(ClientAuth.REQUIRE);
            }

            // 配置信任证书
            if (StringUtils.hasText(config.getTrustCertPath())) {
                TrustManagerFactory tmf = createTrustManagerFactory(config.getTrustCertPath());
                builder.trustManager(tmf);
            }

            // 使用 OpenSsl 如果可用，否则使用 JDK
            builder.sslProvider(SslProvider.JDK);

            // 设置安全的加密套件
            builder.protocols("TLSv1.2", "TLSv1.3");

            SslContext sslContext = builder.build();
            log.info("Server SSL context created successfully");
            return sslContext;

        } catch (Exception e) {
            log.error("Failed to create server SSL context: {}", e.getMessage(), e);
            throw new SSLException("Failed to create SSL context", e);
        }
    }

    /**
     * 创建客户端 SSL Context
     *
     * @param config SSL 配置
     * @return SslContext
     */
    public static SslContext createClientSslContext(OutViewProperties.SslConfig config) throws SSLException {
        if (!config.isEnabled()) {
            return null;
        }

        log.info("Creating client SSL context...");

        try {
            SslContextBuilder builder = SslContextBuilder.forClient();

            // 配置信任管理器
            if (StringUtils.hasText(config.getTrustCertPath())) {
                // 使用指定的信任证书
                TrustManagerFactory tmf = createTrustManagerFactory(config.getTrustCertPath());
                builder.trustManager(tmf);
            } else {
                // 对于自签名证书或测试环境，使用不安全的信任管理器
                // 生产环境应配置 trustCertPath
                log.warn("Using InsecureTrustManagerFactory - only for development/testing!");
                builder.trustManager(InsecureTrustManagerFactory.INSTANCE);
            }

            // 配置客户端证书（如果需要双向认证）
            if (StringUtils.hasText(config.getCertPath()) && StringUtils.hasText(config.getKeyPath())) {
                KeyManagerFactory kmf = createKeyManagerFactory(config);
                builder.keyManager(kmf);
            }

            builder.sslProvider(SslProvider.JDK);
            builder.protocols("TLSv1.2", "TLSv1.3");

            SslContext sslContext = builder.build();
            log.info("Client SSL context created successfully");
            return sslContext;

        } catch (Exception e) {
            log.error("Failed to create client SSL context: {}", e.getMessage(), e);
            throw new SSLException("Failed to create client SSL context", e);
        }
    }

    /**
     * 创建自签名证书的 SslContextBuilder
     */
    private static SslContextBuilder createSelfSignedContextBuilder(OutViewProperties.SslConfig config) throws SSLException {
        try {
            log.info("Generating self-signed certificate for CN: {}", config.getSelfSignedCn());

            SelfSignedCertificate ssc = new SelfSignedCertificate(config.getSelfSignedCn());
            return SslContextBuilder.forServer(ssc.certificate(), ssc.privateKey());

        } catch (Exception e) {
            log.error("Failed to create self-signed certificate: {}", e.getMessage(), e);
            throw new SSLException("Failed to create self-signed certificate", e);
        }
    }

    /**
     * 使用指定证书创建 SslContextBuilder
     */
    private static SslContextBuilder createCertContextBuilder(OutViewProperties.SslConfig config) throws Exception {
        log.info("Loading SSL certificate from: {}", config.getCertPath());

        // 加载证书链
        X509Certificate[] certChain = loadCertificateChain(config.getCertPath());

        // 加载私钥
        PrivateKey privateKey = loadPrivateKey(config.getKeyPath(), config.getKeyPassword());

        return SslContextBuilder.forServer(privateKey, config.getKeyPassword(), certChain);
    }

    /**
     * 加载证书链
     */
    private static X509Certificate[] loadCertificateChain(String certPath) throws Exception {
        Resource resource = resourceLoader.getResource(certPath);

        try (InputStream is = resource.getInputStream()) {
            CertificateFactory cf = CertificateFactory.getInstance(CERTIFICATE_TYPE);
            List<X509Certificate> certs = new ArrayList<>();

            while (is.available() > 0) {
                Certificate cert = cf.generateCertificate(is);
                if (cert instanceof X509Certificate) {
                    certs.add((X509Certificate) cert);
                }
            }

            if (certs.isEmpty()) {
                throw new SSLException("No certificates found in: " + certPath);
            }

            log.info("Loaded {} certificate(s) from {}", certs.size(), certPath);
            return certs.toArray(new X509Certificate[0]);
        }
    }

    /**
     * 加载私钥 (PEM 格式)
     * 注意：此方法支持 PKCS#8 格式的 PEM 私钥
     */
    private static PrivateKey loadPrivateKey(String keyPath, String keyPassword) throws Exception {
        Resource resource = resourceLoader.getResource(keyPath);

        try (InputStream is = resource.getInputStream()) {
            byte[] keyBytes = new byte[is.available()];
            is.read(keyBytes);

            String keyContent = new String(keyBytes, "UTF-8");

            // 移除 PEM 头尾
            keyContent = keyContent.replace("-----BEGIN PRIVATE KEY-----", "")
                    .replace("-----END PRIVATE KEY-----", "")
                    .replace("-----BEGIN RSA PRIVATE KEY-----", "")
                    .replace("-----END RSA PRIVATE KEY-----", "")
                    .replaceAll("\\s+", "");

            // Base64 解码
            byte[] decoded = java.util.Base64.getDecoder().decode(keyContent);

            // 使用 KeyFactory 解析私钥
            java.security.spec.PKCS8EncodedKeySpec keySpec = new java.security.spec.PKCS8EncodedKeySpec(decoded);

            try {
                java.security.KeyFactory keyFactory = java.security.KeyFactory.getInstance("RSA");
                return keyFactory.generatePrivate(keySpec);
            } catch (Exception e) {
                // 尝试 EC 密钥
                java.security.KeyFactory keyFactory = java.security.KeyFactory.getInstance("EC");
                return keyFactory.generatePrivate(keySpec);
            }
        }
    }

    /**
     * 创建 KeyManagerFactory
     */
    private static KeyManagerFactory createKeyManagerFactory(OutViewProperties.SslConfig config) throws Exception {
        X509Certificate[] certChain = loadCertificateChain(config.getCertPath());
        PrivateKey privateKey = loadPrivateKey(config.getKeyPath(), config.getKeyPassword());

        // 创建临时 KeyStore
        KeyStore keyStore = KeyStore.getInstance(KEYSTORE_TYPE);
        keyStore.load(null, null);
        char[] password = config.getKeyPassword() != null ? config.getKeyPassword().toCharArray() : new char[0];
        keyStore.setKeyEntry("key", privateKey, password, certChain);

        KeyManagerFactory kmf = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm());
        kmf.init(keyStore, password);

        return kmf;
    }

    /**
     * 创建 TrustManagerFactory
     */
    private static TrustManagerFactory createTrustManagerFactory(String trustCertPath) throws Exception {
        Resource resource = resourceLoader.getResource(trustCertPath);

        try (InputStream is = resource.getInputStream()) {
            CertificateFactory cf = CertificateFactory.getInstance(CERTIFICATE_TYPE);
            KeyStore trustStore = KeyStore.getInstance(KeyStore.getDefaultType());
            trustStore.load(null, null);

            int i = 0;
            while (is.available() > 0) {
                Certificate cert = cf.generateCertificate(is);
                trustStore.setCertificateEntry("trust-" + i++, cert);
            }

            TrustManagerFactory tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
            tmf.init(trustStore);

            log.info("Loaded {} trust certificate(s) from {}", i, trustCertPath);
            return tmf;
        }
    }
}