# 狙击机器人：设计、流程与实施文档

> 状态：草案 v2 · 更新：2026-10-20 · 编码规范依据：仓库根目录 `AGENTS.md`

## 0. 变更记录

| 版本 | 日期 | 摘要 |
| --- | --- | --- |
| v1 | 2026-10-20 | 初稿：抢发射后最初几个区块、us-east-2 极限延迟路线 |
| v2 | 2026-10-20 | 批准 WebSocket 依赖 `github.com/coder/websocket`；根据 Pons V2 狙击税（已在链上实测）重写买点模型：从"抢第一块"改为"抢税率归零的秒边界"；**不自建 Nitro 节点**（成本约束），检测改走 Alchemy WebSocket 日志订阅；新增排序器时钟校准、税率保护下单、内部盘画像、链健康熔断；v1 只做 Pons V2，删除多链/Solana 预设计；路线图新增阶段 0（回测先行） |

## 1. 项目目标

在 Robinhood Chain（RHC，chain id 4663）上参与 Pons V2 新代币发射：在**狙击税降到可接受水平的第一个区块**
买入，并自动卖出。v1 只支持 Pons V2；其他 launchpad 与其他链在回测证明有正期望之后再接入。

核心原则：

- 买入触发只来自链上信号（发射本身），不来自社媒公告。
- 社媒（推文/KOL）只作为离场信号与钱包发现辅助，不作为买点。
- 不需要预先知道项目方钱包或代币地址；代币地址从工厂事件中被动解出。
- **先证明期望为正，再投入低延迟基础设施**（阶段 0 回测是硬门槛）。
- **不自建链节点**：只使用官方公开端点 + Alchemy（成本约束，见 3.1）。

## 2. 目标链与 launchpad 关键事实（调研结论，2026-10）

### 2.1 链特性

