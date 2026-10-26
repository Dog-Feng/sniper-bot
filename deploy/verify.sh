#!/bin/bash
# 最终验证：进程 + 数据 + 指标
set -e
cd /root/sniper-bot

echo "=== process ==="
ps -p "$(cat watch.pid)" -o pid,stat,etime,cmd || echo "NOT RUNNING"

echo "=== candidates count ==="
wc -l candidates.jsonl 2>/dev/null || echo "no file"

echo "=== metrics (chain detected) ==="
curl -s --max-time 5 http://127.0.0.1:18080/debug/vars | grep -oE '"launch_chain_detected_total":[0-9]+' || echo "unreachable"

echo "=== metrics (all launch) ==="
curl -s --max-time 5 http://127.0.0.1:18080/debug/vars | grep -oE '"launch_[a-z_]+_total":[0-9]+' || echo "unreachable"

echo "=== log tail ==="
tail -n 5 watch.log
