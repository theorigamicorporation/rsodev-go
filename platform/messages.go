package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Message is what is published to a queue or a topic. Set Body, or JSON for a
// value the platform encodes as application/json.
type Message struct {
	Body        string
	JSON        any
	ContentType string
	Attributes  map[string]string
	MessageID   string
}

// ReceivedMessage is one message taken from a queue.
type ReceivedMessage struct {
	Body        string            `json:"body"`
	Encoding    string            `json:"encoding"`
	ContentType string            `json:"contentType"`
	Attributes  map[string]string `json:"attributes"`
	MessageID   string            `json:"messageId"`
	PublishedAt string            `json:"publishedAt"`
	Topic       string            `json:"topic"`
	RoutingKey  string            `json:"routingKey"`
	Redelivered bool              `json:"redelivered"`
}

// DataError is an error answer of the queues HTTP API.
type DataError struct {
	StatusCode int
	Message    string
	Code       string
}

// Error implements error.
func (e *DataError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("platform: queues API answered %d: %s (%s)", e.StatusCode, e.Message, e.Code)
	}
	return fmt.Sprintf("platform: queues API answered %d: %s", e.StatusCode, e.Message)
}

// post calls the queues HTTP API, which lives next to the GraphQL endpoint.
func (c *Client) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authorize(ctx, req); err != nil {
		return err
	}
	resp, err := c.cfg.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("platform: calling the queues API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var problem struct {
			Error any    `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(raw, &problem)
		message, _ := problem.Error.(string)
		if message == "" {
			message = truncate(string(raw), 300)
		}
		return &DataError{StatusCode: resp.StatusCode, Message: message, Code: problem.Code}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (m Message) payload() map[string]any {
	p := map[string]any{}
	if m.JSON != nil {
		p["json"] = m.JSON
	} else {
		p["body"] = m.Body
	}
	if m.ContentType != "" {
		p["contentType"] = m.ContentType
	}
	if len(m.Attributes) > 0 {
		p["attributes"] = m.Attributes
	}
	if m.MessageID != "" {
		p["messageId"] = m.MessageID
	}
	return p
}

// Publish puts a message on a queue. It reports whether the message was routed.
func (c *Client) Publish(ctx context.Context, queue string, m Message) (bool, error) {
	var out struct {
		Routed bool `json:"routed"`
	}
	err := c.post(ctx, "/queues/v1/queues/"+url.PathEscape(queue)+"/messages", m.payload(), &out)
	return out.Routed, err
}

// PublishToTopic publishes a message to a topic with a routing key. It reports
// whether any subscription took it.
func (c *Client) PublishToTopic(ctx context.Context, topic, routingKey string, m Message) (bool, error) {
	p := m.payload()
	if routingKey != "" {
		p["routingKey"] = routingKey
	}
	var out struct {
		Routed bool `json:"routed"`
	}
	err := c.post(ctx, "/queues/v1/topics/"+url.PathEscape(topic)+"/messages", p, &out)
	return out.Routed, err
}

// Receive takes up to max messages from a queue (the platform caps max).
//
// A message is acknowledged when it is handed out: a caller that fails after
// Receive loses it. Use AMQP with a QueueCredential, or a Webhook, when a
// message must survive a crashed consumer.
func (c *Client) Receive(ctx context.Context, queue string, max int) ([]ReceivedMessage, error) {
	var out struct {
		Messages []ReceivedMessage `json:"messages"`
	}
	payload := map[string]any{}
	if max > 0 {
		payload["max"] = max
	}
	err := c.post(ctx, "/queues/v1/queues/"+url.PathEscape(queue)+"/receive", payload, &out)
	return out.Messages, err
}
