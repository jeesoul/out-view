package com.outview.config;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
@ConfigurationProperties(prefix = "outview-admin")
public class AdminProperties {

    private List<AdminUser> users = new ArrayList<>();

    public List<AdminUser> getUsers() { return users; }
    public void setUsers(List<AdminUser> users) { this.users = users; }

    public static class AdminUser {
        private String username;
        private String password;
        private String role = "ADMIN";

        public String getUsername() { return username; }
        public void setUsername(String username) { this.username = username; }
        public String getPassword() { return password; }
        public void setPassword(String password) { this.password = password; }
        public String getRole() { return role; }
        public void setRole(String role) { this.role = role; }
    }
}
