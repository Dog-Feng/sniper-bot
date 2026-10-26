// Package config 加载 watch 配置（*.local.json）。
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config 是第一阶段监控配置。
type Config struct {
	Output string `json:"output"` // JSONL 输出路径

	Chains map[string]Chain `json:"chains"`

	ChainTargets  []ChainTarget  `json:"chainTargets"`
	SocialTargets []SocialTarget `json:"socialTargets"`

	Social Social `json:"social"`

	Notify Notify `json:"notify"`
}

// Chain 是单链端点配置。
type Chain struct {
	Name string   `json:"name"`
	WS   string   `json:"ws"`
	HTTP string   `json:"http"`
	RPC  []string `json:"rpc"` // 多端点（fallback）
}

// ChainTarget 是链上工厂订阅目标。
type ChainTarget struct {
	ChainID        uint64 `json:"chainId"`
	Factory        string `json:"factory"`
	EventTopic0    string `json:"eventTopic0"`
	PollBlockRange uint64 `json:"pollBlockRange"`
}

// SocialTarget 是推文监控目标。
type SocialTarget struct {
	Handle          string `json:"handle"`
	PollIntervalSec int    `json:"pollIntervalSec"`
	ChainIDHint     uint64 `json:"chainIdHint,omitempty"`
}

// Notify 告警配置（第一阶段可选）。
type Notify struct {
	Telegram Telegram `json:"telegram"`
}

// Telegram 通知配置。
type Telegram struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatID"`
}

// Social 是社交 API 凭证与端点配置。
type Social struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

// Load 读取配置文件。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config read: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config parse: %w", err)
	}
	if c.Output == "" {
		c.Output = "candidates.jsonl"
	}
	return &c, nil
}
