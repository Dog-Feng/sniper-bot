// Package observe 消费候选事实：落盘 + 指标。
package observe

import (
	"context"
	"sync"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
	"github.com/Dog-Feng/sniper-bot/internal/metrics"
)

// Service 接收事实并落盘、计数。
type Service struct {
	sink outbound.EventSink
	mu   sync.Mutex
	// 延迟采样：最近 N 条（chain 源）的 detect_latency_ms。
	latencies []int64
	lastSeen  map[string]time.Time
}

// New 构造 observe 服务。
func New(sink outbound.EventSink) *Service {
	return &Service{
		sink:     sink,
		lastSeen: map[string]time.Time{},
	}
}

// OnCandidate 落盘并更新指标。
func (s *Service) OnCandidate(ctx context.Context, c model.Candidate) error {
	switch c.Source {
	case model.SourceChain:
		metrics.ChainDetected.Add(1)
		// 链上源记录检测延迟（本地接收时刻 - 区块时间戳）。
		if !c.BlockTime.IsZero() {
			s.recordLatency(c.ReceivedAt.Sub(c.BlockTime).Milliseconds())
		}
	case model.SourceSocial:
		metrics.SocialDetected.Add(1)
	}
	if c.Verified != nil {
		if c.Verified.Passed {
			metrics.VerifyPass.Add(1)
		} else {
			metrics.VerifyFail.Add(1)
		}
	}
	// 记录本事实的接收时刻（用于失明检测，语义正确）。
	s.mu.Lock()
	s.lastSeen[string(c.Source)] = c.ReceivedAt
	s.mu.Unlock()
	return s.sink.Append(ctx, c)
}

// LastSeen 返回各源最近一次收到事实的接收时刻。
func (s *Service) LastSeen(src model.Source) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.lastSeen[string(src)]
	return t, ok
}

func (s *Service) recordLatency(ms int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies = append(s.latencies, ms)
	if len(s.latencies) > 1000 {
		s.latencies = s.latencies[len(s.latencies)-1000:]
	}
}

// LatencyPercentile 返回检测延迟的 p50/p90/p99（毫秒）；样本不足返回 0。
func (s *Service) LatencyPercentile(p float64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0
	}
	sorted := append([]int64(nil), s.latencies...)
	// 插入排序（样本 ≤1000，足够）。
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
