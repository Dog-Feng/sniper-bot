// Package verify 提供候选地址的只读校验闸门。
package verify

import (
	"context"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/adapter/out/chain/evm"
	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
	"github.com/Dog-Feng/sniper-bot/internal/domain/port/outbound"
)

// Service 校验候选地址（合约存在、ERC20/ERC721、蜜罐粗筛）。
type Service struct {
	r outbound.ChainReader
}

// New 构造校验服务。
func New(r outbound.ChainReader) *Service { return &Service{r: r} }

// Verify 对候选地址做只读校验，返回填充好的 Verification。
func (s *Service) Verify(ctx context.Context, addr string) *model.Verification {
	v := &model.Verification{CheckedAt: time.Now()}

	// 1. 合约存在（有字节码）。
	code, err := s.r.Code(ctx, addr)
	if err != nil {
		v.Reason = "code read: " + err.Error()
		return v
	}
	if len(code) == 0 {
		v.Reason = "no code (EOA or not deployed)"
		return v
	}
	v.HasCode = true

	// 2. ERC-721 探测（supportsInterface）。
	is721 := s.supportsInterface(ctx, addr, evm.ERC721InterfaceID)
	v.IsERC721 = is721

	// 3. ERC-20 探测（name/symbol 可读）。
	if s.readableString(ctx, addr, evm.SelectorName) && s.readableString(ctx, addr, evm.SelectorSymbol) {
		v.IsERC20 = true
	}

	// 4. 蜜罐粗筛：1A 仅做占位，默认不判定为蜜罐。
	//    真实检测需模拟买卖（transfer/approve 对成交的约束）与黑名单探测，
	//    留到后续阶段实现，避免朴素字节匹配产生大量误报。
	v.IsHoneypot = false

	if !v.IsERC20 && !v.IsERC721 {
		v.Reason = "not ERC20/ERC721"
		return v
	}
	if v.IsHoneypot {
		v.Reason = "honeypot marker detected"
		return v
	}

	v.Passed = true
	return v
}

func (s *Service) supportsInterface(ctx context.Context, addr, ifaceID string) bool {
	sel := evm.SelectorSupportsInterface
	arg := evm.Bytes4Slot(mustHex(ifaceID))
	out, err := s.r.Call(ctx, addr, evm.EncodeFunctionCall(sel, [][]byte{arg}))
	if err != nil || len(out) != 32 {
		return false
	}
	ok, err := evm.DecodeBool(out)
	return err == nil && ok
}

func (s *Service) readableString(ctx context.Context, addr string, sel []byte) bool {
	out, err := s.r.Call(ctx, addr, evm.EncodeFunctionCall(sel, nil))
	return err == nil && len(out) >= 32
}

func mustHex(s string) []byte {
	b, _ := evm.DecodeHex(s)
	return b
}
