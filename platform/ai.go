package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// AIModel is a model the platform can run inference on.
type AIModel struct {
	Name         string   `json:"name"`
	Size         float64  `json:"size"`
	Status       string   `json:"status"`
	Loaded       bool     `json:"loaded"`
	Capabilities []string `json:"capabilities"`
}

// ListAIModels lists the models available for inference.
func (c *Client) ListAIModels(ctx context.Context) ([]AIModel, error) {
	var out struct {
		Models struct {
			Items []AIModel `json:"items"`
		} `json:"aiModels"`
	}
	err := c.Do(ctx, `query { aiModels { items { name size status loaded capabilities } } }`, nil, &out)
	return out.Models.Items, err
}

// ChatMessage is one turn of a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatUsage is the token count of one completion; it is what the tenant is
// metered for.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Chat asks a model for a completion and calls onText with each piece of
// the answer as it arrives. A request may wait its turn; the wait is silent.
// The route follows the widely used chat completions format, so existing
// client libraries work against it as well.
func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage, onText func(string)) (*ChatUsage, error) {
	payload, err := json.Marshal(map[string]any{"model": model, "messages": messages, "stream": true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/ai/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authorize(ctx, req); err != nil {
		return nil, err
	}
	// A completion can outlast the default request timeout.
	streaming := *c.cfg.httpClient
	streaming.Timeout = 0
	resp, err := streaming.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, restError(resp.StatusCode, body)
	}

	var usage *ChatUsage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Comment lines keep the connection alive while the request waits.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *ChatUsage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error != nil {
			return usage, &Error{StatusCode: http.StatusBadGateway, Message: chunk.Error.Message, code: chunk.Error.Code}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && onText != nil {
				onText(choice.Delta.Content)
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	return usage, scanner.Err()
}

// restError turns the JSON error body of a REST route into an Error. The
// routes answer {"error": "text"} or {"error": {"message", "code"}}.
func restError(status int, body []byte) error {
	e := &Error{StatusCode: status}
	var parsed struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Error) > 0 {
		var text string
		var object struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		switch {
		case json.Unmarshal(parsed.Error, &text) == nil:
			e.Message, e.code = text, parsed.Code
		case json.Unmarshal(parsed.Error, &object) == nil:
			e.Message, e.code = object.Message, object.Code
		}
	}
	if e.Message == "" {
		e.Message = "the API answered " + http.StatusText(status) + ": " + truncate(string(body), 200)
	}
	return e
}
