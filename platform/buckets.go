package platform

import "context"

// Bucket is an object storage bucket of the tenant. Name is what the tenant
// called it; Bucket is the name S3 clients use.
type Bucket struct {
	Name        string  `json:"name"`
	Bucket      string  `json:"bucket"`
	CreatedAt   *string `json:"createdAt"`
	SizeBytes   float64 `json:"sizeBytes"`
	ObjectCount float64 `json:"objectCount"`
	Endpoint    string  `json:"endpoint"`
	Region      string  `json:"region"`
}

const bucketFields = `name bucket createdAt sizeBytes objectCount endpoint region`

// ListBuckets lists the tenant's buckets.
func (c *Client) ListBuckets(ctx context.Context) ([]Bucket, error) {
	var out struct {
		Buckets []Bucket `json:"buckets"`
	}
	err := c.Do(ctx, `query { buckets { `+bucketFields+` } }`, nil, &out)
	return out.Buckets, err
}

// FindBucket returns the bucket with the given name, or nil.
func (c *Client) FindBucket(ctx context.Context, name string) (*Bucket, error) {
	buckets, err := c.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range buckets {
		if buckets[i].Name == name {
			return &buckets[i], nil
		}
	}
	return nil, nil
}

// CreateBucket creates a bucket.
func (c *Client) CreateBucket(ctx context.Context, name string) (*Bucket, error) {
	var out struct {
		Bucket Bucket `json:"createBucket"`
	}
	err := c.Do(ctx, `mutation($name: String!) { createBucket(name: $name) { `+bucketFields+` } }`, map[string]any{"name": name}, &out)
	return &out.Bucket, err
}

// DeleteBucket deletes a bucket.
func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteBucket(name: $name) }`, map[string]any{"name": name}, nil)
}

// ObjectStorage is how S3 clients reach the tenant's buckets.
type ObjectStorage struct {
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	PathStyle    bool   `json:"pathStyle"`
	BucketPrefix string `json:"bucketPrefix"`
}

// ObjectStorageInfo returns the S3 endpoint and settings of the region.
func (c *Client) ObjectStorageInfo(ctx context.Context) (*ObjectStorage, error) {
	var out struct {
		Info ObjectStorage `json:"objectStorageInfo"`
	}
	err := c.Do(ctx, `query { objectStorageInfo { endpoint region pathStyle bucketPrefix } }`, nil, &out)
	return &out.Info, err
}

// StorageKey is an S3 access key. SecretAccessKey is only set in the answer
// of CreateStorageKey and cannot be read again.
type StorageKey struct {
	Name            string  `json:"name"`
	AccessKeyID     string  `json:"accessKeyId"`
	SecretAccessKey string  `json:"secretAccessKey,omitempty"`
	CreatedAt       *string `json:"createdAt"`
}

// ListStorageKeys lists the tenant's S3 access keys, without their secrets.
func (c *Client) ListStorageKeys(ctx context.Context) ([]StorageKey, error) {
	var out struct {
		Keys []StorageKey `json:"objectStorageKeys"`
	}
	err := c.Do(ctx, `query { objectStorageKeys { name accessKeyId createdAt } }`, nil, &out)
	return out.Keys, err
}

// CreateStorageKey issues an S3 access key.
func (c *Client) CreateStorageKey(ctx context.Context, name string) (*StorageKey, error) {
	var out struct {
		Key StorageKey `json:"createObjectStorageKey"`
	}
	err := c.Do(ctx, `mutation($name: String!) { createObjectStorageKey(name: $name) { name accessKeyId secretAccessKey createdAt } }`, map[string]any{"name": name}, &out)
	return &out.Key, err
}

// DeleteStorageKey revokes an S3 access key.
func (c *Client) DeleteStorageKey(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteObjectStorageKey(name: $name) }`, map[string]any{"name": name}, nil)
}
