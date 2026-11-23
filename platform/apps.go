package platform

import (
	"context"
	"fmt"
)

// KeyValue is a label or an annotation.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SecretKeyRef points at one key of a secret.
type SecretKeyRef struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	Optional *bool  `json:"optional,omitempty"`
}

// EnvSource takes an environment variable's value from a secret.
type EnvSource struct {
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

// EnvVar is an environment variable of a container.
type EnvVar struct {
	Name      string     `json:"name"`
	Value     *string    `json:"value,omitempty"`
	ValueFrom *EnvSource `json:"valueFrom,omitempty"`
}

// ContainerPort is a port a container listens on.
type ContainerPort struct {
	Name          *string `json:"name,omitempty"`
	ContainerPort int     `json:"containerPort"`
}

// ResourceList is an amount of CPU and memory, in Kubernetes notation
// ("500m", "256Mi").
type ResourceList struct {
	CPU    *string `json:"cpu,omitempty"`
	Memory *string `json:"memory,omitempty"`
}

// Resources are what a container asks for and what it is capped at.
type Resources struct {
	Requests *ResourceList `json:"requests,omitempty"`
	Limits   *ResourceList `json:"limits,omitempty"`
}

// VolumeMount puts a volume of the app at a path of the container.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  *bool  `json:"readOnly,omitempty"`
}

// SecretVolume mounts a secret as files.
type SecretVolume struct {
	SecretName string `json:"secretName"`
	Optional   *bool  `json:"optional,omitempty"`
}

// ClaimVolume mounts a persistent volume of the tenant.
type ClaimVolume struct {
	ClaimName string `json:"claimName"`
	ReadOnly  *bool  `json:"readOnly,omitempty"`
}

// AppVolume is a volume containers of the app can mount.
type AppVolume struct {
	Name   string        `json:"name"`
	Secret *SecretVolume `json:"secret,omitempty"`
	Claim  *ClaimVolume  `json:"persistentVolumeClaim,omitempty"`
}

// PullSecret names a registry secret used to pull the app's images.
type PullSecret struct {
	Name string `json:"name"`
}

// AppContainer is one container of an app.
type AppContainer struct {
	Image        string          `json:"image"`
	Env          []EnvVar        `json:"env,omitempty"`
	Ports        []ContainerPort `json:"ports,omitempty"`
	Resources    *Resources      `json:"resources,omitempty"`
	VolumeMounts []VolumeMount   `json:"volumeMounts,omitempty"`
}

// AppSpec is what an app runs.
type AppSpec struct {
	Containers       []AppContainer `json:"containers"`
	ImagePullSecrets []PullSecret   `json:"imagePullSecrets,omitempty"`
	Volumes          []AppVolume    `json:"volumes,omitempty"`
}

// AppTemplateMetadata carries the annotations of an app's revisions, which
// is where scaling bounds are set.
type AppTemplateMetadata struct {
	Annotations []KeyValue `json:"annotations,omitempty"`
}

// AppTemplate describes the revisions of an app.
type AppTemplate struct {
	Metadata *AppTemplateMetadata `json:"metadata,omitempty"`
	Spec     AppSpec              `json:"spec"`
}

// AppMetadata carries the labels and annotations of an app.
type AppMetadata struct {
	Labels      []KeyValue `json:"labels,omitempty"`
	Annotations []KeyValue `json:"annotations,omitempty"`
}

// AppInput describes an app to create or to replace.
type AppInput struct {
	Name     string       `json:"name"`
	Metadata *AppMetadata `json:"metadata,omitempty"`
	Template AppTemplate  `json:"template"`
}

// AppStatus is where an app is.
type AppStatus struct {
	URL                 *string `json:"url"`
	LatestReadyRevision *string `json:"latestReadyRevisionName"`
	LastUpdated         *string `json:"lastUpdatedTimestamp"`
}

// App is a container application of the tenant, served over HTTPS and
// scaled with its traffic.
type App struct {
	Name      string      `json:"name"`
	CreatedAt *string     `json:"creationTimestamp"`
	Metadata  AppMetadata `json:"metadata"`
	Status    AppStatus   `json:"status"`
	Template  AppTemplate `json:"template"`
}

// Ready reports whether the app has a revision that serves traffic.
func (a *App) Ready() bool {
	return a != nil && a.Status.LatestReadyRevision != nil && *a.Status.LatestReadyRevision != ""
}

const appFields = `name creationTimestamp metadata { labels { key value } annotations { key value } } status { url latestReadyRevisionName lastUpdatedTimestamp } template { metadata { annotations { key value } } spec { imagePullSecrets { name } volumes { name secret { secretName optional } persistentVolumeClaim { claimName readOnly } } containers { image env { name value valueFrom { secretKeyRef { name key optional } } } ports { name containerPort } resources { requests { cpu memory } limits { cpu memory } } volumeMounts { name mountPath readOnly } } } }`

