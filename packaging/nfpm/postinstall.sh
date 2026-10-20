#!/bin/sh
set -e
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload || true
fi
echo "levelrail-agent installed. Edit /etc/levelrail-agent/agent.env, then run: systemctl enable --now levelrail-agent"
