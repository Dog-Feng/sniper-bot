// Package rpc 实现极简 JSON-RPC 客户端（HTTP），供只读调用、区块查询使用。
package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Dog-Feng/sniper-bot/internal/domain/model"
)

// Client 封装单个 HTTP JSON-RPC 端点。并发安全（内部用原子自增 ID）。
type Client struct {
	url string
	hc  *http.Client
	id  atomic.Uint64
}

// New 构造客户端。
func New(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		hc:  &http.Client{Timeout: timeout},
	}
}

func (c *Client) nextID() uint64 {
	return c.id.Add(1)
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: c.nextID(), Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("rpc marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc do: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("rpc read: %w", err)
	}
	var r rpcResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("rpc unmarshal: %w", err)
	}
	if r.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	return r.Result, nil
}

func hexToBytes(raw json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s == "" || s == "0x" {
		return nil, nil
	}
	return hex.DecodeString(s[2:])
}

// Code 返回地址的字节码。
func (c *Client) Code(ctx context.Context, addr string) ([]byte, error) {
	res, err := c.call(ctx, "eth_getCode", []interface{}{addr, "latest"})
	if err != nil {
		return nil, err
	}
	return hexToBytes(res)
}

// Balance 返回原生币余额（wei）。
func (c *Client) Balance(ctx context.Context, addr string) (*big.Int, error) {
	res, err := c.call(ctx, "eth_getBalance", []interface{}{addr, "latest"})
	if err != nil {
		return nil, err
	}
	var s string
	if err := json.Unmarshal(res, &s); err != nil {
		return nil, err
	}
	if s == "" || s == "0x" {
		return new(big.Int), nil
	}
	n, ok := new(big.Int).SetString(s[2:], 16)
	if !ok {
		return nil, fmt.Errorf("invalid balance hex %q", s)
	}
	return n, nil
}

// Call 执行只读 eth_call。
func (c *Client) Call(ctx context.Context, to string, data []byte) ([]byte, error) {
	res, err := c.call(ctx, "eth_call", []interface{}{
		map[string]string{"to": to, "data": "0x" + hex.EncodeToString(data)},
		"latest",
	})
	if err != nil {
		return nil, err
	}
	return hexToBytes(res)
}

// ChainID 返回当前链 ID。
func (c *Client) ChainID(ctx context.Context) (uint64, error) {
	res, err := c.call(ctx, "eth_chainId", []interface{}{})
	if err != nil {
		return 0, err
	}
	var s string
	if err := json.Unmarshal(res, &s); err != nil {
		return 0, err
	}
	n, ok := new(big.Int).SetString(s[2:], 16)
	if !ok {
		return 0, fmt.Errorf("invalid chainId hex %q", s)
	}
	return n.Uint64(), nil
}

// BlockByNumber 返回区块号与时间戳。
func (c *Client) BlockByNumber(ctx context.Context, num uint64) (*model.Block, error) {
	param := "latest"
	if num > 0 {
		param = fmt.Sprintf("0x%x", num)
	}
	res, err := c.call(ctx, "eth_getBlockByNumber", []interface{}{param, false})
	if err != nil {
		return nil, err
	}
	var b struct {
		Number    string `json:"number"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(res, &b); err != nil {
		return nil, err
	}
	if b.Number == "" || b.Number == "0x" {
		return nil, fmt.Errorf("empty block number")
	}
	numOut, ok := new(big.Int).SetString(b.Number[2:], 16)
	if !ok {
		return nil, fmt.Errorf("invalid block number %q", b.Number)
	}
	ts, ok := new(big.Int).SetString(b.Timestamp[2:], 16)
	if !ok {
		return nil, fmt.Errorf("invalid timestamp %q", b.Timestamp)
	}
	return &model.Block{Number: numOut.Uint64(), Timestamp: ts.Int64()}, nil
}

// Log is a minimal Ethereum log entry.
// Logs 执行 eth_getLogs 查询（from/to 为闭区间块号，16 进制字符串参数）。
func (c *Client) Logs(ctx context.Context, address string, topics []string, fromBlock, toBlock uint64) ([]model.Log, error) {
	filter := map[string]interface{}{}
	if address != "" {
		filter["address"] = address
	}
	if len(topics) > 0 {
		filter["topics"] = topics
	}
	params := []interface{}{
		map[string]interface{}{
			"fromBlock": fmt.Sprintf("0x%x", fromBlock),
			"toBlock":   fmt.Sprintf("0x%x", toBlock),
		},
	}
	// merge filter into params[0]
	m := params[0].(map[string]interface{})
	for k, v := range filter {
		m[k] = v
	}
	res, err := c.call(ctx, "eth_getLogs", params)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		BlockNumber     string   `json:"blockNumber"`
		TransactionHash string   `json:"transactionHash"`
		LogIndex        string   `json:"logIndex"`
		Topics          []string `json:"topics"`
		Data            string   `json:"data"`
	}
	if err := json.Unmarshal(res, &raw); err != nil {
		return nil, err
	}
	out := make([]model.Log, 0, len(raw))
	for _, l := range raw {
		bn := new(big.Int)
		if l.BlockNumber != "" && l.BlockNumber != "0x" {
			if _, ok := bn.SetString(l.BlockNumber[2:], 16); !ok {
				return nil, fmt.Errorf("invalid log blockNumber %q", l.BlockNumber)
			}
		}
		out = append(out, model.Log{
			BlockNumber:     bn.Uint64(),
			TransactionHash: l.TransactionHash,
			LogIndex:        l.LogIndex,
			Topics:          l.Topics,
			Data:            l.Data,
		})
	}
	return out, nil
}

// Head 返回最新区块号。
func (c *Client) Head(ctx context.Context) (uint64, error) {
	res, err := c.call(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}
	var s string
	if err := json.Unmarshal(res, &s); err != nil {
		return 0, err
	}
	n, ok := new(big.Int).SetString(s[2:], 16)
	if !ok {
		return 0, fmt.Errorf("invalid block number %q", s)
	}
	return n.Uint64(), nil
}
