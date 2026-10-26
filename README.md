# sniper-bot

Robinhood Chain（RHC，chain id 4663）上 Pons V2 新代币发射的**秒边界狙击机器人**监控端。

> 当前阶段：**1A 双源监控**（链上发射 + 项目方推文），只读、不交易。
> 设计文档见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)，部署流程见 [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)。

## 它能做什么

1. **链上源**：通过 Alchemy WebSocket 订阅 Pons V2 工厂的 `TokenLaunched` 事件，实时解出 `{token, curve, deployer, pairToken, configId, graduationThreshold}`，并记录检测延迟。
2. **推文源**：轮询指定项目方账号，规则解析出代币地址（CA）、链、预告/已上线状态，走只读校验闸门后落盘。
3. **统一落盘**：两类事实统一写入 `candidates.jsonl`，带 `source`（chain/social）与 `verified` 字段。
4. **本地指标**：`expvar` 暴露检测延迟分位数（p50/p90/p99）、各源计数、丢弃/解码错误/重连计数等。

## 目录结构

```
cmd/watch/                 组合根：双源装配 + 优雅退出 + 指标
internal/
  domain/                  六边形内核：model + 端口 + monitor 服务
  adapter/
    in/launchsource/rhc/   链上源：WS 订阅 + getLogs 兜底
    in/socialsource/x/     推文源：轮询 + 规则解析
    out/rpc/               JSON-RPC 客户端
    out/chain/evm/         EVM 原语（Keccak/ABI/地址）
    out/store/jsonl/       非阻塞 JSONL 落盘
    out/clock/             系统时钟
  config/                  配置加载
  metrics/                 进程级 expvar 指标
configs/                   示例配置（真实配置 *.local.json 不入库）
docs/                      设计 + 部署文档
scripts/selfcheck.ps1|sh  一键代码自检
```

## 快速开始

```bash
# 1. 准备本地配置
cp configs/watch.example.json configs/watch.local.json
# 编辑 watch.local.json，填入真实 Alchemy key（或从 alchemy_rpc.txt 复制）

# 2. 运行
./watch -config configs/watch.local.json -metrics 127.0.0.1:18080

# 3. 查看指标
curl http://127.0.0.1:18080/debug/vars
```

## 依赖

| 依赖 | 用途 | 状态 |
| --- | --- | --- |
| `github.com/coder/websocket` | WebSocket 客户端（Alchemy 订阅） | 已批准 |
| `golang.org/x/crypto` | legacy Keccak-256 | 已批准 |
| 其余 | 标准库 | — |

新增依赖需在 `AGENTS.md` 中说明理由并获批准。

## 代码自检（强制）

每次代码实现/更新完成后必须执行：

```bash
powershell -File scripts\selfcheck.ps1   # Windows
./scripts/selfcheck.sh                    # Linux/macOS
```

规范见 `docs/ARCHITECTURE.md` 第 13 节。
