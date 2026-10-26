package rhc

import (
	"strings"
	"testing"

	"github.com/Dog-Feng/sniper-bot/internal/adapter/out/chain/evm"
)

func TestDecodeLaunch(t *testing.T) {
	token := "0x331dcbdfa2f5dfd140452311f42406d83734383f"
	curve := "0x3605c01432527f1e653c5bb2be6aeb4775d10c48"
	deployer := "0xc5e8cdd7d14e0272c2a6c8570497da5ea47f7ccf"
	threshold := "4200000000000000000" // 4.2 ETH in wei

	// 真实链上 topic 是 32 字节（地址右对齐，左补零到 64 hex 字符）。
	topic32 := func(addr string) string {
		return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(addr, "0x")
	}
	topics := []string{
		"0x" + evm.TokenLaunchedTopic0,
		topic32(token),
		topic32(curve),
		topic32(deployer),
	}

	// data = pairToken(address,32B) ++ configID(bytes32) ++ threshold(uint256)
	data := "0x" +
		strings.Repeat("0", 64) + // pairToken = zero address（32 字节槽）
		"01" + strings.Repeat("00", 31) + // configID = 0x01...01（bytes32）
		"0000000000000000000000000000000000000000000000003a4965bf58a40000" // threshold

	c, err := decodeLaunch(4663, 92729072, "0xtx", topics, data)
	if err != nil {
		t.Fatalf("decodeLaunch: %v", err)
	}
	if c.Address != "0x"+evm.NormalizeHex(token) {
		t.Errorf("address = %s, want %s", c.Address, "0x"+evm.NormalizeHex(token))
	}
	if c.Curve != curve {
		t.Errorf("curve = %s, want %s", c.Curve, curve)
	}
	if c.Deployer != deployer {
		t.Errorf("deployer = %s", c.Deployer)
	}
	if c.PairToken != "0x0000000000000000000000000000000000000000" {
		t.Errorf("pairToken = %s", c.PairToken)
	}
	if c.GraduationThreshold != threshold {
		t.Errorf("threshold = %s, want %s", c.GraduationThreshold, threshold)
	}
	if c.Block != 92729072 {
		t.Errorf("block = %d", c.Block)
	}
	if c.Kind != "launch" || c.Source != "chain" {
		t.Errorf("kind/source = %s/%s", c.Kind, c.Source)
	}
	if c.LaunchConfigID != "0x01"+strings.Repeat("00", 31) {
		t.Errorf("launchConfigId = %s", c.LaunchConfigID)
	}
}

func TestDecodeLaunchWrongTopic(t *testing.T) {
	_, err := decodeLaunch(4663, 1, "0xtx", []string{"0xdead", "0x01", "0x02", "0x03"}, "0x")
	if err == nil {
		t.Fatal("expected error for wrong topic0")
	}
}
