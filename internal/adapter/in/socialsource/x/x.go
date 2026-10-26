// Package x 实现项目方推文源：轮询 + 规则解析出候选地址。
package x

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/inbound"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
)

// Config 是单个项目方账号的监控配置。
type Config struct {
	Handle          string
	PollIntervalSec int
	ChainIDHint     uint64 // 0 表示由解析决定
}

// Client 通过 HTTP 调用 Grok/X API 拉取推文。
type Client struct {
	baseURL string
	apiKey  string
	hc      *http.Client
	clock   outbound.Clock
}

// NewClient 构造客户端。baseURL 是 Grok/X API 端点，apiKey 为凭证。
func NewClient(baseURL, apiKey string, clock outbound.Clock) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		hc:      &http.Client{Timeout: 15 * time.Second},
		clock:   clock,
	}
}

// GetTweets 拉取比 sinceID 新的推文。
// 第一阶段：若未配置真实端点（baseURL 为空），返回 nil 并标记"未接入"。
func (c *Client) GetTweets(ctx context.Context, handle string, sinceID string) ([]model.Tweet, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("social source not configured (empty baseURL)")
	}
	u := c.baseURL
	if !strings.Contains(u, "?") {
		u += "?"
	}
	u += "&handle=" + urlQueryEscape(handle) + "&since_id=" + sinceID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("social api status %d: %s", resp.StatusCode, string(data))
	}
	var tweets []model.Tweet
	if err := json.Unmarshal(data, &tweets); err != nil {
		return nil, err
	}
	return tweets, nil
}

// Source 轮询指定项目方账号，解析候选地址后投递。
type Source struct {
	cfg      Config
	client   outbound.SocialReader
	listener inbound.CandidateListener
	clock    outbound.Clock
	log      *slog.Logger

	mu      sync.Mutex
	sinceID string
}

// New 构造推文源。
func New(cfg Config, client outbound.SocialReader, listener inbound.CandidateListener, clock outbound.Clock, log *slog.Logger) *Source {
	return &Source{cfg: cfg, client: client, listener: listener, clock: clock, log: log}
}

// Run 阻塞轮询，直至 ctx 取消。
func (s *Source) Run(ctx context.Context) error {
	interval := time.Duration(s.cfg.PollIntervalSec) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := s.poll(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("social poll", "handle", s.cfg.Handle, "err", err)
			}
		}
	}
}

func (s *Source) poll(ctx context.Context) error {
	s.mu.Lock()
	since := s.sinceID
	s.mu.Unlock()

	tweets, err := s.client.GetTweets(ctx, s.cfg.Handle, since)
	if err != nil {
		return err
	}
	for _, tw := range tweets {
		c := parseTweet(tw, s.cfg)
		if c.Address == "" && c.Confidence < 0.5 {
			// 非代币公告推文，但仍推进游标，避免重复拉取。
			s.advanceCursor(tw.ID)
			continue
		}
		c.ReceivedAt = s.clock.Now()
		c.Handle = s.cfg.Handle
		if err := s.listener.OnCandidate(ctx, c); err != nil {
			s.log.Warn("social emit", "err", err)
		}
		s.advanceCursor(tw.ID)
	}
	return nil
}

// advanceCursor 推进 sinceID 游标（用字符串比较，推文 ID 按时间单调递增）。
func (s *Source) advanceCursor(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	if id > s.sinceID {
		s.sinceID = id
	}
	s.mu.Unlock()
}

var (
	ethAddrRe = regexp.MustCompile(`(?i)0x[0-9a-fA-F]{40}`)
	// 链线索：小写关键字 → chainID。
	chainHints = map[string]uint64{
		"robinhood": 4663,
		"rhc":       4663,
		"ethereum":  1,
		"eth":       1,
		"base":      8453,
		"bsc":       56,
		"bnb":       56,
		"arbitrum":  42161,
		"arb":       42161,
		"solana":    900, // 非 EVM，仅标记
		"sol":       900,
	}
	liveWords     = []string{"已上线", "上线了", "is live", "now live", "launched", "live now", "已开盘", "开盘"}
	announceWords = []string{"即将", "上线时间", "launching", "coming soon", "即将发行", "发行", "预售", "presale", "fair launch", "tge"}
)

func parseTweet(tw model.Tweet, cfg Config) model.Candidate {
	c := model.Candidate{
		Source:    model.SourceSocial,
		TweetID:   tw.ID,
		TweetTime: tw.CreatedAt,
		TweetURL:  tw.URL,
		RawText:   tw.Text,
	}
	text := tw.Text

	// 1. 提取地址。
	if m := ethAddrRe.FindString(text); m != "" {
		c.Address = strings.ToLower(strings.TrimPrefix(m, "0x"))
	}

	// 2. 链线索。
	low := strings.ToLower(text)
	chainID := cfg.ChainIDHint
	for k, id := range chainHints {
		if strings.Contains(low, k) {
			chainID = id
			break
		}
	}
	c.ChainID = chainID

	// 3. kind 判定。
	kind := "announce"
	for _, w := range liveWords {
		if strings.Contains(low, w) {
			kind = "live"
			break
		}
	}
	if kind == "announce" {
		for _, w := range announceWords {
			if strings.Contains(low, w) {
				kind = "announce"
				break
			}
		}
	}
	c.Kind = kind

	// 4. 置信度。
	c.Confidence = confidence(c)
	return c
}

func confidence(c model.Candidate) float64 {
	score := 0.0
	if c.Address != "" {
		score += 0.5
	}
	if c.ChainID != 0 {
		score += 0.3
	}
	if c.Kind != "" {
		score += 0.2
	}
	return score
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(s, " ", "%20")
}
