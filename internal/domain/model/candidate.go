package model

import "time"

// Source 标记候选事实的来源：链上事件（可信）或推文（待校验）。
type Source string

const (
	SourceChain  Source = "chain"
	SourceSocial Source = "social"
)

// Candidate 是监控阶段统一的结构化事实。
// 链上源与推文源共用此结构，用 Source 字段区分。
type Candidate struct {
	Source   Source `json:"source"`
	ChainID  uint64 `json:"chainId"`
	Kind     string `json:"kind"` // launch(链上发射) | announce(预告) | live(已上线)
	Address  string `json:"address,omitempty"`
	Curve    string `json:"curve,omitempty"`
	Deployer string `json:"deployer,omitempty"`

	// 链上源专有字段。
	PairToken           string    `json:"pairToken,omitempty"`
	LaunchConfigID      string    `json:"launchConfigId,omitempty"`
	GraduationThreshold string    `json:"graduationThreshold,omitempty"`
	Block               uint64    `json:"block,omitempty"`
	BlockTime           time.Time `json:"blockTime,omitempty"`
	TxHash              string    `json:"txHash,omitempty"`

	// 推文源专有字段。
	TweetID    string    `json:"tweetId,omitempty"`
	TweetTime  time.Time `json:"tweetTime,omitempty"`
	TweetURL   string    `json:"tweetUrl,omitempty"`
	Handle     string    `json:"handle,omitempty"`
	RawText    string    `json:"rawText,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`

	// 本地接收时刻（单调时钟优先），用于观测延迟。
	ReceivedAt time.Time `json:"receivedAt"`

	// 校验结果；链上源天然可信，推文源由校验闸门填充。
	Verified *Verification `json:"verified,omitempty"`
}

// Verification 是候选地址的只读校验结果。
type Verification struct {
	Passed     bool      `json:"passed"`
	HasCode    bool      `json:"hasCode"`
	IsERC20    bool      `json:"isErc20"`
	IsERC721   bool      `json:"isErc721"`
	IsHoneypot bool      `json:"isHoneypot"`
	Reason     string    `json:"reason,omitempty"`
	CheckedAt  time.Time `json:"checkedAt"`
}

// Tweet 是推文源的输入事实。
type Tweet struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	URL       string    `json:"url,omitempty"`
	Handle    string    `json:"handle"`
}

// Alert 是可选告警通道的最小载体。
type Alert struct {
	Severity string    `json:"severity"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

// Block 是延迟观测所需的最小区块信息。
type Block struct {
	Number    uint64
	Timestamp int64 // Unix 秒
}

// Log 是最小事件日志项（兜底轮询用）。
type Log struct {
	BlockNumber     uint64
	TransactionHash string
	LogIndex        string
	Topics          []string
	Data            string
}
