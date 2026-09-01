package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// client is a thin OpenAI-compatible chat completions client. It is shared by
// the local (Ollama, llama.cpp) and remote (any OpenAI-compatible endpoint)
// providers. The HTTP transport is injectable so tests exercise the request
// and response handling without opening a socket.
type client struct {
	baseURL string
	model   string
	key     string
	hc      *http.Client
}

func newClient(baseURL, model, apiKey string) *client {
	return &client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		key:     apiKey,
		hc: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ping reports whether the endpoint answers. Any response below 500 counts as
// reachable; 401/403 mean the server is up but may need a key.
func (c *client) ping(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < 500
}

// chatCompletion mirrors the OpenAI-compatible response shape we consume.
type chatCompletion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *client) Complete(ctx context.Context, req Request) (Result, error) {
	model := c.model
	if req.Model != "" {
		model = req.Model
	}

	messages := []map[string]string{{"role": "user", "content": req.Prompt}}
	if req.System != "" {
		messages = append([]map[string]string{{"role": "system", "content": req.System}}, messages...)
	}
	body := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("marshal request: %w", err)
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		hreq.Header.Set("Authorization", "Bearer "+c.key)
	}

	resp, err := c.hc.Do(hreq)
	if err != nil {
		return Result{}, fmt.Errorf("post completion: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return Result{}, fmt.Errorf("completion endpoint %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var cc chatCompletion
	if err := json.Unmarshal(data, &cc); err != nil {
		return Result{}, fmt.Errorf("decode completion: %w", err)
	}
	if len(cc.Choices) == 0 || cc.Choices[0].Message.Content == "" {
		return Result{}, fmt.Errorf("completion returned no content")
	}

	res := Result{
		Content: cc.Choices[0].Message.Content,
		Model:   model,
	}
	if cc.Usage != nil {
		res.Usage = Usage{
			PromptTokens:     cc.Usage.PromptTokens,
			CompletionTokens: cc.Usage.CompletionTokens,
			TotalTokens:      cc.Usage.TotalTokens,
		}
	}
	return res, nil
}
