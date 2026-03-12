package com.outview.config;

import lombok.Data;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

/**
 * outView 服务配置属性
 */
@Data
@Component
@ConfigurationProperties(prefix = "outview")
public class OutViewProperties {

    /**
     * 控制端口 (客户端注册/心跳)
     */
    private int controlPort = 7000;

    /**
     * 数据端口范围起始
     */
    private int dataPortStart = 6000;

    /**
     * 数据端口范围结束
     */
    private int dataPortEnd = 6500;

    /**
     * 心跳超时时间 (秒)
     */
    private int heartbeatTimeout = 90;

    /**
     * 心跳间隔 (秒)
     */
    private int heartbeatInterval = 30;

    /**
     * Token 有效期 (天)
     */
    private int tokenExpireDays = 30;

    /**
     * SSL/TLS 配置
     */
    private SslConfig ssl = new SslConfig();

    /**
     * 管理员账号配置
     */
    private AdminConfig admin = new AdminConfig();

    @Data
    public static class SslConfig {
        /**
         * 是否启用 SSL/TLS
         */
        private boolean enabled = false;

        /**
         * 证书文件路径 (支持 classpath: 和 file: 前缀)
         */
        private String certPath;

        /**
         * 私钥文件路径 (支持 classpath: 和 file: 前缀)
         */
        private String keyPath;

        /**
         * 私钥密码 (可选)
         */
        private String keyPassword;

        /**
         * 信任证书文件路径 (用于客户端证书验证，可选)
         */
        private String trustCertPath;

        /**
         * 是否需要客户端证书认证
         */
        private boolean needClientAuth = false;

        /**
         * 是否使用自签名证书 (仅用于开发/测试环境)
         */
        private boolean useSelfSigned = false;

        /**
         * 自签名证书的 CN (Common Name)
         */
        private String selfSignedCn = "outview-server";

        /**
         * 自签名证书有效期 (天)
         */
        private int selfSignedValidityDays = 365;
    }

    @Data
    public static class AdminConfig {
        /**
         * 管理员用户名
         */
        private String username = "admin";

        /**
         * 管理员密码
         */
        private String password = "admin123";
    }
}