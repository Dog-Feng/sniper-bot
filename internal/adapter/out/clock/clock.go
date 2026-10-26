// Package clock 提供系统时钟实现。
package clock

import (
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
)

// System 是真实系统时钟（含墙钟与单调分量）。
type System struct{}

// Now 返回当前时间。
func (System) Now() time.Time { return time.Now() }

var _ outbound.Clock = System{}
