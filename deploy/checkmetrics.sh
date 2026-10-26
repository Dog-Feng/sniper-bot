#!/bin/bash
# 检查 metrics 端口监听与内容
set -e
cd /root/sniper-bot

echo "=== port 18080 listening? ==="
ss -tlnp 2>/dev/null | grep 18080 || netstat -tlnp 2>/dev/null | grep 18080 || echo "not listening"

echo "=== curl raw (first 500 chars) ==="
curl -s --max-time 5 http://127.0.0.1:18080/debug/vars | head -c 500 || echo "curl failed"
echo ""
echo "=== curl with explicit exit code ==="
curl -s -o /tmp/vars.json -w "http_code=%{http_code}\n" --max-time 5 http://127.0.0.1:18080/debug/vars
if [ -s /tmp/vars.json ]; then
  echo "=== launch_chain_detected_total ==="
  grep -o '"launch_chain_detected_total": [0-9]*' /tmp/vars.json
fi
