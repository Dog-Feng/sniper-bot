# 狙击机器人：设计、流程与实施文档

> 状态：草案 v1 · 更新：2026-10-20 · 编码规范依据：仓库根目录 `AGENTS.md`

## 1. 项目目标

在 Robinhood Chain（RHC，chain id 4663）上狙击新代币发射：在 launchpad
（Pons / Pools.trade / LONG 等）开交易的最初几个区块内买入，并自动卖出。
架构为多链扩展预留：后续接入其他 EVM 链，未来可能支持 Solana。

核心原则：

- 买入触发只来自链上信号（发射本身），不来自社媒公告。
- 社媒（推文/KOL）只作为离场信号与钱包发现辅助，不作为买点。
- 不需要预先知道项目方钱包或代币地址；代币地址从工厂事件中被动解出。

## 2. 目标链关键事实（调研结论，2026-10）

### 2.1 链特性

- Arbitrum Orbit L2，chain id 4663（测试网 46630），gas 用 ETH，数据可用性走以太坊 blob。
- 区块约 100ms 一个；日交易量约 760 万笔；TVL 约 10 亿美元（2026-10 中旬）。
- **无公开 mempool**：交易直入私有排序器，顺序严格按到达时间（FCFS）决定，
  **提高 gas 无法插队**。
- 最早的公开可见点：交易被排序之后（feed RECEIVED 阶段，区块封存之前）。

### 2.2 官方端点

| 用途 | 地址 |
| --- | --- |
| 公开 RPC | `https://rpc.mainnet.chain.robinhood.com` |
| 排序器 feed（已排序交易流） | `wss://feed.mainnet.chain.robinhood.com` |
| 排序器（直接提交） | `https://sequencer.mainnet.chain.robinhood.com` |
| 延迟备份 feed（慢约 500ms） | `wss://delayed-feed.mainnet.chain.robinhood.com` |
| 推荐商用 RPC | Alchemy（官方推荐；本项目使用用户提供的 key，见 `alchemy_rpc.txt`） |

### 2.3 数据源清单

| 源 | 用途 | 已知限制 |
| --- | --- | --- |
| 官方 sequencer feed | 实时检测（v2 接入） | 消息压缩，需按协议解压 |
| 公共 JSON-RPC | 轮询/回填/兜底 | 限流严重；宽区间 getLogs 频繁 429 |
| Alchemy（用户提供） | 读取/模拟/兜底提交 | Free 层 getLogs 单次限 10 块；升级 PAYG 可解 |
| HoodScan MCP | 发射清单、协议锚点、市场数据 | 快照级新鲜度（几分钟） |
| GeckoTerminal API | 池子/代币行情、发射时间 | 无部署者字段 |
| Eira 等第三方 feed 中继 | 低延迟交叉验证 | 按区计费 |

### 2.4 Launchpad 锚点（固定常量，一次配置）

| 协议 | 合约 | 说明 |
| --- | --- | --- |
| Pons V2 launch factory | `0x7ed598bcef8bd9edd8c97a195c6d13f40801ec7e` | 发射事件 `0x8d4aad49…`：topics[1]=token，topics[2]=curve，topics[3]=deployer |
| Pons V2 launch-and-buy router | `0xe33e9e479df8802cb0866d5d05258bec4cf62948` | 买入入口 |
| Uniswap v4 PoolManager | `0x8366a39cc670b4001a1121b8f6a443a643e40951` | 开池/毕业信号 |
| Uniswap UniversalRouter | `0x8876789976decbfcbbbe364623c63652db8c0904` | 通用 swap |
| pools.trade router | `0xfccb3d4353f667ff54c638736812594166361210` | Uniswap Labs launchpad |
| LONG Doppler initializer | `0x4e3468951d49f2eea976ed0d6e75ffcb44a9a544` | LONG 系开池 |

### 2.5 生态实据

- 仅 Pons V2 一个工厂，高峰期 30 分钟 94 次发射（约每 19 秒一个，实测）。
- 部署者画像：样本中 nonce 普遍 100~数万，工业化发射钱包占绝对多数；
  存在复用钱包（单窗口内 5 个钱包重复发射，最多 x3）。
