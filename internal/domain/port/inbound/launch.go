// Package inbound 定义驱动内核的入站端口。
package inbound

import (
	"context"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
)

// CandidateListener 由候选事实源（链上/推文）驱动内核。
type CandidateListener interface {
	OnCandidate(ctx context.Context, c model.Candidate) error
}
