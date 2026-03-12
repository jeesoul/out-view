package com.outview.netty.ssl;

import lombok.extern.slf4j.Slf4j;

import java.io.File;
import java.io.FileOutputStream;
import java.math.BigInteger;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.KeyStore;
import java.security.PrivateKey;
import java.security.cert.Certificate;
import java.security.cert.X509Certificate;
import java.util.Date;

import javax.security.auth.x500.X500Principal;

import sun.security.x509.AlgorithmId;
import sun.security.x509.CertificateAlgorithmId;
import sun.security.x509.CertificateSerialNumber;
import sun.security.x509.CertificateValidity;
import sun.security.x509.CertificateVersion;
import sun.security.x509.CertificateX509Key;
import sun.security.x509.X500Name;
import sun.security.x509.X509CertImpl;
import sun.security.x509.X509CertInfo;

/**
 * 自签名证书生成工具
 * 用于开发和测试环境，生产环境应使用 CA 签名的证书
 *
 * 注意：此类使用 Sun 内部 API，可能在未来的 JDK 版本中不可用
 * 生产环境建议使用 keytool 或 OpenSSL 生成证书
 */
@Slf4j
public class CertificateGenerator {

    private static final String KEY_ALGORITHM = "RSA";
    private static final int KEY_SIZE = 2048;
    private static final String SIGNATURE_ALGORITHM = "SHA256WithRSA";

    /**
     * 生成自签名证书和私钥，保存为 PKCS12 格式的 KeyStore
     *
     * @param cn           Common Name (域名或主机名)
     * @param validityDays 有效期（天）
     * @param keyStorePath KeyStore 文件路径
     * @param password      KeyStore 密码
     * @return 生成的证书
     */
    public static X509Certificate generateSelfSignedCertificate(
            String cn,
            int validityDays,
            String keyStorePath,
            String password) throws Exception {

        log.info("Generating self-signed certificate for CN: {}", cn);

        // 生成密钥对
        KeyPairGenerator keyPairGenerator = KeyPairGenerator.getInstance(KEY_ALGORITHM);
        keyPairGenerator.initialize(KEY_SIZE);
        KeyPair keyPair = keyPairGenerator.generateKeyPair();

        // 构建证书主题
        X500Name subject = new X500Name("CN=" + cn + ", O=outView, OU=Development, C=CN");
        X500Principal principal = new X500Principal(subject.getName());

        // 计算有效期
        Date notBefore = new Date();
        Date notAfter = new Date(System.currentTimeMillis() + (long) validityDays * 24 * 60 * 60 * 1000);

        // 构建证书信息
        X509CertInfo info = new X509CertInfo();
        info.set(X509CertInfo.VERSION, new CertificateVersion(CertificateVersion.V3));
        info.set(X509CertInfo.SERIAL_NUMBER, new CertificateSerialNumber(BigInteger.valueOf(System.currentTimeMillis())));
        info.set(X509CertInfo.VALIDITY, new CertificateValidity(notBefore, notAfter));
        info.set(X509CertInfo.SUBJECT, subject);
        info.set(X509CertInfo.ISSUER, subject); // 自签名，颁发者和主题相同
        info.set(X509CertInfo.KEY, new CertificateX509Key(keyPair.getPublic()));
        info.set(X509CertInfo.ALGORITHM_ID, new CertificateAlgorithmId(AlgorithmId.get(SIGNATURE_ALGORITHM)));

        // 创建证书
        X509CertImpl cert = new X509CertImpl(info);
        cert.sign(keyPair.getPrivate(), SIGNATURE_ALGORITHM);

        // 保存到 KeyStore
        KeyStore keyStore = KeyStore.getInstance("PKCS12");
        keyStore.load(null, null);
        keyStore.setKeyEntry(
                cn,
                keyPair.getPrivate(),
                password.toCharArray(),
                new Certificate[]{cert}
        );

        // 写入文件
        File keyStoreFile = new File(keyStorePath);
        keyStoreFile.getParentFile().mkdirs();

        try (FileOutputStream fos = new FileOutputStream(keyStoreFile)) {
            keyStore.store(fos, password.toCharArray());
        }

        log.info("Certificate generated and saved to: {}", keyStorePath);
        log.info("Certificate valid from {} to {}", notBefore, notAfter);

        return cert;
    }

    /**
     * 导出证书为 PEM 格式
     *
     * @param cert     证书
     * @param certPath 证书文件路径
     */
    public static void exportCertificatePem(X509Certificate cert, String certPath) throws Exception {
        try (FileOutputStream fos = new FileOutputStream(certPath)) {
            String pem = "-----BEGIN CERTIFICATE-----\n" +
                    java.util.Base64.getMimeEncoder(64, "\n".getBytes())
                            .encodeToString(cert.getEncoded()) +
                    "\n-----END CERTIFICATE-----\n";
            fos.write(pem.getBytes());
        }
        log.info("Certificate exported to: {}", certPath);
    }

    /**
     * 导出私钥为 PEM 格式 (PKCS#8)
     *
     * @param privateKey 私钥
     * @param keyPath    私钥文件路径
     */
    public static void exportPrivateKeyPem(PrivateKey privateKey, String keyPath) throws Exception {
        try (FileOutputStream fos = new FileOutputStream(keyPath)) {
            String pem = "-----BEGIN PRIVATE KEY-----\n" +
                    java.util.Base64.getMimeEncoder(64, "\n".getBytes())
                            .encodeToString(privateKey.getEncoded()) +
                    "\n-----END PRIVATE KEY-----\n";
            fos.write(pem.getBytes());
        }
        log.info("Private key exported to: {}", keyPath);
    }
}