- Arbitrum Orbit L2，chain id 4663（测试网 46630），gas 用 ETH，数据可用性走以太坊 blob。
- 区块约 100ms 一个；**`block.timestamp` 以秒为单位，约 10 个区块共用同一时间戳**（这是买点模型的基础，见 2.5）。
- **无公开 mempool**：交易直入私有排序器，顺序严格按到达时间（FCFS）决定，**提高 gas 无法插队**
  （[Robinhood Chain 官方文档](https://docs.robinhood.com/chain/)）。
- 最早的公开可见点：交易被排序之后（feed 推送，区块封存之前）。
- 官方未公布排序器所在区域（[连接文档](https://docs.robinhood.com/chain/connecting/)），部署位置必须实测（见第 7 节）。
- 2026-09-04 出现过 L1 数据发布中断、交易积压与 feed 连接问题，部分应用的成功交易在约 43 分钟内降到平时的两成左右；
  官方于 09-22 增加了延迟备份 feed（[TokenPost](https://www.tokenpost.com/news/technology/25888)）。熔断设计见 5.8。

### 2.2 官方端点

| 用途 | 地址 |
| --- | --- |
| 公开 RPC | `https://rpc.mainnet.chain.robinhood.com` |
| 排序器 feed（已排序交易流） | `wss://feed.mainnet.chain.robinhood.com` |
| 排序器（直接提交） | `https://sequencer.mainnet.chain.robinhood.com` |
| 延迟备份 feed（慢约 500ms） | `wss://delayed-feed.mainnet.chain.robinhood.com` |
| Alchemy RPC / WebSocket | `https://robinhood-mainnet.g.alchemy.com/v2/{KEY}` / `wss://robinhood-mainnet.g.alchemy.com/v2/{KEY}`（key 见本地 `alchemy_rpc.txt`） |

公开端点有限流，官方不推荐生产使用（[连接文档](https://docs.robinhood.com/chain/connecting/)）。

### 2.3 数据源清单（不自建节点前提下）

| 源 | v2 用途 | 已知限制 |
| --- | --- | --- |
| Alchemy WebSocket `eth_subscribe logs` | **主检测源**：订阅 Pons 工厂 `TokenLaunched` | 区块封存后推送，延迟需实测；按事件计 CU |
| Alchemy HTTP | 读取发射交易 calldata、曲线参数、nonce；兜底提交 | Free 层 getLogs 单次限 10 块 |
| 官方 sequencer feed | **时钟校准**（只读消息头里的区块时间戳，不解码交易）；后续可选的检测加速 | 无历史回放；09-04 出现过连接问题 |
| 官方排序器端点 | **主提交路径** | — |
| 公共 JSON-RPC | 回填与回测：宽区间 getLogs、历史区块 eth_call | 有限流；2026-10-20 实测单次 3000 块 getLogs、历史区块 eth_call 均成功，但不保证稳定 |
| Bitquery Pons 数据集 | 回测备选数据源（CurveBuy/CurveSell 带狙击税字段，见 [Bitquery 文档](https://docs.bitquery.io/docs/blockchain/robinhood/)） | 第三方、可能收费 |
| HoodScan MCP / GeckoTerminal | 发射清单、行情交叉验证 | 快照级新鲜度 |

### 2.4 Launchpad 锚点（固定常量，一次配置）

| 协议 | 合约 | 说明 |
| --- | --- | --- |
| Pons V2 launch factory | `0x7ed598bcef8bd9edd8c97a195c6d13f40801ec7e` | `TokenLaunched` 事件 `0x8d4aad49…`：topics[1]=token，topics[2]=curve，topics[3]=deployer；data=pairToken、launchConfigId、graduationThreshold |
| Pons V2 launch-and-buy router | `0xe33e9e479df8802cb0866d5d05258bec4cf62948` | `launchAndBuy`（实测 selector `0xf85f8e41`）：创建 + 创建者首买同一笔交易 |
| Uniswap v4 PoolManager | `0x8366a39cc670b4001a1121b8f6a443a643e40951` | 毕业后交易场所 |
| Uniswap UniversalRouter | `0x8876789976decbfcbbbe364623c63652db8c0904` | 毕业后卖出路由 |
| pools.trade router / LONG Doppler initializer | `0xfccb…1210` / `0x4e34…a544` | v1 不支持，保留调研 |

实测观察到的发射入口：约一半经 router（`0xf85f8e41`），其余直接调工厂（`0xa72101af`、`0xf35abbcf` 两种 selector，需对照已验证 ABI 确认）。

### 2.5 Pons V2 发射机制（决定买点的核心事实）

来源：[Pons V2 官方文档](https://docs.ponsfamily.com/v2) + 2026-10-20 链上实测。

**狙击税**（只对买入征收，按代币接收地址 `recipient` 计算，卖出不收）：

- 曲线合约暴露 `launchedAt()`（等于发射区块时间戳）和 `currentSnipeTaxBps(address)`。
- 设 `Δt = block.timestamp − launchedAt`（单位：秒）。对 7 个独立发射在历史区块上调用 `eth_call` 实测，税率表完全一致：

| Δt（秒） | 税率 bps | 税率 |
| --- | --- | --- |
| 0 | 9900 | 99% |
| 1 | 618 | 6.18% |
| 2 | 19 | 0.19% |
| ≥3 | 0 | 0 |

- 官方文档说"约 1 秒时接近 25%、约 2 秒时约 3%"，但实测衰减更快、按秒跳变；**以链上实测为准**，并在运行时用 `currentSnipeTaxBps` 校验。
- 推论：在 Δt=0 抢先买入等于把 99% 本金交给发射方的手续费池；**真正的竞争点是 Δt 跨到 2（或 3）的第一个区块**。

**豁免**：发射地址与 creator fee 接收地址自动免税；创建时可额外指定最多 32 个豁免地址，之后不可追加。
`launchAndBuy` 的 `recipient` 自动豁免。→ 发射方及其关联钱包可以在 Δt=0 零税买入，我方结构性地排在其后。

**费用**：每次发射固定 `feeBps`（实测均为 100，即 1%）与 `creatorTaxBps`（实测 0~300，即 0~3%），终身不变，买入从输入扣，卖出从输出扣。

**定价**：带虚拟储备的恒定乘积。`getReserves()` 返回定价储备（含 phantom quote reserve）：

```
净输入 = quoteIn × (10000 − feeBps − creatorTaxBps − snipeTaxBps) / 10000
amountOut = 净输入 × tokenReserve / (quoteReserve + 净输入)        // 整数除法
```

**毕业与流动性**：曲线卖完即毕业（原生 ETH 计价阈值 4.2 ETH），毕业后曲线以 `CurveGraduated` revert，
流动性进入 Uniswap v4 并**永久锁定**（[Bitquery Pons API 文档](https://docs.bitquery.io/docs/blockchain/robinhood/pons-api/)）。
→ 不存在"撤池" rug；风险来自内部人在曲线上集中卖出。

**地址**：token 与 curve 均为 CREATE2 部署（salt + 发起钱包 + 构造参数），官方未公开推导公式。v2 不做本地推导，直接读 `TokenLaunched`。

### 2.6 生态实据

- 仅 Pons V2 一个工厂，高峰期 30 分钟 94 次发射（约每 19 秒一个，即每分钟约 3.1 次，实测）。
- 2026-10-20 抽样 10 次发射的前 9 秒交易数：多数只有 0~4 笔；有 1 次在 Δt=0 出现 16 笔交易（疑似豁免地址捆绑买入）。
  样本太小，只作方向判断，以阶段 0 的全量统计为准。
- 部署者画像：样本中 nonce 普遍 100~数万，工业化发射钱包占绝对多数；存在复用钱包。
  注意 `deployers.csv` 有 53% 因限流失败，**样本有偏，画像结论暂不可用**。
- 风险背景：公开报道过 53 次发行、约 1800 万美元的规模化 rug 操作。

## 3. 关键设计决策

1. **不自建 Nitro 节点**（3.1）。
2. **买点 = 秒边界**：目标在 Δt = k（默认 k=2，由回测决定）的第一个区块成交；下单用 `minTokensOut` 做税率保护，早到的交易直接 revert（5.5）。
3. **检测不再是延迟瓶颈**：发射到 Δt=2 边界之间有 1~2 秒，Alchemy WebSocket 日志推送足够；排序器 feed 降级为时钟校准与可选加速。
4. **本地定价**：Pons 曲线报价是纯计算，热路径不做远程模拟。
5. 六边形架构（Ports & Adapters）：内核只含业务规则与端口，所有外部协议在适配器层。
6. 语言 Go，标准库优先；已批准依赖：`github.com/decred/dcrd/dcrec/secp256k1/v4`（secp256k1 签名）、`golang.org/x/crypto`（legacy Keccak-256）、
   `github.com/coder/websocket`（WebSocket 客户端，用于 Alchemy 订阅与 sequencer feed；标准库没有 WebSocket，2026-10-20 批准）。其余新增依赖仍需逐项批准。
7. 存储 v1 用 SQLite，经 Repository 端口隔离；**热路径不同步写盘**。
8. **v1 单链单 launchpad**：只保留 chainID/venue/strategy 注册表这一层扩展点，不预先设计 Solana 抽象（遵循 `AGENTS.md` "no speculative code"）。

### 3.1 为什么不自建节点，以及代价

自建 Nitro 全节点需要高配机器（8 核以上、64GB 内存、数 TB NVMe，见 [QuickNode 指南](https://www.quicknode.com/guides/robinhood/how-to-run-a-robinhood-chain-full-node)）和以太坊 L1 执行层 + Beacon 端点，超出预算。

代价与对策：

| 代价 | 对策 |
| --- | --- |
| feed 只给交易、不给回执，无法即时拿到 token 地址 | 用 Alchemy WS 订阅 `TokenLaunched`；秒边界模型下有 1~2 秒余量 |
| 读取与模拟受第三方限流 | 本地定价；每次发射只做少量读取（6 节）；回测走公共 RPC/Bitquery，低峰运行 |
| 无法本地回放历史状态 | 公共 RPC 历史区块 eth_call（实测可用）+ 本地缓存 |
| 检测延迟比自建节点高几百毫秒 | 若实测检测延迟让 Δt=2 来不及，自动降级到 Δt=3 |

## 4. 架构（六边形）

```
                    ┌────────────────────────────────────┐
  驱动适配器          │         DOMAIN（六边形内核）           │
  CLI / 管理API ────▶│                                     │◀──── RPC 网关（Alchemy/公共/排序器）
  发射事件源 ────────▶│  engine、ponsv2 策略、protectors、     │◀──── 签名器 / 密钥库
  区块时钟源(feed) ──▶│  risk、timing、positions、monitor     │◀──── 存储 / 时钟
  定时器 ───────────▶│                                     │
                    └────────────────────────────────────┘
```

依赖只向内：adapter import domain 端口；domain 不 import 任何 adapter 或协议库；cmd 是唯一知道具体实现的地方（组合根）。

### 4.1 包结构

```
cmd/sniper/                          组合根：配置、装配、启动
cmd/backfill/                        离线：发射与曲线事件回填（替代 discover_deployers.py）
cmd/backtest/                        离线：按 Δt=k 入场 + 离场规则回测
internal/domain/                     内核，零第三方依赖
  model/                             实体与值对象（Launch、Curve、Position、Order）
  service/
    engine/                          薄编排：路由事件、生命周期
    strategy/                        策略端口 + 注册表
      ponsv2/                        Pons V2 策略：本地定价、税率表、下单参数
    protectors/                      检查链：发射参数 / 内部盘 / 部署者画像 / 黑名单
    risk/                            账户级风控聚合器（唯一共享状态所有者）+ 链健康熔断
    timing/                          排序器时钟偏移估计、秒边界发射调度
    positions/                       持仓与卖出规则（含毕业后路由切换）
    monitor/                         只读观测（只写日志/指标，永不发交易指令）
  port/
    inbound/                         LaunchListener、BlockClockListener、AdminConsole、Ticker
    outbound/                        ChainReader、TxSubmitter、KeySigner、
                                     PositionStore、DeployerStore、EventJournal、Clock
internal/adapter/
  in/
    cli/                             管理命令
    launchsource/rhc/                Alchemy WS 日志订阅（主）+ HTTP 轮询（兜底）
    blockclock/rhc/                  sequencer feed 消息头时间戳（主）+ newHeads（兜底）
    scheduler/                       止损巡检、日损重置
  out/
    rpc/                             多端点 JSON-RPC 网关（按用途路由、限流感知）
    chain/evm/                       共享 EVM 原语（RLP/Keccak/EIP-1559 交易/签名）
    chain/rhc/                       Pons calldata 编解码、曲线与 v4 venue
    store/sqlite/                    仓库实现
    keys/file/                       密钥加密存储与签名
```

v1 删除的预设计：`chain/eth`、`chain/solana`、`twitter`、`v4pool`、`kol` 策略。需要时作为新包加入，不改 engine。

### 4.2 端口清单

inbound（外部驱动内核）：

| 端口 | 方法 | 实现适配器 |
| --- | --- | --- |
| LaunchListener | `OnLaunch(ctx, LaunchEvent)` | launchsource/rhc |
| BlockClockListener | `OnBlockHeader(ctx, BlockHeader)`（块号、时间戳、本地接收时刻） | blockclock/rhc |
| AdminConsole | `Pause / Resume / ClosePosition / SetMode` | cli |
| Ticker | `OnTick(ctx)` | scheduler |

outbound（内核调用外部；按 `AGENTS.md` 拆成小接口）：

| 端口 | 方法 | 实现适配器 |
| --- | --- | --- |
| ChainReader | `Call / TxByHash / Nonce / BaseFee` | rpc + chain/rhc |
| TxSubmitter | `Submit(ctx, rawTx)`（多路发送） | rpc |
| KeySigner | `Sign(ctx, tx)` | keys/file + chain/evm |
| PositionStore / DeployerStore | `Save / Get / List` | store/sqlite |
| EventJournal | `Append(event)`（非阻塞，满则丢弃并计数） | store + 指标 |
| Clock | `Now`（单调时钟 + 墙钟） | 系统时钟（测试用假钟） |

### 4.3 模块独立性规则

1. adapter 之间禁止互相 import；共享原语放入库性质的 `chain/evm`。
2. 模块间通信只有两种：端口调用（同步）与内核事件（异步单向）。
3. monitor 只订阅内核事件，无任何路径可令其发出交易指令。
4. risk 是唯一持有账户级共享状态的服务；mutex 管共享状态，channel 管跨 goroutine 信号，所有 goroutine 以 context 取消为退出条件。
5. 注册表：链（chainID）、venue（chainID:venueName）、策略（名称）、protector（名称）。新增能力 = 新包 + 注册，不改 engine。
6. 热路径（发射 → 决策 → 签名 → 提交）不做磁盘 I/O；画像库启动时全量载入内存，后台增量更新。

## 5. 狙击流程设计

### 5.1 检测

1. launchsource 通过 Alchemy WS `eth_subscribe("logs", {address: 工厂})` 接收 `TokenLaunched`，
   解出 token、curve、deployer、pairToken、launchConfigId、graduationThreshold，以及块号与交易哈希。
2. 并行读取：发射交易 calldata（`TxByHash`，解出创建者首买金额与 `snipeTaxExemptions` 列表）、
   曲线的 `launchedAt / feeBps / creatorTaxBps / getReserves`。
3. 兜底：WS 断线时切到 HTTP 轮询（每次 ≤10 块，Alchemy Free 限制），重连后按块号去重补齐。
4. 可选加速（阶段 6）：从 feed 解码发射交易，提前约一个区块得知发射，并在拿到日志前预热读取。

### 5.2 热路径时间线（以发射区块时间戳 `S = launchedAt` 为基准）

| 时刻（排序器时钟） | 动作 |
| --- | --- |
| S + 0 ~ 1s 内 | 发射交易被排序；创建者与豁免地址零税买入 |
| 发射后 +100ms ~ +500ms（待实测） | 收到 `TokenLaunched`，并行读取 calldata 与曲线参数 |
| 收到后 ≤300ms | protector 链 + risk 决策；计算下单量与 `minTokensOut`；预先签名 |
| 边界 `S + k` 前 | 继续观察 Δt<k 期间的买单（内部盘规模），必要时取消 |
| 本地 `T_fire = 边界本地时刻 + ε − 单程延迟` | 多路提交（排序器为主） |
| 边界后第一个区块 | 成交；早到则因 `minTokensOut` revert，只损失少量 gas |

发射可能发生在秒 S 内的任意时刻，所以边界 S+2 距离发射有 1~2 秒。若检测加决策来不及在 S+2 前完成，自动改投 S+3（税率为 0，但竞争更多）。

### 5.3 排序器时钟校准（timing）

FCFS 下决定名次的是"边界后多快到达排序器"，所以需要估计排序器时钟的秒翻转时刻：

1. blockclock 从 feed 消息头读取每个区块的 L2 时间戳，并记录本地单调时钟的接收时刻（不需要解码交易）。
2. 每当相邻两块时间戳从 s 变为 s+1，记录"翻转观测时刻"；减去估计的单程延迟，得到排序器秒相位的一个样本。
3. 用滑动窗口（如最近 600 次翻转）取中位数，得到相位估计和误差分布；VPS 跑 chrony 保证本地墙钟稳定。
4. 输出：`BoundaryLocal(s)`（秒 s 在本地时钟上的预计开始时刻）与置信区间，供发射调度使用。
5. 区块约 100ms 一个，单次观测分辨率约 ±100ms；ε 用误差分布的高分位，宁可略晚，不可大幅提前（提前会 revert）。

### 5.4 决策与风控（protector 链）

任一否决即放弃；决策截止时间 = `T_fire − 签名与提交预留`，超时按 fail-closed 放弃（或降级到 k+1）：

1. **发射参数**：`pairToken` 白名单（v1 只做原生 ETH）、`creatorTaxBps` 上限、`feeBps` 校验、配置 ID 校验。
2. **内部盘**：`snipeTaxExemptions` 长度、创建者首买占毕业阈值的比例、Δt<k 期间的零税/豁免买入总量；超过阈值即放弃。
3. **部署者画像**：历史发射的毕业率与砸盘率、nonce 特征、资金来源聚类、黑名单。
4. **risk**：单笔上限、并发持仓、链级日损、同部署者冷却、钱包余额上限、链健康状态（5.8）。

v1 删除的检查：字节码模板指纹、可疑 opcode、买卖模拟查蜜罐。Pons V2 代币由工厂固定模板部署，这些检查没有意义；如果以后接入任意代币的 venue，再作为该 venue 的 protector 加入。

### 5.5 下单与税率保护

1. 目标税率 `taxTarget = 税率表[k]`（k=2 时为 19 bps）。
2. 按 2.5 节公式和当前储备本地计算 `expectedOut(taxTarget)`，`minTokensOut = expectedOut × (1 − slippage)`。
   `slippage` 必须小于"税率表[k−1] − 税率表[k]"（k=2 时约 6%），确保交易若落在 Δt=k−1 一定 revert。
3. 发射前可选 `eth_call currentSnipeTaxBps(我方地址)` 复核，但不作为热路径必需步骤。
4. 交易参数：EIP-1559，`maxFeePerGas` 覆盖 basefee 突增；FCFS 下多给小费没有意义；预留 L1 数据费。
5. 进阶（需评估）：部署一个条件买入合约，在链上原子检查 `currentSnipeTaxBps` 与储备，不满足就 revert。
   只有在 `minTokensOut` 方案实测不够用时才做。

### 5.6 nonce 与钱包

- 钱包池 + 租约：每个钱包同时最多一笔在途交易；本地维护 nonce，失败或超时后从链上重新同步。
- 无公开 mempool，不做加 gas 替换。
- 热钱包只放当日预算；利润定时归集到冷钱包。

### 5.7 卖出流程

买入确认后 positions 接管，卖出自动化与买入同等权重：

- 规则：止盈阶梯、尾随止损、时间止损（默认 5 分钟，由回测调优）。
- **内部人卖出监控**：订阅持仓曲线上 deployer、豁免地址、关联地址的卖出；触发即市价离场。
  FCFS 下看到对方卖单时它已经排在前面，这只能减少损失，不能避免损失；真正的防线是 5.4 的入场筛选和仓位大小。
- **毕业路由切换**：曲线 revert `CurveGraduated` 后，改走 Uniswap v4（UniversalRouter）卖出。
- 卖出没有狙击税，但要扣 `feeBps + creatorTaxBps`，计入收益计算。

### 5.8 故障与熔断

- 链健康指标：feed 落后、区块间隔异常（>1s）、提交失败率、Alchemy 推送延迟。
- 任一超阈值：暂停开新仓，只允许平仓；恢复需连续 N 分钟正常。
- 时钟源切换顺序：主 feed → Alchemy newHeads → 延迟 feed（慢约 500ms，只用于监控和平仓，不用于开仓定时）。

### 5.9 程序启动时序

1. 冷启动（0~5s）：加载配置与密钥 → 建 RPC/WS 连接 → 同步各钱包 nonce → 载入画像库 → 路径自检。
2. 时钟预热：等待至少 60 次秒翻转观测，timing 置信区间合格后才允许开仓。
3. 稳态：WS 常连、心跳重连、断线补齐；画像库后台更新。

注意：WS 推送与 feed 都没有历史回放，进程停机期间的发射需要启动后用 HTTP 补齐（只用于画像，不追买）。服务需 7×24 常驻。

## 6. 容量评估（RPC 承载）

每次发射的调用：WS 推送 1 次 + `TxByHash` 1 次 + 曲线读取 3~4 次（可用 multicall 合并）+ 提交 1~3 路 + 确认 1 次。
按峰值每分钟 5 次发射计，约每秒 0.5~1 次请求，Alchemy Free 层足够。持仓期间的曲线事件订阅只覆盖持仓代币，量很小。

规则：

- 时钟校准走官方 feed，不占 Alchemy 配额。
- 回测与回填只用公共 RPC 或 Bitquery，低峰运行，与实盘 key 隔离。
- 提交主路径直连官方排序器端点；Alchemy 只做冗余。

### 6.1 Alchemy 是否升级 PAYG

决策：**暂不升级**。热路径只需要日志订阅、少量读取和兜底提交；宽区间 getLogs 只在离线回填时需要，可以改用公共 RPC 或 Bitquery。
升级条件（满足任一）：实测 Free 层吞吐或 CU 触顶；WS 推送延迟明显劣于付费档且影响 Δt=2 命中率。

## 7. 部署方案

- 一台小型固定性能 Linux 实例即可（无节点，资源需求低）；chrony 对时。
- 选址实测：在 us-east-1 与 us-east-2 各开一台临时实例，测量到 `sequencer.mainnet` 的 RTT、feed 到达时间差、Alchemy WS 推送延迟，选最优可用区。在此之前，任何 RTT 数字都只是估计值。
- 同机部署检测 + 决策 + 提交；国内/德国机器只跑离线回测与分析。
- 主提交路径：官方排序器端点；冗余：Alchemy。

## 8. 已完成的准备（资产清单）

- `discover_deployers.py`：部署者发现器（未入库）。**计划用 `cmd/backfill` 的 Go 实现替代并入库**，保证可复现。
- `deployers.csv`：100 个发射的回溯结果（27 ok / 20 no_logs / 53 限流失败），样本有偏，仅作参考。
- `pons_launches_evidence.json`：Pons 工厂 30 分钟 94 次发射的（token, curve, deployer）三元组。
- 2026-10-20 链上实测：狙击税表（2.5）、费用参数、前 9 秒交易分布（2.6）。实测脚本需纳入 `cmd/backtest` 的 `taxprobe` 子命令。
- `alchemy_rpc.txt`：本地配置，不入库。

## 9. 实施路线图

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| 0 | `cmd/backfill` 全量回填 Pons V2 发射与曲线事件；`cmd/backtest` 按 Δt=1/2/3/5 入场 + 离场规则回测；统计各秒边界竞争者 | 输出分组期望收益报告；**所有分组期望为负则停止项目** |
| 1 | domain 骨架 + 端口 + engine + monitor + ponsv2 本地定价 | gofmt/vet/test 全绿；本地报价与历史成交逐笔一致 |
| 2 | launchsource（Alchemy WS + 轮询兜底）+ calldata 解码 + timing 时钟校准 | 干跑：实时输出发射与特征；边界估计误差分布达标 |
| 3 | protectors + risk（含熔断）+ 边界发射调度（纸面） | 纸面成交与回测结论一致 |
| 4 | keys + 签名 + nonce 池 + 多路提交 | 主网极小额闭环（若测试网有 Pons V2 则先测试网） |
| 5 | positions 全套卖出（含内部人监控、毕业路由） | 主网小资金 1~2 周，实盘与回测偏差可解释 |
| 6 | 扩展：feed 解码加速、条件买入合约、其他 launchpad、第二条链 | 每项都需回测或实测证明收益 |

## 10. 未决事项

- [x] 2026-10-20 已批准：decred/secp256k1 + x/crypto（legacy Keccak-256）
- [x] 2026-10-20 决定：不自建 Nitro 节点
- [x] 2026-10-20 已批准：`github.com/coder/websocket`（WebSocket 客户端）
- [ ] Alchemy 暂不升级 PAYG（见 6.1）
- [ ] us-east-1 / us-east-2 选址实测
- [ ] Alchemy WS 推送延迟实测（决定 k=2 是否可行）
- [ ] 对照已验证合约 ABI 确认 CurveBuy/CurveSell 等事件签名与工厂两种 selector
- [ ] 测试网（46630）是否部署 Pons V2；钱包与测试资金
- [ ] 阶段 0 回测结论

## 11. 风险与合规

- 只参与 memecoin 类代币；不触碰链上的代币化股票（受监管、带交易限制）。
- 结构性劣势：发射方与豁免地址零税先买，我方天然是后手；protector 链与仓位纪律是生存线，不是可选项。
- 规则风险：Pons 可能调整狙击税或发射配置；运行时用 `currentSnipeTaxBps` 校验税率表，偏离即停止开仓。
- 预算纪律：单笔小额起步，实盘结果必须与回测对照，偏差无法解释时停止放大。
