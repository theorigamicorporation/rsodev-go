package platform

import "context"

// Bucket events a notification can announce.
const (
	BucketEventCreated = "CREATED"
	BucketEventRemoved = "REMOVED"
)

// BucketNotification sends a message to a queue or a topic when objects in a
// bucket are created or removed.
type BucketNotification struct {
	ID              string   `json:"id"`
	Bucket          string   `json:"bucket"`
	Events          []string `json:"events"`
	KeyPrefix       string   `json:"keyPrefix"`
	KeySuffix       string   `json:"keySuffix"`
	Queue           string   `json:"queue"`
	Topic           string   `json:"topic"`
	RoutingKey      string   `json:"routingKey"`
	CreatedAt       string   `json:"createdAt"`
	Delivered       float64  `json:"delivered"`
	Dropped         float64  `json:"dropped"`
	LastDeliveredAt string   `json:"lastDeliveredAt"`
	LastError       string   `json:"lastError"`
}

// BucketNotificationInput creates a notification. Set exactly one of Queue
// and Topic.
type BucketNotificationInput struct {
	Bucket     string
	Events     []string
	KeyPrefix  string
	KeySuffix  string
	Queue      string
	Topic      string
	RoutingKey string
}

const bucketNotificationFields = `id bucket events keyPrefix keySuffix queue topic routingKey createdAt delivered dropped lastDeliveredAt lastError`

// ListBucketNotifications returns the notifications of one bucket, or of
// every bucket when bucket is empty.
func (c *Client) ListBucketNotifications(ctx context.Context, bucket string) ([]BucketNotification, error) {
	vars := map[string]any{}
	if bucket != "" {
		vars["bucket"] = bucket
	}
	var out struct {
		Items []BucketNotification `json:"bucketNotifications"`
	}
	err := c.Do(ctx, `query($bucket: String) { bucketNotifications(bucket: $bucket) { `+bucketNotificationFields+` } }`, vars, &out)
	return out.Items, err
}

// FindBucketNotification looks a notification up by id: nil without an error
// when the bucket has none with that id.
func (c *Client) FindBucketNotification(ctx context.Context, bucket, id string) (*BucketNotification, error) {
	items, err := c.ListBucketNotifications(ctx, bucket)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, nil
}

// CreateBucketNotification creates a notification. It applies to changes from
// a few seconds after it is created; existing objects are not announced.
func (c *Client) CreateBucketNotification(ctx context.Context, in BucketNotificationInput) (*BucketNotification, error) {
	input := map[string]any{"bucket": in.Bucket, "events": orEmpty(in.Events)}
	for key, value := range map[string]string{
		"keyPrefix": in.KeyPrefix, "keySuffix": in.KeySuffix, "queue": in.Queue, "topic": in.Topic, "routingKey": in.RoutingKey,
	} {
		if value != "" {
			input[key] = value
		}
	}
	var out struct {
		Created BucketNotification `json:"createBucketNotification"`
	}
	err := c.Do(ctx, `mutation($input: CreateBucketNotificationInput!) { createBucketNotification(input: $input) { `+bucketNotificationFields+` } }`,
		map[string]any{"input": input}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Created, nil
}

// DeleteBucketNotification deletes a notification.
func (c *Client) DeleteBucketNotification(ctx context.Context, bucket, id string) error {
	return c.Do(ctx, `mutation($bucket: String!, $id: ID!) { deleteBucketNotification(bucket: $bucket, id: $id) }`,
		map[string]any{"bucket": bucket, "id": id}, nil)
}
