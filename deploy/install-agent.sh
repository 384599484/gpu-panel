#!/usr/bin/env bash
# 在矿机上一键安装 agent（需要 root）
# 用法: ./install-agent.sh --server ws://1.2.3.4:8080/ws/agent --token <token> [--id <机器名>]
# 也支持环境变量: SERVER / TOKEN / AGENT_ID
set -euo pipefail

SERVER_ADDR=${SERVER:-}
TOKEN=${TOKEN:-}
AGENT_ID=${AGENT_ID:-}
BIN=${BIN:-./dist/linux-amd64/gpu-panel-agent}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server) SERVER_ADDR="$2"; shift 2;;
    --token)  TOKEN="$2"; shift 2;;
    --id)     AGENT_ID="$2"; shift 2;;
    --bin)    BIN="$2"; shift 2;;
    *) echo "未知参数: $1"; exit 1;;
  esac
done

if [[ -z "$SERVER_ADDR" || -z "$TOKEN" ]]; then
  echo "用法: $0 --server ws://<服务端IP>:8080/ws/agent --token <token> [--id <机器名>]"
  exit 1
fi
if [[ ! -f "$BIN" ]]; then
  echo "找不到 agent 二进制: $BIN（先执行 ./build.sh）"
  exit 1
fi
if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 运行（sudo $0 ...）"
  exit 1
fi

install -d /etc/gpu-panel
install -m 0755 "$BIN" /usr/local/bin/gpu-panel-agent

cat > /etc/gpu-panel/agent.json <<EOF
{
  "server": "$SERVER_ADDR",
  "token": "$TOKEN",
  "agent_id": "$AGENT_ID",
  "interval_ms": 2000,
  "mock": false
}
EOF
chmod 0600 /etc/gpu-panel/agent.json

cp "$(dirname "$0")/gpu-panel-agent.service" /etc/systemd/system/gpu-panel-agent.service
systemctl daemon-reload
systemctl enable --now gpu-panel-agent

sleep 2
systemctl --no-pager status gpu-panel-agent | head -20
echo
echo "安装完成。查看日志: journalctl -u gpu-panel-agent -f"
