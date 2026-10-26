package evm

import (
	"fmt"
	"math/big"
	"strings"
)

// ABI 的最小编解码：只支持第一阶段所需的 uint256/address/bytes32/bool 与静态读取。
// 这是独立实现，避免引入第三方 ABI 库（符合"标准库优先"约束）。

// TokenLaunched 事件：TokenLaunched(address,address,address,address,uint256,uint256)
// topics[1..3]=token,curve,deployer；data=pairToken,launchConfigId,graduationThreshold。
// topic0 由 keccak256("TokenLaunched(address,address,address,address,uint256,uint256)") 计算并已核对。
const TokenLaunchedTopic0 = "8d4aad4953d0ca700d468f3753aa14432d1b35b43ec6409f051fb6aa43a89607"

// 常用函数 selector（4 字节）。
var (
	SelectorSupportsInterface = FuncSelector("supportsInterface(bytes4)")
	SelectorBalanceOf         = FuncSelector("balanceOf(address)")
	SelectorName              = FuncSelector("name()")
	SelectorSymbol            = FuncSelector("symbol()")
)

// ERC721 interface id（bytes4）。
const ERC721InterfaceID = "80ac58cd"

// DecodeUint256 解码 32 字节大端无符号整数。
func DecodeUint256(b []byte) (*big.Int, error) {
	if len(b) != 32 {
		return nil, fmt.Errorf("DecodeUint256: expected 32 bytes, got %d", len(b))
	}
	return new(big.Int).SetBytes(b), nil
}

// DecodeAddress 取 32 字节槽的低 20 字节为地址。
func DecodeAddress(slot []byte) (Address, error) {
	if len(slot) != 32 {
		return "", fmt.Errorf("DecodeAddress: expected 32 bytes, got %d", len(slot))
	}
	return BytesToAddress(slot[12:]), nil
}

// DecodeBytes32 返回 32 字节槽。
func DecodeBytes32(slot []byte) (Hash, error) {
	if len(slot) != 32 {
		return "", fmt.Errorf("DecodeBytes32: expected 32 bytes, got %d", len(slot))
	}
	return BytesToHash(slot), nil
}

// DecodeBool 返回 32 字节槽最低位是否为 true。
func DecodeBool(slot []byte) (bool, error) {
	if len(slot) != 32 {
		return false, fmt.Errorf("DecodeBool: expected 32 bytes, got %d", len(slot))
	}
	for i := 0; i < 31; i++ {
		if slot[i] != 0 {
			return false, fmt.Errorf("DecodeBool: invalid encoding")
		}
	}
	return slot[31] != 0, nil
}

// SplitSlots 将 data 切成 32 字节槽（data 长度须为 32 的倍数）。
func SplitSlots(data []byte) ([][]byte, error) {
	if len(data)%32 != 0 {
		return nil, fmt.Errorf("SplitSlots: data length %d not multiple of 32", len(data))
	}
	n := len(data) / 32
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		out[i] = data[i*32 : (i+1)*32]
	}
	return out, nil
}

// EncodeFunctionCall 编码静态函数调用（如 supportsInterface(bytes4) / balanceOf(address)）。
// 参数以已编码的 32 字节槽传入。
func EncodeFunctionCall(selector []byte, args [][]byte) []byte {
	out := make([]byte, 0, 4+len(args)*32)
	out = append(out, selector...)
	for _, a := range args {
		out = append(out, a...)
	}
	return out
}

// Uint256Slot 将 *big.Int 编码为 32 字节槽。
func Uint256Slot(v *big.Int) []byte {
	return PadLeft(v.Bytes(), 32)
}

// AddressSlot 将地址编码为 32 字节槽（低 20 字节）。
func AddressSlot(a Address) []byte {
	var slot [32]byte
	copy(slot[12:], a.Bytes())
	return slot[:]
}

// Bytes4Slot 将 4 字节（如 interface id）右对齐到 32 字节槽。
func Bytes4Slot(b []byte) []byte {
	if len(b) > 32 {
		b = b[len(b)-32:]
	}
	return PadLeft(b, 32)
}

// NormalizeHex 去除空白与 0x 前缀，返回小写 hex。
func NormalizeHex(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	return strings.ToLower(s)
}
