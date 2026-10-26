// Package rhc 实现 RHC 链上发射源：Alchemy WS 日志订阅 + HTTP 兜底轮询。
package rhc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/adapter/out/chain/evm"
	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/inbound"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
	"github.com/Dog-Feng/sniper-bot/internal/metrics"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Config 是单个工厂的订阅配置。
type Config struct {
	ChainID        uint64
	Factory        string
	Topic0         string
	WSURL          string
	PollBlockRange uint64
}

// Source 订阅工厂事件，解码为 Candidate 后投递给 listener。
type Source struct {
	cfg      Config
	chain    outbound.ChainReader
	logs     outbound.LogReader
	listener inbound.CandidateListener
	clock    outbound.Clock
	log      *slog.Logger

	mu     sync.Mutex
	seen   map[uint64]map[string]struct{} // 按块分组的去重键（block → set of "tx:logIndex"）
	latest uint64                         // 已处理最高块
}

// seenWindow 保留最近多少块的去重键；超出窗口的块惰性清理，防止 map 无界增长。
const seenWindow = 2000

// New 构造链上源。logs 为 nil 时禁用兜底轮询。
func New(cfg Config, chain outbound.ChainReader, logs outbound.LogReader, listener inbound.CandidateListener, clock outbound.Clock, log *slog.Logger) *Source {
	return &Source{
		cfg:      cfg,
		chain:    chain,
		logs:     logs,
		listener: listener,
		clock:    clock,
		log:      log,
		seen:     map[uint64]map[string]struct{}{},
	}
}

