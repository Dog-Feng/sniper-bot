#!/bin/bash
# 精确清理所有 watch 进程并重启（用 PID 精确 kill）
set -e
cd /root/sniper-bot

echo "=== before ==="
ps -eo pid,cmd | grep '[w]atch' || echo none

# 收集所有 watch 二进制进程 PID（排除 kernel watchdogd 和 grep）
PIDS=$(ps -eo pid,cmd | awk '/\.\/watch/ && !/awk/ {print $1}')
echo "PIDS to kill: $PIDS"
for p in $PIDS; do
  echo "killing $p"
  kill -9 "$p" 2>/dev/null || true
done

# 也清理残留的 bash -c 包装进程
BASH_PIDS=$(ps -eo pid,cmd | awk '/bash -c cd \/root\/sniper-bot/ && !/awk/ {print $1}')
for p in $BASH_PIDS; do
  echo "killing wrapper $p"
  kill -9 "$p" 2>/dev/null || true
done

sleep 2

echo "=== after kill ==="
ps -eo pid,cmd | grep '[w]atch' || echo "none (all killed)"

# 干净启动
nohup ./watch -config configs/watch.local.json -metrics 127.0.0.1:18080 > watch.log 2>&1 &
echo $! > watch.pid
sleep 3

PID="$(cat watch.pid)"
echo "=== restarted pid=$PID ==="
ps -p "$PID" -o pid,stat,etime,cmd
echo "=== log ==="
cat watch.log
