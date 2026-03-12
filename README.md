# outView - 远程桌面内网穿透系统

## 项目状态

| 模块 | 状态 | 说明 |
|------|------|------|
| 服务端核心 | ✅ 完成 | Spring Boot + Netty |
| 数据端口监听 | ✅ 完成 | 动态分配 6000-6500 |
| 数据转发 | ✅ 完成 | 双向转发 + 连接ID |
| SSL/TLS | ✅ 完成 | 自签名证书支持 |
| Go客户端核心 | ✅ 完成 | 协议 + 心跳 + 转发 |
| GUI界面 | ✅ 完成 | fyne 跨平台UI |
| exe打包 | ⏳ 待Go环境 | 需安装Go编译 |

## 快速开始

### 1. 启动服务端

```bash
cd D:\claudeCodeSpace\java\out-view

# 编译
set JAVA_HOME=C:\Program Files\Java\jdk-1.8
D:\java\maven\apache-maven-3.8.8\bin\mvn.cmd package -Dmaven.test.skip=true -s D:\java\maven\my-settings.xml

# 运行
java -jar target\outview-server.jar
```

### 2. 生成 Token

访问 http://localhost:8080/index.html，点击"生成新 Token"

### 3. 编译客户端 (需要 Go)

```bash
cd D:\claudeCodeSpace\java\out-view\client

# 安装依赖
go mod tidy

# 编译 GUI 版本
build-gui.bat

# 或编译 CLI 版本
build.bat
```

### 4. 使用客户端

**GUI 版本** (推荐):
- 双击运行 `outview-client-gui-windows-amd64.exe`
- 输入服务器地址、端口、Token
- 点击"连接"

**CLI 版本**:
```bash
outview-client-windows-amd64.exe -host 服务器IP -port 7000 -device-id 设备ID -token 密钥
```

### 5. 连接远程桌面

1. 打开 Windows 远程桌面连接 (mstsc)
2. 输入 `服务器IP:分配的端口` (如 `192.168.1.100:6001`)
3. 输入家庭电脑的用户名和密码

## 目录结构

```
out-view/
├── src/main/java/com/outview/    # 服务端 Java 代码
│   ├── config/                   # 配置类
│   ├── controller/               # REST API
│   ├── entity/                   # 实体类
│   ├── netty/                    # Netty 核心代码
│   │   ├── handler/              # 处理器
│   │   └── ssl/                  # SSL 相关
│   ├── protocol/                 # 协议相关
│   ├── service/                  # 服务层
│   └── client/                   # Java 测试客户端
├── client/                       # Go 客户端
│   ├── cmd/
│   │   ├── outview-client/       # CLI 入口
│   │   └── outview-gui/          # GUI 入口
│   ├── internal/
│   │   ├── protocol/             # 协议实现
│   │   └── client/               # 客户端核心
│   ├── build.bat                 # CLI 构建脚本
│   └── build-gui.bat             # GUI 构建脚本
├── target/outview-server.jar     # 服务端可执行文件
└── pom.xml                       # Maven 配置
```

## 端口说明

| 端口 | 用途 |
|------|------|
| 8080 | HTTP API / 管理后台 |
| 7000 | 客户端注册/心跳 |
| 6000-6500 | 数据转发端口 |

## 依赖要求

### 服务端
- JDK 8+
- Maven 3.6+

### 客户端编译
- Go 1.21+
- CGO (GUI版本需要)

## 下载 Go

如果需要编译客户端，请下载安装 Go:
- 官网: https://go.dev/dl/
- 国内镜像: https://golang.google.cn/dl/

Windows 推荐下载 `go1.21.x.windows-amd64.msi` 直接安装。

## 常见问题

### Q: Go 未安装怎么办?
A: 客户端代码已完成，但需要安装 Go 才能编译。安装 Go 后运行 `build.bat` 即可。

### Q: GUI 编译失败?
A: GUI 版本需要 CGO 支持。如果编译失败，可以使用 CLI 版本，或在有 MinGW 环境下编译。

### Q: 连接远程桌面失败?
A: 检查:
1. 服务端是否启动
2. 客户端是否成功注册
3. 防火墙是否开放数据端口