- 风险背景：公开报道 53 次发行、约 1800 万美元的规模化 rug 操作。

## 3. 关键设计决策

1. 六边形架构（Ports & Adapters）：内核只含业务规则与端口，所有外部协议在适配器层。
2. 语言 Go，标准库优先；第三方依赖仅限签名密码学，已于 2026-10-20 获用户批准：
   `github.com/decred/dcrd/dcrec/secp256k1/v4`（secp256k1 签名）与
   `golang.org/x/crypto`（legacy Keccak-256）。其余保持标准库，新增依赖仍需逐项批准。
3. 事件源 v1 用块轮询 + 日志（正确性优先），v2 换官方 feed（延迟优先）；端口不变。
4. 存储 v1 用 SQLite，经 Repository 端口隔离，可平滑换 Postgres。
5. 部署位置：us-east-2（俄亥俄）匹配可用区 ID；详见第 7 节。

## 4. 架构（六边形）

```
                    ┌────────────────────────────────────┐
  驱动适配器          │         DOMAIN（六边形内核）           │
  CLI / 管理API ────▶│                                     │◀──── RPC 网关 / 数据库
  链上事件源(feed) ──▶│  应用服务：engine、策略、风控、        │◀──── 签名器 / 密钥库
  定时器(止损巡检) ──▶│  protector、持仓管理                  │◀──── 推特(未来) / 时钟
                    │  端口：inbound(被外部调用)             │
                    │        outbound(由内核调用)           │
                    └────────────────────────────────────┘
```

依赖只向内：adapter import domain 端口；domain 不 import 任何 adapter 或协议库；
cmd 是唯一知道具体实现存在的地方（组合根）。

### 4.1 包结构

```
cmd/sniper/                          组合根：配置、装配、启动
internal/domain/                     内核，零第三方依赖
  model/                             实体与值对象
  service/                           应用服务
    engine/                          薄编排：路由事件、生命周期
    strategy/                        策略端口 + 注册表（ponslaunch、v4pool、kol）
    risk/                            账户级风控聚合器（唯一共享状态所有者）
    protectors/                      检查责任链：静态/模拟/画像/黑名单
    positions/                       持仓与卖出规则
    monitor/                         只读观测（只写日志/指标，永不发交易指令）
  port/
    inbound/                         LaunchListener、AdminConsole、Ticker
    outbound/                        ChainGateway、KeySigner、PositionStore、
                                     DeployerStore、EventJournal、Clock
internal/adapter/
  in/
    cli/                             管理命令适配器
    chainfeed/rhc/                   RHC 事件源（v1 轮询，v2 feed）
    scheduler/                       止损巡检、日损重置
  out/
    rpc/                             多端点 JSON-RPC 网关
    chain/evm/                       共享 EVM 原语（RLP/Keccak/交易结构）
    chain/rhc/ eth/ solana/         链协议实现（签名/解码/venue）
    store/sqlite/                    仓库实现
    keys/file/                       密钥加密存储与签名
    twitter/                         未来
```

### 4.2 端口清单

inbound（外部驱动内核）：

| 端口 | 方法 | 实现适配器 |
| --- | --- | --- |
| LaunchListener | `OnLaunch(ctx, LaunchEvent)` | chainfeed/rhc 等 |
| AdminConsole | `Pause / Resume / ClosePosition / SetMode` | cli（未来 HTTP） |
| Ticker | `OnTick(ctx)` | scheduler |

outbound（内核调用外部）：

