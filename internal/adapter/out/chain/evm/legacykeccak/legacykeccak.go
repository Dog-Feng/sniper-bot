// Package legacykeccak 暴露已批准依赖 golang.org/x/crypto 的 legacy Keccak-256。
// 单独成包以隔离第三方依赖，符合"domain 零第三方依赖"的约束。
package legacykeccak

import "golang.org/x/crypto/sha3"

// Sum256 返回 data 的 legacy Keccak-256 摘要（与 Solidity keccak256 一致）。
func Sum256(data []byte) [32]byte {
	var out [32]byte
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	copy(out[:], h.Sum(nil))
	return out
}
