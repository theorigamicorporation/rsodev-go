package platform

import (
	"context"
	"fmt"
)

// Queue is a durable message queue of the tenant.
type Queue struct {
	Name                   string  `json:"name"`
	Status                 Status  `json:"status"`
	StatusReason           string  `json:"statusReason"`
	CreatedAt              string  `json:"createdAt"`
	Messages               float64 `json:"messages"`
	MessagesReady          float64 `json:"messagesReady"`
	MessagesUnacknowledged float64 `json:"messagesUnacknowledged"`
	Consumers              int     `json:"consumers"`
}

// Topic copies each message published to it to every matching subscription.
type Topic struct {
	Name              string `json:"name"`
	Status            Status `json:"status"`
	StatusReason      string `json:"statusReason"`
	CreatedAt         string `json:"createdAt"`
	SubscriptionCount int    `json:"subscriptionCount"`
}

// QueuesInfo says whether queues are available and how to reach them.
type QueuesInfo struct {
	Available               bool    `json:"available"`
	HTTPEndpoint            string  `json:"httpEndpoint"`
	AMQPHost                string  `json:"amqpHost"`
	AMQPPort                int     `json:"amqpPort"`
	Vhost                   string  `json:"vhost"`
	MaxMessageBytes         int     `json:"maxMessageBytes"`
	MaxQueueLength          int     `json:"maxQueueLength"`
	MaxQueueBytes           float64 `json:"maxQueueBytes"`
	MessageRetentionSeconds int     `json:"messageRetentionSeconds"`
	MaxReceive              int     `json:"maxReceive"`
}

const (
	queueFields = `name status statusReason createdAt messages messagesReady messagesUnacknowledged consumers`
	topicFields = `name status statusReason createdAt subscriptionCount`
)

// QueuesInfo reads the queue product's availability, endpoints and limits.
func (c *Client) QueuesInfo(ctx context.Context) (*QueuesInfo, error) {
	var out struct {
		Info QueuesInfo `json:"queuesInfo"`
	}
	err := c.Do(ctx, `query { queuesInfo { available httpEndpoint amqpHost amqpPort vhost maxMessageBytes maxQueueLength maxQueueBytes messageRetentionSeconds maxReceive } }`, nil, &out)
	return &out.Info, err
}

// ListQueues returns the tenant's queues.
func (c *Client) ListQueues(ctx context.Context) ([]Queue, error) {
	var out struct {
		Queues []Queue `json:"queues"`
	}
	err := c.Do(ctx, `query { queues { `+queueFields+` } }`, nil, &out)
	return out.Queues, err
}

// GetQueue returns one queue; IsNotFound(err) when there is none.
func (c *Client) GetQueue(ctx context.Context, name string) (*Queue, error) {
	var out struct {
		Queue Queue `json:"queue"`
	}
	if err := c.Do(ctx, `query($name: String!) { queue(name: $name) { `+queueFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Queue, nil
}

// CreateQueue creates a queue. It may still be PENDING; see WaitQueueReady.
func (c *Client) CreateQueue(ctx context.Context, name string) (*Queue, error) {
	var out struct {
		Queue Queue `json:"createQueue"`
	}
	if err := c.Do(ctx, `mutation($name: String!) { createQueue(name: $name) { `+queueFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Queue, nil
}

// DeleteQueue deletes a queue and the messages in it.
func (c *Client) DeleteQueue(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteQueue(name: $name) }`, map[string]any{"name": name}, nil)
}

// PurgeQueue discards the messages of a queue and keeps the queue.
func (c *Client) PurgeQueue(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { purgeQueue(name: $name) }`, map[string]any{"name": name}, nil)
}

// FindQueue looks a queue up in the list: nil without an error when the
// tenant has no such queue.
func (c *Client) FindQueue(ctx context.Context, name string) (*Queue, error) {
	queues, err := c.ListQueues(ctx)
	if err != nil {
		return nil, err
	}
	for i := range queues {
		if queues[i].Name == name {
			return &queues[i], nil
		}
	}
	return nil, nil
}

// WaitQueueReady waits until the platform has created the queue.
func (c *Client) WaitQueueReady(ctx context.Context, name string, o WaitOptions) error {
	return waitReady(ctx, "queue "+name, o, func(ctx context.Context) (Status, string, error) {
		q, err := c.FindQueue(ctx, name)
		if err != nil || q == nil {
			return StatusPending, "", err
		}
		return q.Status, q.StatusReason, nil
	})
}

// ListTopics returns the tenant's topics.
func (c *Client) ListTopics(ctx context.Context) ([]Topic, error) {
	var out struct {
		Topics []Topic `json:"topics"`
	}
	err := c.Do(ctx, `query { topics { `+topicFields+` } }`, nil, &out)
	return out.Topics, err
}

// FindTopic looks a topic up in the list: nil without an error when the
// tenant has no such topic.
func (c *Client) FindTopic(ctx context.Context, name string) (*Topic, error) {
	topics, err := c.ListTopics(ctx)
	if err != nil {
		return nil, err
	}
	for i := range topics {
		if topics[i].Name == name {
			return &topics[i], nil
		}
	}
	return nil, nil
}

