package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestQueueLifecycleAndWaiter(t *testing.T) {
	g := newGateway(t)
	lists := 0
	g.handle = func(query string, v map[string]any) (any, string, string) {
		switch {
		case strings.Contains(query, "createQueue("):
			if v["name"] != "orders" {
				t.Errorf("name = %v", v["name"])
			}
			return map[string]any{"createQueue": map[string]any{"name": "orders", "status": "PENDING"}}, "", ""
		case strings.Contains(query, "{ queues {"):
			lists++
			status := "PENDING"
			if lists >= 2 {
				status = "READY"
			}
			return map[string]any{"queues": []map[string]any{{"name": "orders", "status": status, "messages": 3}}}, "", ""
		case strings.Contains(query, "deleteQueue("):
			return map[string]any{"deleteQueue": true}, "", ""
		}
		t.Errorf("unexpected query %s", query)
		return nil, "", ""
	}
	c := g.client()
	ctx := context.Background()
	q, err := c.CreateQueue(ctx, "orders")
	if err != nil || q.Status != StatusPending {
		t.Fatalf("create: %+v %v", q, err)
	}
	if err := c.WaitQueueReady(ctx, "orders", WaitOptions{Interval: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	found, err := c.FindQueue(ctx, "orders")
	if err != nil || found == nil || found.Messages != 3 {
		t.Fatalf("find: %+v %v", found, err)
	}
	if missing, err := c.FindQueue(ctx, "other"); err != nil || missing != nil {
		t.Fatalf("a queue that does not exist: %+v %v", missing, err)
	}
	if err := c.DeleteQueue(ctx, "orders"); err != nil {
		t.Fatal(err)
	}
}

func TestWaiterReportsFailureAndTimeout(t *testing.T) {
	g := newGateway(t)
	status := "FAILED"
	g.handle = func(string, map[string]any) (any, string, string) {
		return map[string]any{"topics": []map[string]any{{"name": "events", "status": status, "statusReason": "broker refused"}}}, "", ""
	}
	c := g.client()
	err := c.WaitTopicReady(context.Background(), "events", WaitOptions{Interval: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "broker refused") {
		t.Fatalf("want the failure reason, got %v", err)
	}
	status = "PENDING"
	err = c.WaitTopicReady(context.Background(), "events", WaitOptions{Interval: time.Millisecond, Timeout: 5 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "still PENDING") {
		t.Fatalf("want a timeout, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.WaitTopicReady(ctx, "events", WaitOptions{Interval: time.Hour}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestSubscriptionsCarryTheirTarget(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, v map[string]any) (any, string, string) {
		if strings.Contains(query, "createTopicSubscription(") {
			input := v["input"].(map[string]any)
			if input["topic"] != "events" || input["queue"] != "orders" || input["filter"] != "order.*" {
				t.Errorf("input = %v", input)
			}
			return map[string]any{"createTopicSubscription": map[string]any{
				"id": "abc", "topic": "events", "filter": "order.*", "status": "READY", "target": map[string]any{"queue": "orders"},
			}}, "", ""
		}
		return map[string]any{"topicSubscriptions": []map[string]any{
			{"id": "abc", "topic": "events", "filter": "order.*", "target": map[string]any{"queue": "orders"}},
			{"id": "def", "topic": "events", "filter": "#", "target": map[string]any{"webhook": "notify"}},
		}}, "", ""
	}
	c := g.client()
	sub, err := c.SubscribeQueue(context.Background(), "events", "orders", "order.*")
	if err != nil || sub.ID != "abc" || sub.Queue != "orders" {
		t.Fatalf("subscribe: %+v %v", sub, err)
	}
	hook, err := c.FindSubscription(context.Background(), "", "def")
	if err != nil || hook == nil || hook.Webhook != "notify" || hook.Queue != "" {
		t.Fatalf("find: %+v %v", hook, err)
	}
}

func TestWebhookInputsOnlySendWhatIsSet(t *testing.T) {
	g := newGateway(t)
	var seen map[string]any
	g.handle = func(query string, v map[string]any) (any, string, string) {
		seen = v["input"].(map[string]any)
		key := "createQueueWebhook"
		if strings.Contains(query, "updateQueueWebhook(") {
			key = "updateQueueWebhook"
		}
		return map[string]any{key: map[string]any{"name": "notify", "enabled": true, "deadLetterQueue": "rso-wh-notify-dead"}}, "", ""
	}
	c := g.client()
	hook, err := c.CreateWebhook(context.Background(), WebhookInput{
		Name: "notify", Topic: "events", URL: "https://example.com/hook", Headers: map[string]string{"B": "2", "A": "1"},
	})
	if err != nil || hook.DeadLetterQueue != "rso-wh-notify-dead" {
		t.Fatalf("create: %+v %v", hook, err)
	}
	if _, ok := seen["queue"]; ok || seen["topic"] != "events" {
		t.Fatalf("input = %v", seen)
	}
	headers := seen["headers"].([]any)
	if headers[0].(map[string]any)["name"] != "A" {
		t.Fatalf("headers are not sorted: %v", headers)
	}
	off := false
	if _, err := c.UpdateWebhook(context.Background(), "notify", WebhookUpdate{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen["enabled"] != false {
		t.Fatalf("update sent %v, want only name and enabled", seen)
	}
}

func TestPublishAndReceiveUseTheDataAPI(t *testing.T) {
	g := newGateway(t)
	g.rest = func(path string, body map[string]any) (int, any) {
		switch path {
		case "/queues/v1/queues/orders/messages":
			if body["body"] != "hello" || body["contentType"] != "text/plain" {
				t.Errorf("publish body = %v", body)
			}
			return 200, map[string]any{"routed": true}
		case "/queues/v1/topics/events/messages":
			if body["routingKey"] != "order.created" || body["json"].(map[string]any)["id"] != float64(7) {
				t.Errorf("topic body = %v", body)
			}
			return 200, map[string]any{"routed": false}
		case "/queues/v1/queues/orders/receive":
			return 200, map[string]any{"messages": []map[string]any{{"body": "hello", "routingKey": "orders"}}}
		}
		return 404, map[string]any{"error": "queue not found", "code": "not_found"}
	}
	c := g.client()
	ctx := context.Background()
	if routed, err := c.Publish(ctx, "orders", Message{Body: "hello", ContentType: "text/plain"}); err != nil || !routed {
		t.Fatalf("publish: %v %v", routed, err)
	}
	if routed, err := c.PublishToTopic(ctx, "events", "order.created", Message{JSON: map[string]any{"id": 7}}); err != nil || routed {
		t.Fatalf("topic publish: %v %v", routed, err)
	}
	messages, err := c.Receive(ctx, "orders", 10)
	if err != nil || len(messages) != 1 || messages[0].Body != "hello" {
		t.Fatalf("receive: %+v %v", messages, err)
	}
	_, err = c.Receive(ctx, "missing", 1)
	var dataErr *DataError
	if !errors.As(err, &dataErr) || dataErr.StatusCode != 404 || dataErr.Code != "not_found" {
		t.Fatalf("want a 404 DataError, got %v", err)
	}
}

func TestLabelsRoundTrip(t *testing.T) {
	g := newGateway(t)
	g.handle = func(query string, v map[string]any) (any, string, string) {
		if strings.Contains(query, "setResourceLabels(") {
			labels := v["labels"].([]any)
			if len(labels) != 2 || labels[0].(map[string]any)["key"] != "env" {
				t.Errorf("labels = %v", labels)
			}
			return map[string]any{"setResourceLabels": map[string]any{"name": "media"}}, "", ""
		}
		return map[string]any{"resourceLabels": []map[string]any{{"name": "media", "labels": []map[string]any{{"key": "env", "value": "prod"}}}}}, "", ""
	}
	c := g.client()
	if err := c.SetResourceLabels(context.Background(), ResourceBucket, "media", map[string]string{"team": "a", "env": "prod"}); err != nil {
		t.Fatal(err)
	}
	labels, err := c.GetResourceLabels(context.Background(), ResourceBucket, "media")
	if err != nil || labels["env"] != "prod" {
		t.Fatalf("labels = %v, %v", labels, err)
	}
}
