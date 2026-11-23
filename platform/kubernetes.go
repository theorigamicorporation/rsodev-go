package platform

import "context"

// ClusterKubeconfig returns a kubeconfig for one of the tenant's Kubernetes
// clusters. It holds no secret: it tells kubectl to run a sign-in helper that
// calls ClusterToken.
func (c *Client) ClusterKubeconfig(ctx context.Context, name string) (string, error) {
	var out struct {
		Kubeconfig string `json:"tenantClusterKubeconfig"`
	}
	err := c.Do(ctx, `mutation($name: String!) { tenantClusterKubeconfig(name: $name, kind: EXEC) }`, map[string]any{"name": name}, &out)
	return out.Kubeconfig, err
}

// ClusterToken returns a short-lived credential for one of the tenant's
// Kubernetes clusters, as the ExecCredential JSON kubectl expects from a
// sign-in helper.
func (c *Client) ClusterToken(ctx context.Context, name string, ttlSeconds int) (string, error) {
	var out struct {
		Token struct {
			ExecCredential string `json:"execCredential"`
		} `json:"tenantClusterToken"`
	}
	err := c.Do(ctx, `mutation($name: String!, $ttl: Int!) { tenantClusterToken(name: $name, ttlSeconds: $ttl) { execCredential } }`,
		map[string]any{"name": name, "ttl": ttlSeconds}, &out)
	return out.Token.ExecCredential, err
}

// NodeGroup is a group of identical nodes of a Kubernetes cluster.
type NodeGroup struct {
	Name          string `json:"name,omitempty"`
	Replicas      int    `json:"replicas"`
	ReadyReplicas int    `json:"readyReplicas"`
	VCPU          int    `json:"vcpu"`
	RAMGB         int    `json:"ramGb"`
	DiskGB        int    `json:"diskGb"`
}

// KubernetesCluster is a managed Kubernetes cluster of the tenant.
type KubernetesCluster struct {
	Name string `json:"name"`
	// Status is READY, PROVISIONING, SCALING, UPGRADING, DELETING or FAILED.
	Status        string      `json:"status"`
	StatusReason  *string     `json:"statusReason"`
	Version       string      `json:"version"`
	TargetVersion *string     `json:"targetVersion"`
	Endpoint      *string     `json:"endpoint"`
	ControlPlane  NodeGroup   `json:"controlPlane"`
	Pools         []NodeGroup `json:"pools"`
	CreatedAt     *string     `json:"createdAt"`
}

const kubernetesClusterFields = `name status statusReason version targetVersion endpoint createdAt controlPlane { replicas readyReplicas vcpu ramGb diskGb } pools { name replicas readyReplicas vcpu ramGb diskGb }`

// ListKubernetesClusters lists the tenant's Kubernetes clusters.
func (c *Client) ListKubernetesClusters(ctx context.Context) ([]KubernetesCluster, error) {
	var out struct {
		Clusters struct {
			Items []KubernetesCluster `json:"items"`
		} `json:"tenantClusters"`
	}
	err := c.Do(ctx, `query { tenantClusters { items { `+kubernetesClusterFields+` } } }`, nil, &out)
	return out.Clusters.Items, err
}

// GetKubernetesCluster returns one Kubernetes cluster.
func (c *Client) GetKubernetesCluster(ctx context.Context, name string) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"tenantCluster"`
	}
	if err := c.Do(ctx, `query($name: String!) { tenantCluster(name: $name) { `+kubernetesClusterFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Cluster, nil
}

// NodePool describes a pool of worker nodes to create.
type NodePool struct {
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
	VCPU     int    `json:"vcpu"`
	RAMGB    int    `json:"ramGb"`
	DiskGB   int    `json:"diskGb"`
}

// KubernetesClusterInput describes a Kubernetes cluster to create. An empty
// Version takes the platform's default.
type KubernetesClusterInput struct {
	Name    string     `json:"name"`
	Version string     `json:"version,omitempty"`
	Pools   []NodePool `json:"pools"`
}

// CreateKubernetesCluster starts creating a Kubernetes cluster.
// WaitKubernetesClusterReady waits until it is usable.
func (c *Client) CreateKubernetesCluster(ctx context.Context, in KubernetesClusterInput) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"createTenantCluster"`
	}
	err := c.Do(ctx, `mutation($input: CreateTenantClusterInput!) { createTenantCluster(input: $input) { `+kubernetesClusterFields+` } }`, map[string]any{"input": in}, &out)
	return &out.Cluster, err
}

