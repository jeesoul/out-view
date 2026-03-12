package com.outview.config;

import com.outview.service.SysUserService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.stereotype.Component;

/**
 * 应用启动初始化
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class AppInitializer implements ApplicationRunner {

    private final SysUserService userService;

    @Override
    public void run(ApplicationArguments args) {
        // 初始化默认管理员账号
        userService.initDefaultAdmin();
        log.info("应用初始化完成");
    }
}