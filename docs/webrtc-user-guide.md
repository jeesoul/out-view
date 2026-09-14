# WebRTC 实验模块说明

当前稳定链路是 TCP。GUI 新安装默认关闭实验 WebRTC；开启实验信令也不代表 RDP 数据已走 P2P。

仓库包含 Go pion PeerConnection/DataChannel、Java IPC代理、Sidecar 生命周期类、信令消息以及组件测试。尚待接通的环节包括实际 Netty 管线、Sidecar 自动启动及启动参数/Windows IPC 对齐、Sidecar 命令路由、两端真实业务数据与失败恢复。

历史设计针对服务器到被控端传输，服务器仍中转。控制端到被控端直接 P2P 需要额外组件，不属于当前已交付能力。

GUI 的 STUN/TURN 实验设置保存在用户配置目录 `outview/webrtc.json`。正常使用固定端口及 RDP 不需要启动 Sidecar。

本模块的单元测试和进程内 POC 不能替代跨进程、真实 RDP 或公网 NAT 测试。当前架构见 [ARCHITECTURE.md](ARCHITECTURE.md)。
