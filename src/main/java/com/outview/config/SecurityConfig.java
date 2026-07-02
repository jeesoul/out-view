package com.outview.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.core.userdetails.User;
import org.springframework.security.core.userdetails.UserDetails;
import org.springframework.security.crypto.password.NoOpPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.security.provisioning.InMemoryUserDetailsManager;
import org.springframework.security.web.SecurityFilterChain;

import java.util.ArrayList;
import java.util.List;

@Configuration
@EnableWebSecurity
public class SecurityConfig {

    private final AdminProperties adminProperties;

    public SecurityConfig(AdminProperties adminProperties) {
        this.adminProperties = adminProperties;
    }

    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http
            .csrf().disable()
            .authorizeRequests()
                // 健康检查和 Netty 相关接口不需要鉴权
                .antMatchers("/health").permitAll()
                // 管理页面和 API 需要登录
                .antMatchers("/", "/index.html", "/api/**").authenticated()
                .anyRequest().authenticated()
            .and()
            .httpBasic()
            .and()
            .formLogin()
                .loginPage("/login.html")
                .permitAll()
            .and()
            .logout()
                .permitAll();
        return http.build();
    }

    @Bean
    public InMemoryUserDetailsManager userDetailsManager() {
        List<UserDetails> users = new ArrayList<>();
        for (AdminProperties.AdminUser u : adminProperties.getUsers()) {
            users.add(User.withUsername(u.getUsername())
                .password(u.getPassword())
                .roles(u.getRole())
                .build());
        }
        // 如果 yml 没配置任何账号，提供一个默认账号
        if (users.isEmpty()) {
            users.add(User.withUsername("admin")
                .password("outview123")
                .roles("ADMIN")
                .build());
        }
        return new InMemoryUserDetailsManager(users);
    }

    @Bean
    @SuppressWarnings("deprecation")
    public PasswordEncoder passwordEncoder() {
        // 明文密码，方便在 yml 中直接配置
        return NoOpPasswordEncoder.getInstance();
    }
}
