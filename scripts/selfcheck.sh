#!/usr/bin/env bash
# 代码自检脚本（docs/ARCHITECTURE.md 第 13 节规范）。
# 用法：在仓库根目录执行 ./scripts/selfcheck.sh
# 退出码非 0 表示任一检查未通过。
set -uo pipefail

echo "=== 1/4 go build ./... ==="
if ! go build ./...; then
  echo "BUILD FAILED"
  exit 1
fi

echo "=== 2/4 go vet ./... ==="
if ! go vet ./...; then
  echo "VET FAILED"
  exit 1
fi

echo "=== 3/4 gofmt -l . (应无输出) ==="
fmt=$(gofmt -l .)
if [ -n "$fmt" ]; then
  echo "gofmt 发现问题文件："
  echo "$fmt"
  echo "请执行 gofmt -w 上述文件后重试。"
  exit 1
fi

echo "=== 4/4 go test -race ./... ==="
if ! go test -race ./...; then
  echo "TEST FAILED"
  exit 1
fi

echo ""
echo "✅ 自检全部通过（build / vet / gofmt / race test）"
