package platform

import "context"

// PostgresStorage is the disk of each member of a PostgreSQL cluster.
type PostgresStorage struct {
	// Size is in Kubernetes notation, for example "10Gi".
	Size string `json:"size"`
}

// PostgresSpec is the shape of a PostgreSQL cluster.
type PostgresSpec struct {
	// Instances is the number of members: one primary and the rest replicas.
	Instances int             `json:"instances"`
	Storage   PostgresStorage `json:"storage"`
	Resources *Resources      `json:"resources,omitempty"`
}

// PostgresStatus is where a PostgreSQL cluster is.
type PostgresStatus struct {
	Phase          *string `json:"phase"`
	URL            *string `json:"url"`
	Instances      *int    `json:"instances"`
	ReadyInstances *int    `json:"readyInstances"`
}

// PostgresCluster is a managed PostgreSQL cluster of the tenant.
type PostgresCluster struct {
	Name      string          `json:"name"`
	CreatedAt *string         `json:"creationTimestamp"`
	Spec      PostgresSpec    `json:"spec"`
	Status    *PostgresStatus `json:"status"`
}

// Ready reports whether every member of the cluster is up.
func (p *PostgresCluster) Ready() bool {
	return p != nil && p.Status != nil && p.Status.ReadyInstances != nil && *p.Status.ReadyInstances >= p.Spec.Instances
}

// PostgresClusterInput describes a PostgreSQL cluster to create or change.
type PostgresClusterInput struct {
	Name string
	Spec PostgresSpec
}

func (c *Client) postgresInput(in PostgresClusterInput) map[string]any {
	return map[string]any{"name": in.Name, "namespace": c.cfg.tenant, "spec": in.Spec}
}

const postgresFields = `name creationTimestamp spec { instances storage { size } resources { requests { cpu memory } limits { cpu memory } } } status { phase url instances readyInstances }`

// ListPostgresClusters lists the tenant's PostgreSQL clusters.
func (c *Client) ListPostgresClusters(ctx context.Context) ([]PostgresCluster, error) {
	var out struct {
		Clusters struct {
			Items []PostgresCluster `json:"items"`
		} `json:"postgresClustersByCluster"`
	}
	err := c.Do(ctx, `query($namespace: String) { postgresClustersByCluster(namespace: $namespace) { items { `+postgresFields+` } } }`,
		map[string]any{"namespace": c.namespace()}, &out)
	return out.Clusters.Items, err
}

// GetPostgresCluster returns one PostgreSQL cluster.
func (c *Client) GetPostgresCluster(ctx context.Context, name string) (*PostgresCluster, error) {
	var out struct {
		Cluster *PostgresCluster `json:"postgresClusterByCluster"`
	}
	err := c.Do(ctx, `query($name: String!, $namespace: String) { postgresClusterByCluster(name: $name, namespace: $namespace) { `+postgresFields+` } }`,
		map[string]any{"name": name, "namespace": c.namespace()}, &out)
	if err != nil {
		return nil, err
	}
	if out.Cluster == nil {
		return nil, notFound("PostgreSQL cluster", name)
	}
	return out.Cluster, nil
}

// CreatePostgresCluster creates a PostgreSQL cluster. It returns before the
// members are up; WaitPostgresClusterReady waits for that.
func (c *Client) CreatePostgresCluster(ctx context.Context, in PostgresClusterInput) (*PostgresCluster, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Cluster PostgresCluster `json:"createPostgresCluster"`
	}
	err := c.Do(ctx, `mutation($input: PostgresClusterInput!) { createPostgresCluster(input: $input) { `+postgresFields+` } }`,
		map[string]any{"input": c.postgresInput(in)}, &out)
	return &out.Cluster, err
}

// UpdatePostgresCluster changes the members, the resources or the storage of
// a PostgreSQL cluster.
func (c *Client) UpdatePostgresCluster(ctx context.Context, in PostgresClusterInput) (*PostgresCluster, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Cluster PostgresCluster `json:"updatePostgresCluster"`
	}
	err := c.Do(ctx, `mutation($input: PostgresClusterInput!) { updatePostgresCluster(input: $input) { `+postgresFields+` } }`,
		map[string]any{"input": c.postgresInput(in)}, &out)
	return &out.Cluster, err
}

// DeletePostgresCluster deletes a PostgreSQL cluster and its data.
// keepBackups leaves the cluster's backups in place.
func (c *Client) DeletePostgresCluster(ctx context.Context, name string, keepBackups bool) error {
	if c.cfg.tenant == "" {
		return errNoTenant
	}
	return c.Do(ctx, `mutation($name: String!, $namespace: String!, $keep: Boolean!) { deletePostgresCluster(name: $name, namespace: $namespace, keepBackups: $keep) }`,
		map[string]any{"name": name, "namespace": c.cfg.tenant, "keep": keepBackups}, nil)
}

