// Package outbound 定义内核调用外部世界的出站端口。
package outbound

import (
	"context"
	"math/big"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
)

// EventSink 持久化候选事实；实现必须非阻塞（满则丢弃并计数）。
type EventSink interface {
	Append(ctx context.Context, c model.Candidate) error
}

// ChainReader 提供只读链上访问，供校验与延迟观测使用。
type ChainReader interface {
	Code(ctx context.Context, addr string) ([]byte, error)
	Balance(ctx context.Context, addr string) (*big.Int, error)
	Call(ctx context.Context, to string, data []byte) ([]byte, error)
	BlockByNumber(ctx context.Context, num uint64) (*model.Block, error)
	ChainID(ctx context.Context) (uint64, error)
}

// LogReader 提供事件日志查询能力（兜底轮询）。
type LogReader interface {
	Logs(ctx context.Context, address string, topics []string, fromBlock, toBlock uint64) ([]model.Log, error)
	Head(ctx context.Context) (uint64, error)
}

// SocialReader 拉取某账号的推文（比 sinceID 新）。
type SocialReader interface {
	GetTweets(ctx context.Context, handle string, sinceID string) ([]model.Tweet, error)
}

// Notifier 投递告警。
type Notifier interface {
	Notify(ctx context.Context, a model.Alert) error
}

// Clock 抽象时钟（测试用假钟）。
type Clock interface {
	Now() time.Time
}
