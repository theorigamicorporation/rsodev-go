package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Status is where the platform is with creating a queue-like object.
type Status string

// The states of an object the platform creates in the background.
const (
	StatusReady   Status = "READY"
	StatusPending Status = "PENDING"
	StatusFailed  Status = "FAILED"
)

// WaitOptions tune a waiter. The zero value polls every second for two minutes.
type WaitOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

func (o WaitOptions) withDefaults() WaitOptions {
	if o.Interval <= 0 {
		o.Interval = time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 2 * time.Minute
	}
	return o
}

// waitReady polls read until it reports READY. FAILED ends the wait with the
// platform's reason.
func waitReady(ctx context.Context, what string, o WaitOptions, read func(context.Context) (Status, string, error)) error {
	o = o.withDefaults()
	deadline := time.Now().Add(o.Timeout)
	for {
		status, reason, err := read(ctx)
		if err != nil {
			return err
		}
		switch status {
		case StatusReady, "":
			return nil
		case StatusFailed:
			return fmt.Errorf("platform: %s failed: %s", what, reason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("platform: %s is still %s after %s", what, status, o.Timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.Interval):
		}
	}
}

// TaskProgress is the state of a background task, such as cloning an instance.
type TaskProgress struct {
	TaskID   string  `json:"taskId"`
	Status   string  `json:"status"`
	Progress int     `json:"progress"`
	Message  *string `json:"message"`
	Result   *string `json:"result"`
	Error    *string `json:"error"`
}

// WaitForTask polls a background task until it finishes. A task the platform
// no longer knows (results expire) is reported as finished without a result.
func (c *Client) WaitForTask(ctx context.Context, taskID string, interval time.Duration) (*TaskProgress, error) {
	const q = `query($id: ID!) { taskProgress(taskId: $id) { taskId status progress message result error } }`
	for {
		var out struct {
			TaskProgress *TaskProgress `json:"taskProgress"`
		}
		if err := c.Do(ctx, q, map[string]any{"id": taskID}, &out); err != nil {
			return nil, err
		}
		if out.TaskProgress == nil {
			return &TaskProgress{TaskID: taskID, Status: "COMPLETED"}, nil
		}
		switch out.TaskProgress.Status {
		case "COMPLETED":
			return out.TaskProgress, nil
		case "FAILED", "CANCELLED":
			msg := "task " + strings.ToLower(out.TaskProgress.Status)
			if out.TaskProgress.Error != nil && *out.TaskProgress.Error != "" {
				msg += ": " + *out.TaskProgress.Error
			}
			return out.TaskProgress, errors.New(msg)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
