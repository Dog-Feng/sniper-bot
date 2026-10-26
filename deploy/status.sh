#!/bin/bash
# 查看 watch 运行状态与数据
set -e
cd /root/sniper-bot

echo "=== all watch processes ==="
ps aux | grep '[w]atch' || echo "none"

echo "=== latest pid ==="
if [ -f watch.pid ]; then
  PID="$(cat watch.pid)"
  echo "watch.pid = $PID"
  ps -p "$PID" -o pid,stat,etime,cmd 2>/dev/null || echo "pid $PID not running"
fi

echo "=== candidates.jsonl (tail) ==="
if [ -f candidates.jsonl ]; then
  echo "lines: $(wc -l < candidates.jsonl)"
  tail -n 8 candidates.jsonl
else
  echo "no candidates.jsonl yet"
fi

echo "=== metrics (try 18080) ==="
curl -s --max-time 5 http://127.0.0.1:18080/debug/vars 2>/dev/null | grep -o 'launch_[a-z_]*":[0-9]*' || echo "metrics unreachable on 18080"