// CreateTopic creates a topic. It may still be PENDING; see WaitTopicReady.
func (c *Client) CreateTopic(ctx context.Context, name string) (*Topic, error) {
	var out struct {
		Topic Topic `json:"createTopic"`
	}
	if err := c.Do(ctx, `mutation($name: String!) { createTopic(name: $name) { `+topicFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Topic, nil
}

// DeleteTopic deletes a topic.
func (c *Client) DeleteTopic(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteTopic(name: $name) }`, map[string]any{"name": name}, nil)
}

// WaitTopicReady waits until the platform has created the topic.
func (c *Client) WaitTopicReady(ctx context.Context, name string, o WaitOptions) error {
	return waitReady(ctx, "topic "+name, o, func(ctx context.Context) (Status, string, error) {
		t, err := c.FindTopic(ctx, name)
		if err != nil || t == nil {
			return StatusPending, "", err
		}
		return t.Status, t.StatusReason, nil
	})
}

// PlatformEventsTopic is the topic that carries the platform's events for the tenant.
const PlatformEventsTopic = "rso-events"

// Subscription delivers a topic's messages to a queue or to a webhook.
type Subscription struct {
	ID     string `json:"id"`
	Topic  string `json:"topic"`
	Filter string `json:"filter"`
	// Queue or Webhook names the target; exactly one is set.
	Queue        string `json:"-"`
	Webhook      string `json:"-"`
	Status       Status `json:"status"`
	StatusReason string `json:"statusReason"`
	CreatedAt    string `json:"createdAt"`
}

type subscriptionWire struct {
	Subscription
	Target struct {
		Queue   string `json:"queue"`
		Webhook string `json:"webhook"`
	} `json:"target"`
}

func (w subscriptionWire) value() Subscription {
	s := w.Subscription
	s.Queue, s.Webhook = w.Target.Queue, w.Target.Webhook
	return s
}

const subscriptionFields = `id topic filter status statusReason createdAt target { ... on QueueSubscriptionTarget { queue } ... on WebhookSubscriptionTarget { webhook } }`

// ListSubscriptions returns the subscriptions of one topic, or of every topic
// when topic is empty.
func (c *Client) ListSubscriptions(ctx context.Context, topic string) ([]Subscription, error) {
	var out struct {
		Items []subscriptionWire `json:"topicSubscriptions"`
	}
	vars := map[string]any{}
	if topic != "" {
		vars["topic"] = topic
	}
	if err := c.Do(ctx, `query($topic: String) { topicSubscriptions(topic: $topic) { `+subscriptionFields+` } }`, vars, &out); err != nil {
		return nil, err
	}
	subs := make([]Subscription, 0, len(out.Items))
	for _, item := range out.Items {
		subs = append(subs, item.value())
	}
	return subs, nil
}

// FindSubscription looks a subscription up by id, in one topic or (topic
// empty) in all: nil without an error when there is none.
func (c *Client) FindSubscription(ctx context.Context, topic, id string) (*Subscription, error) {
	subs, err := c.ListSubscriptions(ctx, topic)
	if err != nil {
		return nil, err
	}
	for i := range subs {
		if subs[i].ID == id {
			return &subs[i], nil
		}
	}
	return nil, nil
}

// SubscribeQueue delivers a topic's messages into a queue. An empty filter
// matches every message; otherwise it is a pattern on the routing key
// (dot-separated words, * for one word, # for any number).
func (c *Client) SubscribeQueue(ctx context.Context, topic, queue, filter string) (*Subscription, error) {
	input := map[string]any{"topic": topic, "queue": queue}
	if filter != "" {
		input["filter"] = filter
	}
	var out struct {
		Created subscriptionWire `json:"createTopicSubscription"`
	}
	err := c.Do(ctx, `mutation($input: CreateTopicSubscriptionInput!) { createTopicSubscription(input: $input) { `+subscriptionFields+` } }`,
		map[string]any{"input": input}, &out)
	if err != nil {
		return nil, err
	}
	sub := out.Created.value()
	return &sub, nil
}

// DeleteSubscription removes a subscription by id.
func (c *Client) DeleteSubscription(ctx context.Context, id string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteTopicSubscription(id: $id) }`, map[string]any{"id": id}, nil)
}

// WaitSubscriptionReady waits until the platform has created the subscription.
func (c *Client) WaitSubscriptionReady(ctx context.Context, topic, id string, o WaitOptions) error {
	return waitReady(ctx, "subscription "+id, o, func(ctx context.Context) (Status, string, error) {
		s, err := c.FindSubscription(ctx, topic, id)
		if err != nil || s == nil {
			return StatusPending, "", err
		}
		return s.Status, s.StatusReason, nil
	})
}

// QueueCredential is an AMQP login for the tenant's queues. Password and URI
// are only set in the answer to CreateQueueCredential.
type QueueCredential struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	CreatedAt string `json:"createdAt"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Vhost     string `json:"vhost"`
	URI       string `json:"uri"`
}

// ListQueueCredentials returns the tenant's AMQP logins, without secrets.
func (c *Client) ListQueueCredentials(ctx context.Context) ([]QueueCredential, error) {
	var out struct {
		Items []QueueCredential `json:"queueCredentials"`
	}
	err := c.Do(ctx, `query { queueCredentials { name status createdAt } }`, nil, &out)
	return out.Items, err
}

// CreateQueueCredential issues an AMQP login. The password is in the answer
// and cannot be read again.
func (c *Client) CreateQueueCredential(ctx context.Context, name string) (*QueueCredential, error) {
	var out struct {
		Created QueueCredential `json:"createQueueCredential"`
	}
	err := c.Do(ctx, `mutation($name: String!) { createQueueCredential(name: $name) { name username password host port vhost uri createdAt } }`,
		map[string]any{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Created, nil
}

// DeleteQueueCredential revokes an AMQP login.
func (c *Client) DeleteQueueCredential(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteQueueCredential(name: $name) }`, map[string]any{"name": name}, nil)
}

// String keeps a credential's secret out of logs.
func (q QueueCredential) String() string {
	return fmt.Sprintf("QueueCredential{Name: %s, Username: %s}", q.Name, q.Username)
}
