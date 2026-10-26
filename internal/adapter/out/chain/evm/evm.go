// Package evm 提供共享的 EVM 原语：地址、hex、Keccak-256、最小 ABI 编解码。
// 该包是库性质，供各适配器复用，不 import 其他 adapter。
package evm

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Address 统一表示 20 字节地址（小写、无 0x 前缀）。
type Address string

// ParseAddress 校验并规范化地址为小写 40 位 hex（无 0x 前缀）。
func ParseAddress(s string) (Address, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.ToLower(s)
	if len(s) != 40 {
		return "", fmt.Errorf("invalid address length %d: %q", len(s), s)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("invalid address hex: %w", err)
	}
	return Address(s), nil
}

// String 返回带 0x 前缀的小写地址。
func (a Address) String() string { return "0x" + string(a) }

// Raw 返回不带前缀的小写地址。
func (a Address) Raw() string { return string(a) }

// Zero 判断是否零地址。
func (a Address) Zero() bool {
	return strings.Trim(a.Raw(), "0") == ""
}

// Bytes 返回 20 字节切片。
func (a Address) Bytes() []byte {
	b, _ := hex.DecodeString(a.Raw())
	return b
}

// BytesToAddress 由字节切片构造 Address：取低 20 字节（EVM 地址在 32 字节槽中右对齐）。
func BytesToAddress(b []byte) Address {
	var buf [20]byte
	if len(b) >= 20 {
		copy(buf[:], b[len(b)-20:])
	} else {
		copy(buf[20-len(b):], b)
	}
	return Address(hex.EncodeToString(buf[:]))
}

// Hash 统一表示 32 字节值（小写、无 0x 前缀）。
type Hash string

// BytesToHash 由字节切片构造 Hash（不足左补零，超出截取低 32 字节）。
func BytesToHash(b []byte) Hash {
	var buf [32]byte
	if len(b) >= 32 {
		copy(buf[:], b[len(b)-32:])
	} else {
		copy(buf[32-len(b):], b)
	}
	return Hash(hex.EncodeToString(buf[:]))
}

// Raw 返回不带前缀的小写 hex。
func (h Hash) Raw() string { return string(h) }

// String 返回带 0x 前缀的小写 hex。
func (h Hash) String() string { return "0x" + string(h) }

// Topic 返回事件 topic 的 32 字节 hex（带 0x 前缀）。
func Topic(v Hash) string { return v.String() }

// DecodeHex 解码 hex 字符串（容忍 0x 前缀）。
func DecodeHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return nil, nil
	}
	if len(s)%2 != 0 {
		s = "0" + s
	}
	return hex.DecodeString(s)
}

// PadLeft 将字节切片左补零到固定长度。
func PadLeft(b []byte, n int) []byte {
	if len(b) >= n {
		return b[len(b)-n:]
	}
	out := make([]byte, n)
	copy(out[n-len(b):], b)
	return out
}