| 端口 | 方法 | 实现适配器 |
| --- | --- | --- |
| ChainGateway | `Code / Balance / Simulate / Submit / Nonce / GasCost` | rpc + chain/* |
| KeySigner | `Sign(tx)` | keys/file + chain/* |
| PositionStore / DeployerStore | `Save / Get / List` | store/sqlite |
| EventJournal | `Append(event)` | store + 指标 |
| Clock | `Now` | 系统时钟（测试用假钟） |

### 4.3 模块独立性规则

1. adapter 之间禁止互相 import；共享原语放入库性质的 `chain/evm`。
2. 模块间通信只有两种：端口调用（同步）与内核事件（异步单向）。
3. monitor 只订阅内核事件，无任何路径可令其发出交易指令。
4. risk 是唯一持有账户级共享状态的服务；mutex 管共享状态，channel 管跨 goroutine
   信号，所有 goroutine 以 context 取消为退出条件。
5. 三类注册表支持扩展：链（chainID）、venue（chainID:venueName）、策略（名称）。
   新增能力 = 新包 + 注册，永不修改 engine 核心。

### 4.4 多链接入

一条新链 = 三个适配器 + 一次注册：

- `chainfeed/X`（in）：链事件 → `domain.Event`；
- `chain/X`（out）：签名、解码、venue calldata 构造；
- venue 注册表条目：`chainID:venueName → 构造器`。

domain 只见 chainID 字符串与 venue 名称，不感知 EVM/SVM 差异。

## 5. 狙击流程设计

### 5.1 检测：未知地址发现流程（已验证）

输入只是固定锚点（工厂合约）与固定常量（事件签名）：

1. 订阅/查询 launchpad 工厂的事件日志；
2. 命中发射事件后从 topics 解出：token、curve、deployer；
3. 随后走 protector 校验 → 风控额度 → 签名 → 提交。

2026-10-20 实测验证：扫 Pons 工厂 30 分钟日志得到 94 次发射、94 个代币、
88 个部署者，与独立回溯法（日志二分定位创建区块 → 回执回溯部署交易）对
SEASON（`0xea0071…2be6`）的部署者结论完全一致。

### 5.2 热点路径时间线（以项目方调工厂为 T0）

| 时刻 | 动作 | 说明 |
| --- | --- | --- |
| T0 | 项目方创建交易被排序器排序 | 事件出现在链上 |
| T0 + 0~20ms | feed 送达 VPS | us-east-2 实测 p99 ~20ms（官方 feed，含中继处理） |
| T0 + 20ms | 解码 calldata/日志，解出 token 地址 | 预编译签名，微秒级 |
| T0 + 20~80ms | 决策：模板指纹、画像、白名单、额度、滑点 | 高价值目标才做完整模拟 |
| T0 + 80ms | 本地签名 | <1ms |
| T0 + 80~100ms | 多路提交（排序器 + Alchemy + 就近入口） | 同签名交易多路发送，排序器去重 |
| T0 + 100~200ms | 落入同块或下一块成交 | 区块 100ms |

位置对链路的影响（RTT 估计）：

| 位置 | RPC RTT | feed 到达 | 提交到达 | 落区块 |
| --- | --- | --- | --- | --- |
| 国内（现状实测） | 450~650ms | ~300ms | ~300ms | 落后 5~8 块 |
| 德国 | ~100ms | ~50ms | ~50ms | 落后 1~2 块，无法竞争 |
| us-east-1 | 10~15ms | 5~8ms | 5~8ms | 可用 |
| us-east-2 同区 | 1~5ms | 1~3ms | 1~3ms | 同块/下一块，推荐 |

结论：RHC 上延迟即排名，无 gas 插队机制；狙击热路径必须部署 us-east-2。

### 5.3 决策与风控流程

买入前按顺序过 protector 链，任一否决即放弃；单目标决策超时（默认 80ms）后
按 fail-closed 丢弃，宁可错过不可堵队列：

1. 静态检查：字节码模板指纹、黑名单、可疑 opcode；
2. 模拟：买入+卖出，查税与蜜罐（高价值目标）；
3. 画像：deployer 历史 rug 率、nonce 特征、供资方聚类；
4. risk：单笔上限、并发持仓、链级日损、同代币冷却。

### 5.4 卖出流程

买入确认后 positions 服务接管：止盈阶梯、尾随止损、时间止损（默认 5 分钟）、
rug 信号离场（feed 上撤流动性交易 → 同块/下一块抢卖）。卖出自动化与买入同等权重。

### 5.5 程序启动时序

1. 冷启动（0~5s）：加载配置与密钥 → 建 feed/RPC 连接 → 预取 nonce →
   预编译签名与模板 → 路径自检。
2. 稳态监听：feed 常连、心跳重连、第三方流交叉验证、画像库自更新。
3. 热点路径：见 5.2。

注意：feed 无历史回放，晚启动即错过窗口内全部发射；服务必须 7×24 常驻。

## 6. 容量评估（RPC 承载）

发现环节走官方 feed，不占 Alchemy 配额。Alchemy 承担：模拟（eth_call）、
读取（eth_getCode/nonce）、兜底提交。按峰值 5 发射/分钟 × 每发射 5~7 次调用
估算约 0.4~0.6 req/s，Free 层可承受；风险在突发并发与回溯工具共用 key。

规则：回溯发现器独立 key 或低峰运行；提交主路径直连官方排序器端点；
决策层加队列与丢弃策略；VPS 就绪后用混合压测脚本实测各路径配额。

### 6.1 Alchemy 是否升级 PAYG

PAYG（Pay-As-You-Go，按量付费）是 Alchemy 的计费档位：不付固定月费，按消耗的
compute units（CU）计费。相对 Free 层它解锁：宽块区间的 `eth_getLogs`
（Free 限 10 块，实测已触发）、更高的每秒吞吐与突发上限、更多 WebSocket
连接、可选档案数据。

决策：**暂不升级**。理由：狙击热路径的发现走官方 feed，Alchemy 只承担
小范围读取（单块 getLogs、eth_call 模拟）与兜底提交，Free 层配额足够；
真正需要宽区间 getLogs 的只有回溯发现器，它独立 key、低峰运行即可。
升级触发条件（满足任一）：VPS 压测显示 Free 每秒配额触顶；或热路径加入
宽区间日志查询。届时按实测 CU 消耗估算月费再决定。

## 7. 部署方案

- 主战场：AWS us-east-2（匹配可用区 ID use2-az1/2/3），固定性能实例，Linux。
- 同机部署监听 + 决策 + 提交；德国/国内机房仅跑离线分析。
- 主提交路径：官方排序器端点；冗余：Alchemy + Eira per-AZ 入口。
- 钱包：多账户轮换，密钥加密落盘，签名进程内完成。

## 8. 已完成的准备（资产清单）

- `discover_deployers.py`：部署者发现器（时间戳→块号映射、日志二分、回执回溯、
  断点续跑），已验证 SEASON 等样本。
- `deployers.csv`：100 个发射的回溯结果（27 ok / 20 no_logs / 53 限流失败）。
- `pons_launches_evidence.json`：Pons 工厂 30 分钟 94 次发射的（token, curve,
  deployer）三元组，验证未知地址发现流程。
- `alchemy_rpc.txt`：用户提供的 Alchemy 端点（本地配置，不进入代码主体）。

## 9. 实施路线图

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| 1 | domain 骨架 + 端口 + engine + monitor + 事件日志 | 空跑通过，gofmt/vet/test 全绿 |
| 2 | chainfeed/rhc 轮询 + Pons 解码 | 干跑检测出真实发射 |
| 3 | protectors + risk + ponslaunch 策略 | 纸面执行有决策输出 |
| 4 | keys + rpc + chain/rhc 签名与 venue | 测试网真盘闭环 |
| 5 | feed adapter 换入 + positions 卖出 | 主网小资金试跑 |
| 6 | 第二条链接入 | 多链抽象验证 |

## 10. 未决事项

- [x] 2026-10-20 已批准：decred/secp256k1 + x/crypto（legacy Keccak-256）
- [ ] Alchemy 暂不升级 PAYG（见 6.1；触发条件见该节）
- [ ] us-east-2 VPS 就绪 + 压测脚本执行
- [ ] 测试网（46630）钱包与测试资金

## 11. 风险与合规

- 只狙击 memecoin 类代币；不触碰链上的代币化股票（受监管、带交易限制）。
- rug 风险高：protector 链与卖出纪律是生存线，不是可选项。
- 预算纪律：单笔小额起步，先跑 1~2 周统计胜率再放大。
