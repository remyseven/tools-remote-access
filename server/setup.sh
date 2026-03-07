#!/bin/bash
# Remotely signaling server — one-time setup script for Ubuntu 24.04
# Run as root or with sudo: bash setup.sh

set -e

echo "==> Installing Node.js 20..."
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

echo "==> Cloning repo..."
cd /opt
git clone https://github.com/remyseven/tools-remote-access.git remotely
cd remotely/server
npm install --omit=dev

echo "==> Creating systemd service..."
cat > /etc/systemd/system/remotely.service << 'EOF'
[Unit]
Description=Remotely signaling server
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/opt/remotely/server
ExecStart=/usr/bin/node server.js
Restart=always
RestartSec=5
Environment=PORT=3000
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable remotely
systemctl start remotely

echo ""
echo "==> Done! Server is running on port 3000."
echo "    Check status : systemctl status remotely"
echo "    View logs    : journalctl -u remotely -f"