// ListApps lists the tenant's apps.
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var out struct {
		Apps struct {
			Items []App `json:"items"`
		} `json:"apps"`
	}
	err := c.Do(ctx, `query($namespace: String) { apps(namespace: $namespace) { items { `+appFields+` } } }`,
		map[string]any{"namespace": c.namespace()}, &out)
	return out.Apps.Items, err
}

// GetApp returns one app. It answers a NOT_FOUND error when there is none.
func (c *Client) GetApp(ctx context.Context, name string) (*App, error) {
	var out struct {
		App *App `json:"app"`
	}
	err := c.Do(ctx, `query($name: String!, $namespace: String) { app(name: $name, namespace: $namespace) { `+appFields+` } }`,
		map[string]any{"name": name, "namespace": c.namespace()}, &out)
	if err != nil {
		return nil, err
	}
	if out.App == nil {
		return nil, notFound("app", name)
	}
	return out.App, nil
}

// FindApp is GetApp that answers nil, nil when the app does not exist.
func (c *Client) FindApp(ctx context.Context, name string) (*App, error) {
	app, err := c.GetApp(ctx, name)
	if IsNotFound(err) {
		return nil, nil
	}
	return app, err
}

// CreateApp creates an app. It returns before the first revision is ready;
// WaitAppReady waits for that.
func (c *Client) CreateApp(ctx context.Context, in AppInput) (*App, error) {
	var out struct {
		App App `json:"createApp"`
	}
	err := c.Do(ctx, `mutation($input: AppInput!) { createApp(input: $input) { `+appFields+` } }`, map[string]any{"input": in}, &out)
	return &out.App, err
}

// UpdateApp replaces an app's description, which rolls out a new revision.
func (c *Client) UpdateApp(ctx context.Context, in AppInput) (*App, error) {
	var out struct {
		App App `json:"updateApp"`
	}
	err := c.Do(ctx, `mutation($input: AppInput!) { updateApp(input: $input) { `+appFields+` } }`, map[string]any{"input": in}, &out)
	return &out.App, err
}

// DeleteApp deletes an app.
func (c *Client) DeleteApp(ctx context.Context, name string) error {
	if c.cfg.tenant == "" {
		return errNoTenant
	}
	return c.Do(ctx, `mutation($name: String!, $namespace: String!) { deleteApp(name: $name, namespace: $namespace) }`,
		map[string]any{"name": name, "namespace": c.cfg.tenant}, nil)
}

// WaitAppReady polls until the app has a revision that serves traffic.
func (c *Client) WaitAppReady(ctx context.Context, name string, o WaitOptions) (*App, error) {
	var app *App
	err := waitReady(ctx, fmt.Sprintf("app %q", name), o, func(ctx context.Context) (Status, string, error) {
		var err error
		if app, err = c.GetApp(ctx, name); err != nil {
			return "", "", err
		}
		if app.Ready() {
			return StatusReady, "", nil
		}
		return StatusPending, "", nil
	})
	return app, err
}

// LogLine is one line of a container's output.
type LogLine struct {
	Timestamp *string `json:"timestamp"`
	Pod       string  `json:"pod"`
	Container string  `json:"container"`
	Message   string  `json:"message"`
}

// LogOptions narrows the output asked for; the zero value is the most recent
// lines of the running containers.
type LogOptions struct {
	Pod          string
	Container    string
	TailLines    int
	SinceSeconds int
	Previous     bool
}

// Logs is recent output of an app's or a job's containers.
type Logs struct {
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

// AppLogs returns the recent output of an app.
func (c *Client) AppLogs(ctx context.Context, name string, o LogOptions) (*Logs, error) {
	var out struct {
		Logs Logs `json:"appLogs"`
	}
	variables := map[string]any{"name": name, "namespace": c.namespace(), "pod": optional(o.Pod), "container": optional(o.Container), "previous": o.Previous}
	if o.TailLines > 0 {
		variables["tail"] = o.TailLines
	}
	if o.SinceSeconds > 0 {
		variables["since"] = o.SinceSeconds
	}
	err := c.Do(ctx, `query($name: String!, $namespace: String, $pod: String, $container: String, $tail: Int, $since: Int, $previous: Boolean) { appLogs(name: $name, namespace: $namespace, pod: $pod, container: $container, tailLines: $tail, sinceSeconds: $since, previous: $previous) { truncated lines { timestamp pod container message } } }`, variables, &out)
	return &out.Logs, err
}