// Run 阻塞运行：WS 订阅为主，断线切轮询，直至 ctx 取消。
func (s *Source) Run(ctx context.Context) error {
	backoff := 1 * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := s.runWS(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		metrics.WSReconnects.Add(1)
		s.log.Warn("launchsource ws ended, falling back to poll", "err", err)
		// WS 结束后轮询兜底 + 指数退避重连，直到能重连或 ctx 取消。
		if !s.pollLoop(ctx, backoff) {
			return ctx.Err()
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *Source) runWS(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, s.cfg.WSURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	sub := map[string]interface{}{
		"id":     1,
		"method": "eth_subscribe",
		"params": []interface{}{"logs", map[string]interface{}{
			"address": s.cfg.Factory,
			"topics":  []string{s.cfg.Topic0},
		}},
	}
	if err := wsjson.Write(ctx, conn, sub); err != nil {
		return fmt.Errorf("ws subscribe: %w", err)
	}

	s.log.Info("launchsource subscribed", "factory", s.cfg.Factory)
	for {
		var msg json.RawMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return fmt.Errorf("ws read: %w", err)
		}
		if err := s.handleMessage(ctx, msg); err != nil {
			s.log.Warn("launchsource handle message", "err", err)
		}
	}
}

// handleMessage 解析 eth_subscribe 的 log 消息。
func (s *Source) handleMessage(ctx context.Context, raw json.RawMessage) error {
	var msg struct {
		Params struct {
			Result struct {
				BlockNumber     string   `json:"blockNumber"`
				Topics          []string `json:"topics"`
				Data            string   `json:"data"`
				TransactionHash string   `json:"transactionHash"`
				LogIndex        string   `json:"logIndex"`
			} `json:"result"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	// 订阅确认消息没有 blockNumber，忽略。
	if msg.Params.Result.BlockNumber == "" {
		return nil
	}
	block, err := decodeQuantity(msg.Params.Result.BlockNumber)
	if err != nil {
		return err
	}
	return s.emit(ctx, block, msg.Params.Result.TransactionHash, msg.Params.Result.LogIndex, msg.Params.Result.Topics, msg.Params.Result.Data)
}

func (s *Source) emit(ctx context.Context, block uint64, txHash, logIndex string, topics []string, data string) error {
	if !s.markSeen(block, txHash, logIndex) {
		return nil // 已处理过
	}

	c, err := decodeLaunch(s.cfg.ChainID, block, txHash, topics, data)
	if err != nil {
		metrics.DecodeErrors.Add(1)
		return err
	}
	c.ReceivedAt = s.clock.Now()

	// 拉取区块时间戳用于延迟观测（不阻塞投递路径；失败仅记录）。
	if b, err := s.chain.BlockByNumber(ctx, block); err == nil {
		c.BlockTime = time.Unix(b.Timestamp, 0)
	}

	return s.listener.OnCandidate(ctx, c)
}

// markSeen 记录并去重；返回 false 表示已存在（重复）。惰性清理超窗口的旧块。
func (s *Source) markSeen(block uint64, txHash, logIndex string) bool {
	key := txHash + ":" + logIndex
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[block] == nil {
		s.seen[block] = map[string]struct{}{}
	}
	if _, dup := s.seen[block][key]; dup {
		return false
	}
	s.seen[block][key] = struct{}{}
	if block > s.latest {
		s.latest = block
	}
	// 惰性清理：删除 latest - window 之前的块分组。
	if s.latest > seenWindow {
		cutoff := s.latest - seenWindow
		for b := range s.seen {
			if b < cutoff {
				delete(s.seen, b)
			}
		}
	}
	return true
}

// pollLoop 在 WS 断开期间做兜底轮询，同时按 backoff 间隔尝试退出以重连 WS。
// 返回 false 表示 ctx 已取消；返回 true 表示应重连 WS。
func (s *Source) pollLoop(ctx context.Context, backoff time.Duration) bool {
	s.log.Info("launchsource poll fallback active", "backoff", backoff.String())
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	reconnect := time.NewTimer(backoff)
	defer reconnect.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-reconnect.C:
			// 退避到期，返回以重连 WS。
			return true
		case <-t.C:
			if s.logs == nil {
				continue // 无轮询能力，等重连
			}
			if err := s.pollOnce(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("launchsource poll", "err", err)
			}
		}
	}
}

// pollOnce 从 latest 到 head 拉取工厂日志并解码。
func (s *Source) pollOnce(ctx context.Context) error {
	head, err := s.logs.Head(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	from := s.latest + 1
	s.mu.Unlock()
	// 冷启动（latest==0）：不从头扫，只回填最近一个窗口，避免永远追不上 head。
	if s.latest == 0 {
		if head > s.cfg.PollBlockRange {
			from = head - s.cfg.PollBlockRange
		} else {
			from = 0
		}
	}
	if head < from {
		return nil
	}
	to := from + s.cfg.PollBlockRange - 1
	if to > head {
		to = head
	}
	logs, err := s.logs.Logs(ctx, s.cfg.Factory, []string{s.cfg.Topic0}, from, to)
	if err != nil {
		return err
	}
	for _, l := range logs {
		if err := s.emit(ctx, l.BlockNumber, l.TransactionHash, l.LogIndex, l.Topics, l.Data); err != nil {
			s.log.Warn("launchsource poll emit", "err", err)
		}
	}
	// 推进游标（即使无日志）。
	s.mu.Lock()
	if to > s.latest {
		s.latest = to
	}
	s.mu.Unlock()
	return nil
}

// decodeLaunch 按 TokenLaunched(address,address,address,address,uint256,uint256) 解码。
func decodeLaunch(chainID, block uint64, txHash string, topics []string, data string) (model.Candidate, error) {
	if len(topics) < 4 {
		return model.Candidate{}, fmt.Errorf("tokenlaunched: need 4 topics, got %d", len(topics))
	}
	if !hasTopic0(topics[0]) {
		return model.Candidate{}, fmt.Errorf("tokenlaunched: unexpected topic0 %s", topics[0])
	}
	token, err := decodeTopicAddress(topics[1])
	if err != nil {
		return model.Candidate{}, fmt.Errorf("tokenlaunched token: %w", err)
	}
	curve, err := decodeTopicAddress(topics[2])
	if err != nil {
		return model.Candidate{}, fmt.Errorf("tokenlaunched curve: %w", err)
	}
	deployer, err := decodeTopicAddress(topics[3])
	if err != nil {
		return model.Candidate{}, fmt.Errorf("tokenlaunched deployer: %w", err)
	}

	rawData, err := evm.DecodeHex(data)
	if err != nil {
		return model.Candidate{}, fmt.Errorf("tokenlaunched data: %w", err)
	}
	slots, err := evm.SplitSlots(rawData)
	if err != nil {
		return model.Candidate{}, err
	}
	if len(slots) < 3 {
		return model.Candidate{}, fmt.Errorf("tokenlaunched data: need 3 slots, got %d", len(slots))
	}
	pairToken, err := evm.DecodeAddress(slots[0])
	if err != nil {
		return model.Candidate{}, err
	}
	configID, err := evm.DecodeBytes32(slots[1])
	if err != nil {
		return model.Candidate{}, err
	}
	threshold, err := evm.DecodeUint256(slots[2])
	if err != nil {
		return model.Candidate{}, err
	}

	return model.Candidate{
		Source:              model.SourceChain,
		ChainID:             chainID,
		Kind:                "launch",
		Address:             token.String(),
		Curve:               curve.String(),
		Deployer:            deployer.String(),
		PairToken:           pairToken.String(),
		LaunchConfigID:      "0x" + configID.Raw(),
		GraduationThreshold: threshold.String(),
		Block:               block,
		TxHash:              txHash,
	}, nil
}

func hasTopic0(t string) bool {
	return evm.NormalizeHex(t) == evm.TokenLaunchedTopic0
}

// decodeTopicAddress 解码 32 字节 topic 中的地址（右对齐低 20 字节），显式返回错误。
func decodeTopicAddress(hexStr string) (evm.Address, error) {
	raw, err := evm.DecodeHex(hexStr)
	if err != nil {
		return "", fmt.Errorf("topic hex: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("topic length %d, want 32", len(raw))
	}
	return evm.DecodeAddress(raw)
}

func decodeQuantity(s string) (uint64, error) {
	var n uint64
	if _, err := fmt.Sscanf(s, "0x%x", &n); err != nil {
		return 0, fmt.Errorf("invalid quantity %q: %w", s, err)
	}
	return n, nil
}