// WaitPostgresClusterReady polls until every member of the cluster is up.
func (c *Client) WaitPostgresClusterReady(ctx context.Context, name string, o WaitOptions) (*PostgresCluster, error) {
	var cluster *PostgresCluster
	err := waitReady(ctx, "PostgreSQL cluster "+name, o, func(ctx context.Context) (Status, string, error) {
		var err error
		if cluster, err = c.GetPostgresCluster(ctx, name); err != nil {
			return "", "", err
		}
		if cluster.Ready() {
			return StatusReady, "", nil
		}
		return StatusPending, "", nil
	})
	return cluster, err
}

// PostgresDatabase is a database inside a PostgreSQL cluster.
type PostgresDatabase struct {
	Cluster string `json:"clusterName"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	// ReclaimPolicy says what happens to the data when the database object
	// is deleted: "retain" or "delete".
	ReclaimPolicy *string `json:"databaseReclaimPolicy,omitempty"`
}

const databaseFields = `clusterName name owner databaseReclaimPolicy`

// ListPostgresDatabases lists the databases of a PostgreSQL cluster.
func (c *Client) ListPostgresDatabases(ctx context.Context, cluster string) ([]PostgresDatabase, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Databases struct {
			Items []PostgresDatabase `json:"items"`
		} `json:"postgresDatabasesByCluster"`
	}
	err := c.Do(ctx, `query($namespace: String!, $cluster: String!) { postgresDatabasesByCluster(namespace: $namespace, clusterName: $cluster) { items { `+databaseFields+` } } }`,
		map[string]any{"namespace": c.cfg.tenant, "cluster": cluster}, &out)
	return out.Databases.Items, err
}

// CreatePostgresDatabase creates a database in a PostgreSQL cluster.
func (c *Client) CreatePostgresDatabase(ctx context.Context, in PostgresDatabase) (*PostgresDatabase, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Database PostgresDatabase `json:"createPostgresDatabase"`
	}
	err := c.Do(ctx, `mutation($namespace: String!, $input: DatabaseInput!) { createPostgresDatabase(namespace: $namespace, input: $input) { `+databaseFields+` } }`,
		map[string]any{"namespace": c.cfg.tenant, "input": in}, &out)
	return &out.Database, err
}

// UpdatePostgresDatabase changes the owner or the reclaim policy of a database.
func (c *Client) UpdatePostgresDatabase(ctx context.Context, in PostgresDatabase) (*PostgresDatabase, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Database PostgresDatabase `json:"updatePostgresDatabase"`
	}
	err := c.Do(ctx, `mutation($namespace: String!, $input: UpdateDatabaseInput!) { updatePostgresDatabase(namespace: $namespace, input: $input) { `+databaseFields+` } }`,
		map[string]any{"namespace": c.cfg.tenant, "input": in}, &out)
	return &out.Database, err
}

// DeletePostgresDatabase deletes a database of a PostgreSQL cluster.
func (c *Client) DeletePostgresDatabase(ctx context.Context, cluster, name string) error {
	if c.cfg.tenant == "" {
		return errNoTenant
	}
	return c.Do(ctx, `mutation($namespace: String!, $cluster: String!, $name: String!) { deletePostgresDatabase(namespace: $namespace, clusterName: $cluster, name: $name) }`,
		map[string]any{"namespace": c.cfg.tenant, "cluster": cluster, "name": name}, nil)
}

// PostgresBackup is one backup of a PostgreSQL cluster.
type PostgresBackup struct {
	Name      string  `json:"name"`
	Phase     *string `json:"phase"`
	Scheduled *bool   `json:"scheduled"`
	StartedAt *string `json:"startedAt"`
	StoppedAt *string `json:"stoppedAt"`
	Error     *string `json:"error"`
}

// PostgresBackupSchedule is when a PostgreSQL cluster is backed up.
type PostgresBackupSchedule struct {
	// Schedule is a cron expression.
	Schedule  string  `json:"schedule"`
	Suspended bool    `json:"suspended"`
	Last      *string `json:"lastScheduleTime"`
	Next      *string `json:"nextScheduleTime"`
}

// PostgresBackups is the backup state of a PostgreSQL cluster.
type PostgresBackups struct {
	// Available says whether backups are offered in this region at all.
	Available bool                    `json:"available"`
	Enabled   bool                    `json:"enabled"`
	Retention *string                 `json:"retention"`
	Schedule  *PostgresBackupSchedule `json:"schedule"`
	// Window is the span of time the cluster can be restored to.
	Window struct {
		ContinuousArchiving bool    `json:"continuousArchiving"`
		From                *string `json:"from"`
		To                  *string `json:"to"`
	} `json:"window"`
	Backups []PostgresBackup `json:"backups"`
}

const postgresBackupFields = `available enabled retention schedule { schedule suspended lastScheduleTime nextScheduleTime } window { continuousArchiving from to } backups { name phase scheduled startedAt stoppedAt error }`

func (c *Client) postgresBackupCall(ctx context.Context, document, field, cluster string, extra map[string]any) (*PostgresBackups, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	variables := map[string]any{"namespace": c.cfg.tenant, "cluster": cluster}
	for key, value := range extra {
		variables[key] = value
	}
	out := map[string]*PostgresBackups{}
	if err := c.Do(ctx, document, variables, &out); err != nil {
		return nil, err
	}
	return out[field], nil
}

// GetPostgresBackups returns the backup state of a PostgreSQL cluster.
func (c *Client) GetPostgresBackups(ctx context.Context, cluster string) (*PostgresBackups, error) {
	return c.postgresBackupCall(ctx, `query($namespace: String!, $cluster: String!) { postgresBackupOverview(namespace: $namespace, clusterName: $cluster) { `+postgresBackupFields+` } }`,
		"postgresBackupOverview", cluster, nil)
}

// EnablePostgresBackups turns backups on for a cluster. retention is how
// long backups are kept, for example "30d".
func (c *Client) EnablePostgresBackups(ctx context.Context, cluster, retention string) (*PostgresBackups, error) {
	return c.postgresBackupCall(ctx, `mutation($namespace: String!, $cluster: String!, $retention: String!) { enablePostgresBackups(namespace: $namespace, clusterName: $cluster, retention: $retention) { `+postgresBackupFields+` } }`,
		"enablePostgresBackups", cluster, map[string]any{"retention": retention})
}

// SetPostgresBackupSchedule sets when a cluster is backed up. schedule is a
// cron expression; an empty retention keeps the current one.
func (c *Client) SetPostgresBackupSchedule(ctx context.Context, cluster, schedule, retention string, suspended bool) (*PostgresBackups, error) {
	input := map[string]any{"schedule": schedule, "suspended": suspended, "retention": optional(retention)}
	return c.postgresBackupCall(ctx, `mutation($namespace: String!, $cluster: String!, $input: PostgresBackupScheduleInput!) { setPostgresBackupSchedule(namespace: $namespace, clusterName: $cluster, input: $input) { `+postgresBackupFields+` } }`,
		"setPostgresBackupSchedule", cluster, map[string]any{"input": input})
}

// RemovePostgresBackupSchedule stops the scheduled backups of a cluster.
func (c *Client) RemovePostgresBackupSchedule(ctx context.Context, cluster string) error {
	if c.cfg.tenant == "" {
		return errNoTenant
	}
	return c.Do(ctx, `mutation($namespace: String!, $cluster: String!) { removePostgresBackupSchedule(namespace: $namespace, clusterName: $cluster) }`,
		map[string]any{"namespace": c.cfg.tenant, "cluster": cluster}, nil)
}

// BackupPostgresCluster starts a backup now.
func (c *Client) BackupPostgresCluster(ctx context.Context, cluster string) (*PostgresBackup, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Backup PostgresBackup `json:"backupPostgresCluster"`
	}
	err := c.Do(ctx, `mutation($namespace: String!, $cluster: String!) { backupPostgresCluster(namespace: $namespace, clusterName: $cluster) { name phase scheduled startedAt stoppedAt error } }`,
		map[string]any{"namespace": c.cfg.tenant, "cluster": cluster}, &out)
	return &out.Backup, err
}

// RestorePostgresInput describes a new cluster restored from another one's
// backups. Give Backup for one backup, TargetTime (RFC 3339) for a point in
// time, or neither for the latest state.
type RestorePostgresInput struct {
	Name          string  `json:"name"`
	SourceCluster string  `json:"sourceCluster"`
	Backup        *string `json:"backupName,omitempty"`
	TargetTime    *string `json:"targetTime,omitempty"`
	Instances     *int    `json:"instances,omitempty"`
}

// RestorePostgresCluster creates a new cluster from the backups of another.
func (c *Client) RestorePostgresCluster(ctx context.Context, in RestorePostgresInput) (*PostgresCluster, error) {
	if c.cfg.tenant == "" {
		return nil, errNoTenant
	}
	var out struct {
		Cluster PostgresCluster `json:"restorePostgresCluster"`
	}
	err := c.Do(ctx, `mutation($namespace: String!, $input: RestorePostgresClusterInput!) { restorePostgresCluster(namespace: $namespace, input: $input) { `+postgresFields+` } }`,
		map[string]any{"namespace": c.cfg.tenant, "input": in}, &out)
	return &out.Cluster, err
}
