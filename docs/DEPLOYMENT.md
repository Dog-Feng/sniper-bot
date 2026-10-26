# Linux 部署流程（watch 监控端）

> 目标：在 Linux 上部署 `cmd/watch` 监控进程，7×24 常驻，输出 `candidates.jsonl` + 本地指标。
> 当前阶段为**只读监控**，不涉及私钥/签名/交易。

## 0. 前置条件

- Linux 发行版（Ubuntu/Debian 推荐），可访问外网（Alchemy、RHC 公开 RPC）。
- Go 1.26+（编译需要）。运行机器若与编译机不同，可在编译机交叉编译后拷贝二进制。

## 1. 安装 Go（若未安装）

```bash
# 以 Go 1.26 为例（版本以 go.mod 为准）
wget https://go.dev/dl/go1.26.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

## 2. 获取代码

```bash
git clone https://github.com/Dog-Feng/sniper-bot.git
cd sniper-bot
```

## 3. 准备配置（密钥不进库）

```bash
# 复制示例配置
cp configs/watch.example.json configs/watch.local.json

# 填入真实 Alchemy key（不要提交到 git）
# 编辑 configs/watch.local.json，把 <ALCHEMY_KEY> 替换为真实 key
```

Alchemy key 建议通过环境变量或本地文件注入，避免明文写死在配置里。当前实现从配置的 `chains.*.http/ws` 读取，key 直接拼在 URL 中。**务必确认 `configs/watch.local.json` 在 `.gitignore` 内**（已默认忽略）。

## 4. 编译

```bash
# 先自检（可选但推荐）
./scripts/selfcheck.sh

# 编译二进制
go build -o watch ./cmd/watch
```

### 4.1 交叉编译（在编译机产出 Linux 二进制）

若你在 Windows 编译，需在 Linux 运行，可用交叉编译：

```bash
# 在 Linux 编译机
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o watch-linux ./cmd/watch
```

本项目依赖 `golang.org/x/crypto` 的 legacy Keccak（纯 Go，无 CGO），`coder/websocket` 也是纯 Go，因此 `CGO_ENABLED=0` 可产出静态二进制，无需目标机装 Go。

## 5. 运行

```bash
# 前台运行（调试）
./watch -config configs/watch.local.json -metrics 127.0.0.1:18080

# 后台运行
nohup ./watch -config configs/watch.local.json -metrics 127.0.0.1:18080 \
  > watch.log 2>&1 &
echo $! > watch.pid
```

## 6. 验证

```bash
# 1. 进程存活
ps -p $(cat watch.pid)

# 2. 指标
curl -s http://127.0.0.1:18080/debug/vars | jq '.launch_chain_detected_total'

# 3. 落盘
tail -n 5 candidates.jsonl
```

期望看到：`launch_chain_detected_total` 随时间增长（低峰期约 1 次/78 秒，高峰期约 1 次/19 秒），`candidates.jsonl` 持续追加。

## 7. systemd 常驻（推荐）

创建 `/etc/systemd/system/sniper-watch.service`：

```ini
[Unit]
Description=sniper-bot watch monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=YOUR_USER
WorkingDirectory=/opt/sniper-bot
ExecStart=/opt/sniper-bot/watch -config /opt/sniper-bot/configs/watch.local.json -metrics 127.0.0.1:18080
Restart=on-failure
RestartSec=5
# 限制资源与权限
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/opt/sniper-bot

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo cp sniper-watch.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sniper-watch
sudo systemctl status sniper-watch
journalctl -u sniper-watch -f   # 实时日志
```

## 8. 目录布局（生产建议）

```
/opt/sniper-bot/
├── watch                       # 二进制
├── configs/
│   └── watch.local.json        # 真实配置（含 key，权限 600）
└── (运行产物)
    ├── candidates.jsonl
    └── watch.log
```

## 9. 运维要点

- **监控失明检测**：当前 1A 未内置"长时间零发射告警"（属 M3），建议外部加一个 watchdog：定期检查 `launch_chain_detected_total` 是否有增长，长时间停滞则告警。
- **磁盘**：`candidates.jsonl` 会持续增长（低峰约 1 条/78s ≈ 每天 1100 条，每条 ~300B ≈ 每天 ~330KB，可忽略）。如需清理，先停进程再截断/轮转。
- **日志**：`watch.log` 用 logrotate 轮转。
- **时钟**：当前阶段仅观测，不涉及秒边界调度，暂无需 chrony 精确对时（交易阶段必须）。

## 10. 故障排查

| 现象 | 排查 |
| --- | --- |
| 启动即退 | 看 `watch.log`：配置路径错、Alchemy key 错、网络不通 |
| 无数据 | 确认 chainId 4663、工厂地址、topic0 正确；确认 Alchemy key 有效（Free 层够用） |
| `launch_ws_reconnects_total` 持续增长 | 网络不稳定或 Alchemy 限流，检查网络与 key |
| `launch_dropped_total` 增长 | 落盘跟不上，检查磁盘 IO |
| 内存持续增长 | 升级到含"seen 滑动窗口"修复的版本（二轮自检已修复） |
