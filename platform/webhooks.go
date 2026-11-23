package platform

import (
	"context"
	"sort"
)

// Webhook pushes every message of a topic or of a queue to a URL. Delivery is
// at least once; undeliverable messages end up in DeadLetterQueue.
type Webhook struct {
	Name                string   `json:"name"`
	Topic               string   `json:"topic"`
	Filter              string   `json:"filter"`
	Queue               string   `json:"queue"`
	URL                 string   `json:"url"`
	HasSecret           bool     `json:"hasSecret"`
	HeaderNames         []string `json:"headerNames"`
	Enabled             bool     `json:"enabled"`
	State               string   `json:"state"`
	PauseReason         string   `json:"pauseReason"`
	ConsecutiveFailures int      `json:"consecutiveFailures"`
	LastStatus          int      `json:"lastStatus"`
	LastError           string   `json:"lastError"`
	LastAttemptAt       string   `json:"lastAttemptAt"`
	LastSuccessAt       string   `json:"lastSuccessAt"`
	DeliveredCount      float64  `json:"deliveredCount"`
	DeadLetteredCount   float64  `json:"deadLetteredCount"`
	DeadLetterQueue     string   `json:"deadLetterQueue"`
	CreatedAt           string   `json:"createdAt"`
}

// WebhookInput creates a webhook. Set exactly one of Topic and Queue.
type WebhookInput struct {
	Name   string
	Topic  string
	Filter string
	Queue  string
	URL    string
	// Secret signs each delivery (HMAC-SHA256 over the timestamp and the body).
	Secret string
	// Headers are sent with every delivery.
	Headers map[string]string
}

// WebhookUpdate changes a webhook. Nil fields are left as they are; an empty
// Secret removes the secret, and Headers replaces the whole set.
type WebhookUpdate struct {
	URL     *string
	Secret  *string
	Headers map[string]string
	// ReplaceHeaders sends Headers even when it is empty, removing them all.
	ReplaceHeaders bool
	Enabled        *bool
}

const webhookFields = `name topic filter queue url hasSecret headerNames enabled state pauseReason consecutiveFailures lastStatus lastError lastAttemptAt lastSuccessAt deliveredCount deadLetteredCount deadLetterQueue createdAt`

func headerList(headers map[string]string) []map[string]any {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{"name": name, "value": headers[name]})
	}
	return out
}

// ListWebhooks returns the tenant's webhooks.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var out struct {
		Items []Webhook `json:"queueWebhooks"`
	}
	err := c.Do(ctx, `query { queueWebhooks { `+webhookFields+` } }`, nil, &out)
	return out.Items, err
}

