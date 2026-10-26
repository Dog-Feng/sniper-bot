// Package monitor 组合 observe + verify，提供统一的候选事实处理服务。
package monitor

import (
	"context"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/inbound"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
	"github.com/Dog-Feng/sniper-bot/internal/domain/service/monitor/observe"
	"github.com/Dog-Feng/sniper-bot/internal/domain/service/monitor/verify"
)

// Service 实现 CandidateListener：校验（按需）+ 观测落盘。
type Service struct {
	obs    *observe.Service
	verify *verify.Service
}

// New 构造 monitor 服务。chainReader 为 nil 时跳过校验闸门（推文源地址不校验）。
func New(sink outbound.EventSink, chainReader outbound.ChainReader) *Service {
	s := &Service{obs: observe.New(sink)}
	if chainReader != nil {
		s.verify = verify.New(chainReader)
	}
	return s
}

// OnCandidate 处理候选事实。
// 链上源天然可信（直接落盘）；推文源先过只读校验再落盘。
func (s *Service) OnCandidate(ctx context.Context, c model.Candidate) error {
	if c.Source == model.SourceSocial && s.verify != nil && c.Address != "" {
		v := s.verify.Verify(ctx, c.Address)
		c.Verified = v
	}
	return s.obs.OnCandidate(ctx, c)
}

// Metrics 暴露观测服务（供组合根读取延迟等指标）。
func (s *Service) Observe() *observe.Service { return s.obs }

// Compile-time 断言实现端口。
var _ inbound.CandidateListener = (*Service)(nil)
