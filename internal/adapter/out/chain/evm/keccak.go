package evm

import (
	"github.com/Dog-Feng/sniper-bot/internal/adapter/out/chain/evm/legacykeccak"
)

// Keccak256 返回输入的标准 Keccak-256 摘要（与 Solidity keccak256 一致）。
func Keccak256(data []byte) [32]byte {
	return legacykeccak.Sum256(data)
}

// EventTopic 计算事件签名（如 "Transfer(address,address,uint256)"）的 topic0。
func EventTopic(signature string) Hash {
	sum := Keccak256([]byte(signature))
	return BytesToHash(sum[:])
}

// FuncSelector 计算函数签名（如 "paused()"）的 4 字节 selector。
func FuncSelector(signature string) []byte {
	sum := Keccak256([]byte(signature))
	return sum[:4]
}
