package com.outview.service;

import com.outview.config.OutViewProperties;
import com.outview.entity.SysUser;
import com.outview.repository.SysUserRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

/**
 * 用户服务
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class SysUserService {

    private final SysUserRepository userRepository;
    private final PasswordEncoder passwordEncoder;
    private final OutViewProperties properties;

    /**
     * 初始化默认管理员账号
     */
    @Transactional
    public void initDefaultAdmin() {
        String username = properties.getAdmin().getUsername();
        String password = properties.getAdmin().getPassword();

        if (!userRepository.existsByUsername(username)) {
            SysUser admin = SysUser.builder()
                    .username(username)
                    .password(passwordEncoder.encode(password))
                    .role("ADMIN")
                    .enabled(true)
                    .createTime(LocalDateTime.now())
                    .build();
            userRepository.save(admin);
            log.info("管理员账号已创建: {}", username);
        }
    }

    /**
     * 根据用户名查询
     */
    public SysUser getByUsername(String username) {
        return userRepository.findByUsername(username).orElse(null);
    }

    /**
     * 修改密码
     */
    @Transactional
    public boolean changePassword(String username, String oldPassword, String newPassword) {
        SysUser user = getByUsername(username);
        if (user == null) {
            return false;
        }

        // 验证旧密码
        if (!passwordEncoder.matches(oldPassword, user.getPassword())) {
            return false;
        }

        // 更新密码
        user.setPassword(passwordEncoder.encode(newPassword));
        userRepository.save(user);
        log.info("用户 {} 密码已修改", username);
        return true;
    }

    /**
     * 更新最后登录时间
     */
    @Transactional
    public void updateLastLoginTime(String username) {
        SysUser user = getByUsername(username);
        if (user != null) {
            user.setLastLoginTime(LocalDateTime.now());
            userRepository.save(user);
        }
    }
}