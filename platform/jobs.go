package platform

import "context"

// Concurrency policies of a scheduled job.
const (
	ConcurrencyForbid  = "FORBID"
	ConcurrencyAllow   = "ALLOW"
	ConcurrencyReplace = "REPLACE"
)

// JobEnv is one environment variable of a scheduled job: a literal Value, or
// a key of a secret.
type JobEnv struct {
	Name       string `json:"name"`
	Value      string `json:"value,omitempty"`
	SecretName string `json:"secretName,omitempty"`
	SecretKey  string `json:"secretKey,omitempty"`
}

// JobRun is one run of a scheduled job.
type JobRun struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	Manual          bool   `json:"manual"`
	StartedAt       string `json:"startedAt"`
	FinishedAt      string `json:"finishedAt"`
	DurationSeconds int    `json:"durationSeconds"`
	FailedAttempts  int    `json:"failedAttempts"`
	Message         string `json:"message"`
}

// ScheduledJob is a container that runs on a cron schedule.
type ScheduledJob struct {
	Name             string   `json:"name"`
	Image            string   `json:"image"`
	Command          []string `json:"command"`
	Args             []string `json:"args"`
	Env              []JobEnv `json:"env"`
	ImagePullSecrets []string `json:"imagePullSecrets"`
	CPU              float64  `json:"cpu"`
	MemoryMB         int      `json:"memoryMb"`
	Schedule         string   `json:"schedule"`
	Concurrency      string   `json:"concurrency"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	Retries          int      `json:"retries"`
	Suspended        bool     `json:"suspended"`
	CreatedAt        string   `json:"createdAt"`
	NextRunAt        string   `json:"nextRunAt"`
	ActiveRuns       int      `json:"activeRuns"`
	LastRun          *JobRun  `json:"lastRun"`
}

// ScheduledJobInput creates or replaces a scheduled job. TimeoutSeconds and
// Retries fall back to the platform's defaults when nil.
type ScheduledJobInput struct {
	Name             string
	Image            string
	Command          []string
	Args             []string
	Env              []JobEnv
	ImagePullSecrets []string
	CPU              float64
	MemoryMB         int
	// Schedule is a five-field cron expression in UTC.
	Schedule       string
	Concurrency    string
	TimeoutSeconds *int
	Retries        *int
	Suspended      bool
}

const (
	jobRunFields = `name status manual startedAt finishedAt durationSeconds failedAttempts message`
	jobFields    = `name image command args env { name value secretName secretKey } imagePullSecrets cpu memoryMb schedule concurrency timeoutSeconds retries suspended createdAt nextRunAt activeRuns lastRun { ` + jobRunFields + ` }`
)

func orEmpty[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

func (in ScheduledJobInput) variables() map[string]any {
	input := map[string]any{
		"name": in.Name, "image": in.Image, "command": orEmpty(in.Command), "args": orEmpty(in.Args),
		"env": orEmpty(in.Env), "imagePullSecrets": orEmpty(in.ImagePullSecrets),
		"cpu": in.CPU, "memoryMb": in.MemoryMB, "schedule": in.Schedule, "suspended": in.Suspended,
	}
	if in.Concurrency != "" {
		input["concurrency"] = in.Concurrency
	}
	if in.TimeoutSeconds != nil {
		input["timeoutSeconds"] = *in.TimeoutSeconds
	}
	if in.Retries != nil {
		input["retries"] = *in.Retries
	}
	return map[string]any{"input": input}
}

// ListScheduledJobs returns the tenant's scheduled jobs.
func (c *Client) ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error) {
	var out struct {
		Result struct {
			Items []ScheduledJob `json:"items"`
		} `json:"scheduledJobs"`
	}
	err := c.Do(ctx, `query { scheduledJobs { items { `+jobFields+` } } }`, nil, &out)
	return out.Result.Items, err
}

// GetScheduledJob returns one job; IsNotFound(err) when there is none.
func (c *Client) GetScheduledJob(ctx context.Context, name string) (*ScheduledJob, error) {
	var out struct {
		Job ScheduledJob `json:"scheduledJob"`
	}
	if err := c.Do(ctx, `query($name: String!) { scheduledJob(name: $name) { `+jobFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Job, nil
}

// CreateScheduledJob creates a scheduled job.
func (c *Client) CreateScheduledJob(ctx context.Context, in ScheduledJobInput) (*ScheduledJob, error) {
	var out struct {
		Job ScheduledJob `json:"createScheduledJob"`
	}
	if err := c.Do(ctx, `mutation($input: ScheduledJobInput!) { createScheduledJob(input: $input) { `+jobFields+` } }`, in.variables(), &out); err != nil {
		return nil, err
	}
	return &out.Job, nil
}

// UpdateScheduledJob replaces a scheduled job's settings.
func (c *Client) UpdateScheduledJob(ctx context.Context, in ScheduledJobInput) (*ScheduledJob, error) {
	var out struct {
		Job ScheduledJob `json:"updateScheduledJob"`
	}
	if err := c.Do(ctx, `mutation($input: ScheduledJobInput!) { updateScheduledJob(input: $input) { `+jobFields+` } }`, in.variables(), &out); err != nil {
		return nil, err
	}
	return &out.Job, nil
}

// SetScheduledJobSuspended stops or resumes the scheduling of a job.
func (c *Client) SetScheduledJobSuspended(ctx context.Context, name string, suspended bool) error {
	return c.Do(ctx, `mutation($name: String!, $suspended: Boolean!) { setScheduledJobSuspended(name: $name, suspended: $suspended) { name } }`,
		map[string]any{"name": name, "suspended": suspended}, nil)
}

// RunScheduledJob starts a run now, outside the schedule.
func (c *Client) RunScheduledJob(ctx context.Context, name string) (*JobRun, error) {
	var out struct {
		Run JobRun `json:"runScheduledJob"`
	}
	if err := c.Do(ctx, `mutation($name: String!) { runScheduledJob(name: $name) { `+jobRunFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Run, nil
}

// ListScheduledJobRuns returns the recent runs of a job, newest first.
func (c *Client) ListScheduledJobRuns(ctx context.Context, name string, limit int) ([]JobRun, error) {
	vars := map[string]any{"name": name}
	if limit > 0 {
		vars["limit"] = limit
	}
	var out struct {
		Runs []JobRun `json:"scheduledJobRuns"`
	}
	err := c.Do(ctx, `query($name: String!, $limit: Int) { scheduledJobRuns(name: $name, limit: $limit) { `+jobRunFields+` } }`, vars, &out)
	return out.Runs, err
}

// DeleteScheduledJob deletes a scheduled job and its run history.
func (c *Client) DeleteScheduledJob(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteScheduledJob(name: $name) }`, map[string]any{"name": name}, nil)
}

// JobLogs is the output of one run of a scheduled job.
type JobLogs struct {
	Run       *string   `json:"run"`
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

// ScheduledJobLogs returns the output of a run of a job; an empty run means
// the latest one. Only Pod, TailLines and SinceSeconds of the options apply.
func (c *Client) ScheduledJobLogs(ctx context.Context, name, run string, o LogOptions) (*JobLogs, error) {
	var out struct {
		Logs JobLogs `json:"scheduledJobLogs"`
	}
	variables := map[string]any{"name": name, "run": optional(run), "pod": optional(o.Pod)}
	if o.TailLines > 0 {
		variables["tail"] = o.TailLines
	}
	if o.SinceSeconds > 0 {
		variables["since"] = o.SinceSeconds
	}
	err := c.Do(ctx, `query($name: String!, $run: String, $pod: String, $tail: Int, $since: Int) { scheduledJobLogs(name: $name, run: $run, pod: $pod, tailLines: $tail, sinceSeconds: $since) { run truncated lines { timestamp pod container message } } }`, variables, &out)
	return &out.Logs, err
}
