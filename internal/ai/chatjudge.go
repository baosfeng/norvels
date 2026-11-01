package ai

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL   = "https://api.deepseek.com"
	defaultModel     = "deepseek-chat"
	defaultTimeout   = 60 * time.Second
	defaultBackoff   = 500 * time.Millisecond
	maxResponseBytes = 256 << 10 // 响应体上限，超出按解析失败处理
	maxRetries       = 1         // 5xx 只重试一次，避免把失败放大成账单
)

// Options 控制 ChatJudge 行为；零值可用（New 补齐默认值）。
type Options struct {
	// BaseURL 是 OpenAI 兼容 base（默认 https://api.deepseek.com），末尾斜杠可有可无。
	BaseURL string
	// Model 是模型名（默认 deepseek-chat）。
	Model string
	// APIKey 由调用方传入（本包不读全局 env）；为空时 Judge 直接返回 ErrNoKey。
	APIKey string
	// Timeout 是单次请求总超时（默认 60s）。
	Timeout time.Duration
	// Backoff 是 5xx 重试的指数退避基数（默认 500ms）。
	Backoff time.Duration
	// Seed 打乱 A/B 呈现顺序的种子；nil 表示构造时用 crypto/rand 抽一个。
	Seed *int64
	// HTTPClient 覆盖默认客户端（默认按 Timeout 新建）；测试可注入。
	HTTPClient *http.Client
}

// ChatJudge 是 DeepSeek 兼容（OpenAI 请求形态）的裁决器，并发安全。
type ChatJudge struct {
	opt      Options
	endpoint string
	hc       *http.Client
	mu       sync.Mutex
	rng      *rand.Rand
}

// New 按 opt 构造裁决器；零值字段用默认值补齐。
func New(opt Options) *ChatJudge {
	if opt.BaseURL == "" {
		opt.BaseURL = defaultBaseURL
	}
	if opt.Model == "" {
		opt.Model = defaultModel
	}
	if opt.Timeout <= 0 {
		opt.Timeout = defaultTimeout
	}
	if opt.Backoff <= 0 {
		opt.Backoff = defaultBackoff
	}
	seed := randomSeed()
	if opt.Seed != nil {
		seed = *opt.Seed
	}
	c := &ChatJudge{
		opt:      opt,
		endpoint: strings.TrimSuffix(opt.BaseURL, "/") + "/chat/completions",
		rng:      rand.New(rand.NewSource(seed)),
	}
	c.hc = opt.HTTPClient
	if c.hc == nil {
		c.hc = &http.Client{Timeout: opt.Timeout}
	}
	return c
}

// Judge 裁决一处或多处差异。请求体只含段落正文与上下文，不含 Title。
func (c *ChatJudge) Judge(ctx context.Context, req Request) (Verdict, error) {
	if c.opt.APIKey == "" {
		return Verdict{}, ErrNoKey
	}
	segs := clampSegments(req.Segments)
	if len(segs) == 0 {
		return Verdict{}, ErrNoSegments
	}
	swapped := c.flip()
	body, err := c.marshal(segs, swapped)
	if err != nil {
		return Verdict{}, err
	}
	content, err := c.send(ctx, body)
	if err != nil {
		return Verdict{}, err
	}
	v, err := parseVerdict(content)
	if err != nil {
		return Verdict{}, err
	}
	return mapWinner(v, swapped), nil
}

// flip 决定本次调用是否把 B 放到「版本 A」的位置。
func (c *ChatJudge) flip() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rng.Intn(2) == 1
}

// chatMessage / chatRequest / chatResponse 是 OpenAI 兼容的最小请求与响应形态。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// marshal 构造请求体：只有 system 提示词 + 渲染后的段落正文。
func (c *ChatJudge) marshal(segs []Segment, swapped bool) ([]byte, error) {
	req := chatRequest{
		Model: c.opt.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: renderSegments(segs, swapped)},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai: 构造请求体失败: %w", err)
	}
	return body, nil
}

// send 发一次请求；5xx 退避后重试 maxRetries 次，其余错误立即返回。
func (c *ChatJudge) send(ctx context.Context, body []byte) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.opt.Backoff<<(attempt-1)); err != nil {
				return "", err
			}
		}
		content, err := c.once(ctx, body)
		if err == nil {
			return content, nil
		}
		if !errors.Is(err, ErrServer) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}

// once 发一次 HTTP 请求并取回 choices[0].message.content。
func (c *ChatJudge) once(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ai: 构造 HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.opt.APIKey)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", classifyTransport(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", classifyTransport(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", statusError(resp.StatusCode)
	}
	return extractContent(data)
}

// statusError 把非 2xx 归到「可重试的服务端错误」与「不可重试的客户端错误」两类。
func statusError(status int) error {
	if status >= 500 {
		return fmt.Errorf("%w：状态码 %d", ErrServer, status)
	}
	return fmt.Errorf("%w：状态码 %d", ErrClient, status)
}

// classifyTransport 把传输层错误归到超时 / 取消 / 网络三类。
func classifyTransport(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return err
	}
	var ne net.Error
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return fmt.Errorf("%w：%v", ErrTimeout, err)
	}
	return fmt.Errorf("%w：%v", ErrNetwork, err)
}

// extractContent 解析外层响应并取出回答正文。
func extractContent(data []byte) (string, error) {
	var resp chatResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("%w：外层响应解析失败: %v", ErrBadResponse, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%w：响应里没有 choices", ErrBadResponse)
	}
	return resp.Choices[0].Message.Content, nil
}

// sleep 是可取消的退避等待。
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// randomSeed 用 crypto/rand 抽一个种子（不受调用顺序影响）。
func randomSeed() int64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return time.Now().UnixNano()
	}
	return int64(binary.BigEndian.Uint64(b[:]))
}