// DeleteKubernetesCluster deletes a Kubernetes cluster and everything on it.
func (c *Client) DeleteKubernetesCluster(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteTenantCluster(name: $name, confirm: $name) }`, map[string]any{"name": name}, nil)
}

// ScaleNodePool sets the number of nodes of a pool.
func (c *Client) ScaleNodePool(ctx context.Context, cluster, pool string, replicas int) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"scaleTenantClusterPool"`
	}
	err := c.Do(ctx, `mutation($cluster: String!, $pool: String!, $replicas: Int!) { scaleTenantClusterPool(cluster: $cluster, pool: $pool, replicas: $replicas) { `+kubernetesClusterFields+` } }`,
		map[string]any{"cluster": cluster, "pool": pool, "replicas": replicas}, &out)
	return &out.Cluster, err
}

// AddNodePool adds a pool of worker nodes to a cluster.
func (c *Client) AddNodePool(ctx context.Context, cluster string, pool NodePool) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"addTenantClusterPool"`
	}
	err := c.Do(ctx, `mutation($cluster: String!, $pool: TenantClusterPoolInput!) { addTenantClusterPool(cluster: $cluster, pool: $pool) { `+kubernetesClusterFields+` } }`,
		map[string]any{"cluster": cluster, "pool": pool}, &out)
	return &out.Cluster, err
}

// RemoveNodePool removes a pool and its nodes from a cluster.
func (c *Client) RemoveNodePool(ctx context.Context, cluster, pool string) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"removeTenantClusterPool"`
	}
	err := c.Do(ctx, `mutation($cluster: String!, $pool: String!) { removeTenantClusterPool(cluster: $cluster, pool: $pool) { `+kubernetesClusterFields+` } }`,
		map[string]any{"cluster": cluster, "pool": pool}, &out)
	return &out.Cluster, err
}

// UpgradeKubernetesCluster moves a cluster to another Kubernetes version.
func (c *Client) UpgradeKubernetesCluster(ctx context.Context, cluster, version string) (*KubernetesCluster, error) {
	var out struct {
		Cluster KubernetesCluster `json:"upgradeTenantCluster"`
	}
	err := c.Do(ctx, `mutation($cluster: String!, $version: String!) { upgradeTenantCluster(cluster: $cluster, version: $version) { `+kubernetesClusterFields+` } }`,
		map[string]any{"cluster": cluster, "version": version}, &out)
	return &out.Cluster, err
}

// WaitKubernetesClusterReady polls until the cluster is READY. The default
// timeout of two minutes is too short for a new cluster; pass a longer one.
func (c *Client) WaitKubernetesClusterReady(ctx context.Context, name string, o WaitOptions) (*KubernetesCluster, error) {
	var cluster *KubernetesCluster
	err := waitReady(ctx, "Kubernetes cluster "+name, o, func(ctx context.Context) (Status, string, error) {
		var err error
		if cluster, err = c.GetKubernetesCluster(ctx, name); err != nil {
			return "", "", err
		}
		reason := ""
		if cluster.StatusReason != nil {
			reason = *cluster.StatusReason
		}
		switch cluster.Status {
		case "READY":
			return StatusReady, "", nil
		case "FAILED":
			return StatusFailed, reason, nil
		}
		return StatusPending, reason, nil
	})
	return cluster, err
}

// Kinds of kubeconfig.
const (
	// KubeconfigExec holds no secret: kubectl runs a sign-in helper for a
	// fresh credential whenever it needs one.
	KubeconfigExec = "EXEC"
	// KubeconfigSession carries a token that expires within the hour.
	KubeconfigSession = "SESSION"
)

// Kubeconfig returns a kubeconfig of the given kind for a cluster.
func (c *Client) Kubeconfig(ctx context.Context, name, kind string) (string, error) {
	var out struct {
		Kubeconfig string `json:"tenantClusterKubeconfig"`
	}
	err := c.Do(ctx, `mutation($name: String!, $kind: TenantClusterKubeconfigKind!) { tenantClusterKubeconfig(name: $name, kind: $kind) }`,
		map[string]any{"name": name, "kind": kind}, &out)
	return out.Kubeconfig, err
}
