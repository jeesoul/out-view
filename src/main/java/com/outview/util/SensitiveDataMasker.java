package com.outview.util;

/**
 * 敏感数据脱敏工具类
 * 用于日志输出时对敏感信息进行脱敏处理
 */
public class SensitiveDataMasker {

    /**
     * 对 Token 进行脱敏
     * 保留前4位和后4位，中间用星号替换
     *
     * @param token 原始 token
     * @return 脱敏后的 token
     */
    public static String maskToken(String token) {
        if (token == null) {
            return "null";
        }
        if (token.length() <= 8) {
            return "****";
        }
        return token.substring(0, 4) + "****" + token.substring(token.length() - 4);
    }

    /**
     * 对密码进行脱敏
     * 全部用星号替换
     *
     * @param password 原始密码
     * @return 脱敏后的密码
     */
    public static String maskPassword(String password) {
        if (password == null) {
            return "null";
        }
        if (password.isEmpty()) {
            return "";
        }
        int length = Math.min(password.length(), 8);
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < length; i++) {
            sb.append("*");
        }
        return sb.toString();
    }

    /**
     * 对 IP 地址进行脱敏
     * 隐藏最后一段
     *
     * @param ip 原始 IP
     * @return 脱敏后的 IP
     */
    public static String maskIp(String ip) {
        if (ip == null) {
            return "null";
        }
        int lastDot = ip.lastIndexOf('.');
        if (lastDot > 0) {
            return ip.substring(0, lastDot + 1) + "***";
        }
        return "***";
    }

    /**
     * 对手机号进行脱敏
     * 保留前3位和后4位
     *
     * @param phone 原始手机号
     * @return 脱敏后的手机号
     */
    public static String maskPhone(String phone) {
        if (phone == null) {
            return "null";
        }
        if (phone.length() <= 7) {
            return "****";
        }
        return phone.substring(0, 3) + "****" + phone.substring(phone.length() - 4);
    }

    /**
     * 对邮箱进行脱敏
     * 保留前2位和@后的域名
     *
     * @param email 原始邮箱
     * @return 脱敏后的邮箱
     */
    public static String maskEmail(String email) {
        if (email == null) {
            return "null";
        }
        int atIndex = email.indexOf('@');
        if (atIndex <= 2) {
            return "***" + (atIndex > 0 ? email.substring(atIndex) : "");
        }
        return email.substring(0, 2) + "***" + email.substring(atIndex);
    }

    /**
     * 对设备ID进行部分脱敏
     * 保留前4位和后4位
     *
     * @param deviceId 原始设备ID
     * @return 脱敏后的设备ID
     */
    public static String maskDeviceId(String deviceId) {
        if (deviceId == null) {
            return "null";
        }
        if (deviceId.length() <= 8) {
            return "****";
        }
        return deviceId.substring(0, 4) + "****" + deviceId.substring(deviceId.length() - 4);
    }

    /**
     * 对密钥/私钥内容进行脱敏
     * 只显示长度信息
     *
     * @param keyContent 密钥内容
     * @return 脱敏后的信息
     */
    public static String maskKey(String keyContent) {
        if (keyContent == null) {
            return "null";
        }
        return "[KEY_LENGTH=" + keyContent.length() + "]";
    }
}