// GetWebhook returns one webhook; IsNotFound(err) when there is none.
func (c *Client) GetWebhook(ctx context.Context, name string) (*Webhook, error) {
	var out struct {
		Webhook Webhook `json:"queueWebhook"`
	}
	if err := c.Do(ctx, `query($name: String!) { queueWebhook(name: $name) { `+webhookFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Webhook, nil
}

// CreateWebhook creates a webhook. It starts enabled.
func (c *Client) CreateWebhook(ctx context.Context, in WebhookInput) (*Webhook, error) {
	input := map[string]any{"name": in.Name, "url": in.URL}
	for key, value := range map[string]string{"topic": in.Topic, "filter": in.Filter, "queue": in.Queue, "secret": in.Secret} {
		if value != "" {
			input[key] = value
		}
	}
	if len(in.Headers) > 0 {
		input["headers"] = headerList(in.Headers)
	}
	var out struct {
		Webhook Webhook `json:"createQueueWebhook"`
	}
	err := c.Do(ctx, `mutation($input: CreateQueueWebhookInput!) { createQueueWebhook(input: $input) { `+webhookFields+` } }`,
		map[string]any{"input": input}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Webhook, nil
}

// UpdateWebhook changes a webhook's destination, secret, headers or enabled flag.
func (c *Client) UpdateWebhook(ctx context.Context, name string, up WebhookUpdate) (*Webhook, error) {
	input := map[string]any{"name": name}
	if up.URL != nil {
		input["url"] = *up.URL
	}
	if up.Secret != nil {
		input["secret"] = *up.Secret
	}
	if up.Headers != nil || up.ReplaceHeaders {
		input["headers"] = headerList(up.Headers)
	}
	if up.Enabled != nil {
		input["enabled"] = *up.Enabled
	}
	var out struct {
		Webhook Webhook `json:"updateQueueWebhook"`
	}
	err := c.Do(ctx, `mutation($input: UpdateQueueWebhookInput!) { updateQueueWebhook(input: $input) { `+webhookFields+` } }`,
		map[string]any{"input": input}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Webhook, nil
}

// ResumeWebhook restarts delivery of a webhook the platform paused after
// repeated failures.
func (c *Client) ResumeWebhook(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { resumeQueueWebhook(name: $name) { name } }`, map[string]any{"name": name}, nil)
}

// DeleteWebhook deletes a webhook.
func (c *Client) DeleteWebhook(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteQueueWebhook(name: $name) }`, map[string]any{"name": name}, nil)
}

// ScheduledMessage publishes a fixed message on a cron schedule.
type ScheduledMessage struct {
	Name        string  `json:"name"`
	Schedule    string  `json:"schedule"`
	Queue       string  `json:"queue"`
	Topic       string  `json:"topic"`
	RoutingKey  string  `json:"routingKey"`
	Body        string  `json:"body"`
	ContentType string  `json:"contentType"`
	Enabled     bool    `json:"enabled"`
	NextRunAt   string  `json:"nextRunAt"`
	LastRunAt   string  `json:"lastRunAt"`
	LastStatus  string  `json:"lastStatus"`
	LastError   string  `json:"lastError"`
	RunCount    float64 `json:"runCount"`
	CreatedAt   string  `json:"createdAt"`
}

// ScheduledMessageInput creates or replaces a scheduled message. Set exactly
// one of Queue and Topic. Schedule is a five-field cron expression in UTC.
type ScheduledMessageInput struct {
	Name        string
	Schedule    string
	Queue       string
	Topic       string
	RoutingKey  string
	Body        string
	ContentType string
	Enabled     bool
}

const scheduledMessageFields = `name schedule queue topic routingKey body contentType enabled nextRunAt lastRunAt lastStatus lastError runCount createdAt`

func (in ScheduledMessageInput) variables() map[string]any {
	input := map[string]any{"name": in.Name, "schedule": in.Schedule, "body": in.Body, "enabled": in.Enabled}
	for key, value := range map[string]string{"queue": in.Queue, "topic": in.Topic, "routingKey": in.RoutingKey, "contentType": in.ContentType} {
		if value != "" {
			input[key] = value
		}
	}
	return map[string]any{"input": input}
}

// ListScheduledMessages returns the tenant's scheduled messages.
func (c *Client) ListScheduledMessages(ctx context.Context) ([]ScheduledMessage, error) {
	var out struct {
		Items []ScheduledMessage `json:"scheduledMessages"`
	}
	err := c.Do(ctx, `query { scheduledMessages { `+scheduledMessageFields+` } }`, nil, &out)
	return out.Items, err
}

// GetScheduledMessage returns one scheduled message; IsNotFound(err) when there is none.
func (c *Client) GetScheduledMessage(ctx context.Context, name string) (*ScheduledMessage, error) {
	var out struct {
		Message ScheduledMessage `json:"scheduledMessage"`
	}
	if err := c.Do(ctx, `query($name: String!) { scheduledMessage(name: $name) { `+scheduledMessageFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

// CreateScheduledMessage creates a scheduled message.
func (c *Client) CreateScheduledMessage(ctx context.Context, in ScheduledMessageInput) (*ScheduledMessage, error) {
	var out struct {
		Message ScheduledMessage `json:"createScheduledMessage"`
	}
	if err := c.Do(ctx, `mutation($input: ScheduledMessageInput!) { createScheduledMessage(input: $input) { `+scheduledMessageFields+` } }`, in.variables(), &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

// UpdateScheduledMessage replaces a scheduled message's settings.
func (c *Client) UpdateScheduledMessage(ctx context.Context, in ScheduledMessageInput) (*ScheduledMessage, error) {
	var out struct {
		Message ScheduledMessage `json:"updateScheduledMessage"`
	}
	if err := c.Do(ctx, `mutation($input: ScheduledMessageInput!) { updateScheduledMessage(input: $input) { `+scheduledMessageFields+` } }`, in.variables(), &out); err != nil {
		return nil, err
	}
	return &out.Message, nil
}

// RunScheduledMessage publishes the message now, outside its schedule.
func (c *Client) RunScheduledMessage(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { runScheduledMessage(name: $name) { name } }`, map[string]any{"name": name}, nil)
}

// DeleteScheduledMessage deletes a scheduled message.
func (c *Client) DeleteScheduledMessage(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteScheduledMessage(name: $name) }`, map[string]any{"name": name}, nil)
}
