#!/bin/bash
# 部署启动脚本：在 VPS 上运行
set -e
cd /root/sniper-bot

# 若已运行则先停
if [ -f watch.pid ] && kill -0 "$(cat watch.pid)" 2>/dev/null; then
  echo "stopping existing pid $(cat watch.pid)"
  kill "$(cat watch.pid)" 2>/dev/null || true
  sleep 1
fi

nohup ./watch -config configs/watch.local.json -metrics 127.0.0.1:18080 > watch.log 2>&1 &
echo $! > watch.pid
sleep 3

PID="$(cat watch.pid)"
echo "started pid=$PID"
echo "=== watch.log ==="
cat watch.log
echo "=== process check ==="
ps -p "$PID" -o pid,stat,etime,cmd || echo "process not running"
