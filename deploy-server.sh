#!/bin/bash
# Deploy outView server to 120.27.214.55

SERVER="120.27.214.55"
USER="root"  # 修改为实际用户名
DEPLOY_DIR="/opt/outview"

echo "Deploying outView v1.2.0 to ${SERVER}..."

# 1. 上传服务端文件
echo "Uploading server files..."
scp release/outview-1.2.0/outview-server.jar ${USER}@${SERVER}:${DEPLOY_DIR}/
scp release/outview-1.2.0/application.yml ${USER}@${SERVER}:${DEPLOY_DIR}/
scp release/outview-1.2.0/start.sh ${USER}@${SERVER}:${DEPLOY_DIR}/

# 2. 创建 systemd 服务
echo "Creating systemd service..."
ssh ${USER}@${SERVER} << 'EOF'
cat > /etc/systemd/system/outview.service << 'SERVICE'
[Unit]
Description=outView Remote Desktop Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/outview
ExecStart=/usr/bin/java -jar /opt/outview/outview-server.jar
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable outview
systemctl restart outview
systemctl status outview
EOF

echo "Deployment complete!"
echo "Check status: ssh ${USER}@${SERVER} 'systemctl status outview'"
echo "View logs: ssh ${USER}@${SERVER} 'journalctl -u outview -f'"
