package x

import (
	"testing"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
)

func TestParseTweet(t *testing.T) {
	tw := model.Tweet{
		ID:        "123",
		Text:      "PONS 即将上线，CA 是 0x331dcbdfa2f5dfd140452311f42406d83734383f，Robinhood 链，明天 20:00",
		CreatedAt: time.Unix(1000, 0),
		URL:       "https://x.com/p/123",
	}
	cfg := Config{Handle: "@project"}
	c := parseTweet(tw, cfg)

	if c.Address != "331dcbdfa2f5dfd140452311f42406d83734383f" {
		t.Errorf("address = %s", c.Address)
	}
	if c.ChainID != 4663 {
		t.Errorf("chainID = %d", c.ChainID)
	}
	if c.Kind != "announce" {
		t.Errorf("kind = %s", c.Kind)
	}
	if c.Confidence < 0.9 {
		t.Errorf("confidence = %f", c.Confidence)
	}
}

func TestParseTweetLive(t *testing.T) {
	tw := model.Tweet{
		ID:   "456",
		Text: "已上线 base 链，合约 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	c := parseTweet(tw, Config{})
	if c.Kind != "live" {
		t.Errorf("kind = %s, want live", c.Kind)
	}
	if c.ChainID != 8453 {
		t.Errorf("chainID = %d, want 8453", c.ChainID)
	}
}

func TestParseTweetNoAddress(t *testing.T) {
	tw := model.Tweet{ID: "789", Text: "感谢社区支持！"}
	c := parseTweet(tw, Config{})
	if c.Address != "" {
		t.Errorf("address should be empty, got %s", c.Address)
	}
	if c.Confidence >= 0.5 {
		t.Errorf("confidence = %f, expected < 0.5", c.Confidence)
	}
}
