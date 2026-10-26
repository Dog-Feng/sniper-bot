// Package jsonl 实现非阻塞的 JSONL 落盘，供 observe 使用。
package jsonl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
)

// Sink 将候选事实追加写入单个 JSONL 文件；内部用带缓冲 channel，满则丢弃并计数。
// Close 之后 Append 返回错误（不再投递），保证与后台 goroutine 安全共存。
type Sink struct {
	f       *os.File
	ch      chan model.Candidate
	dropped atomic.Uint64
	werr    atomic.Uint64 // 写失败计数

	closeOnce sync.Once
	closed    atomic.Bool
}

// New 打开（追加模式）指定文件并启动后台写入 goroutine。
func New(path string, buf int) (*Sink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("jsonl open: %w", err)
	}
	s := &Sink{f: f, ch: make(chan model.Candidate, buf)}
	go s.loop()
	return s, nil
}

// Append 非阻塞投递；队列满时丢弃并计数；已关闭时返回错误。
func (s *Sink) Append(ctx context.Context, c model.Candidate) error {
	if s.closed.Load() {
		return fmt.Errorf("jsonl sink closed")
	}
	select {
	case s.ch <- c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.dropped.Add(1)
		return nil
	}
}

func (s *Sink) loop() {
	for c := range s.ch {
		b, err := json.Marshal(c)
		if err != nil {
			continue
		}
		b = append(b, '\n')
		if _, err := s.f.Write(b); err != nil {
			s.werr.Add(1)
		}
	}
}

// Dropped 返回丢弃计数。
func (s *Sink) Dropped() uint64 { return s.dropped.Load() }

// WriteErrors 返回写失败计数。
func (s *Sink) WriteErrors() uint64 { return s.werr.Load() }

// Close 幂等关闭：停止接收并关闭文件。可安全并发调用。
func (s *Sink) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.ch)
		err = s.f.Close()
	})
	return err
}
