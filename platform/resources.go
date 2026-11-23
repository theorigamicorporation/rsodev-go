package platform

import (
	"context"
	"time"
)

// Registry is a private container registry of the tenant.
type Registry struct {
	Name string `json:"name"`
	// Domain is the host images are pushed to and pulled from.
	Domain    string  `json:"domain"`
	Status    string  `json:"status"`
	CreatedAt *string `json:"createdAt"`
}

const registryFields = `name domain status createdAt`

// RegistriesPage returns one page of the tenant's registries.
func (c *Client) RegistriesPage(ctx context.Context, o ListOptions) (*Page[Registry], error) {
	var out struct {
		Page Page[Registry] `json:"dockerRegistries"`
	}
	err := c.Do(ctx, `query($list: List) { dockerRegistries(list: $list) { total items { `+registryFields+` } } }`, map[string]any{"list": o.variable()}, &out)
	return &out.Page, err
}

// ListRegistries lists all of the tenant's registries.
func (c *Client) ListRegistries(ctx context.Context) ([]Registry, error) {
	return All(ctx, 0, c.RegistriesPage)
}

// GetRegistry returns one registry.
func (c *Client) GetRegistry(ctx context.Context, name string) (*Registry, error) {
	var out struct {
		Registry *Registry `json:"dockerRegistry"`
	}
	if err := c.Do(ctx, `query($name: String!) { dockerRegistry(name: $name) { `+registryFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	if out.Registry == nil {
		return nil, notFound("registry", name)
	}
	return out.Registry, nil
}

// CreateRegistry creates a registry.
func (c *Client) CreateRegistry(ctx context.Context, name string) (*Registry, error) {
	var out struct {
		Registry Registry `json:"createDockerRegistry"`
	}
	err := c.Do(ctx, `mutation($input: CreateDockerRegistryInput!) { createDockerRegistry(input: $input) { `+registryFields+` } }`,
		map[string]any{"input": map[string]any{"name": name}}, &out)
	return &out.Registry, err
}

// DeleteRegistry deletes a registry and its images.
func (c *Client) DeleteRegistry(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteDockerRegistry(name: $name) }`, map[string]any{"name": name}, nil)
}

// RegistryCredentials is a short-lived sign-in for a registry.
type RegistryCredentials struct {
	Registry  string `json:"registry"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	ExpiresAt string `json:"expiresAt"`
	// PullSecretName is the secret apps can name to pull from the registry.
	PullSecretName *string `json:"pullSecretName"`
}

// IssueRegistryCredentials issues a short-lived sign-in for a registry.
// push asks for write access; ttl zero takes the platform's default.
func (c *Client) IssueRegistryCredentials(ctx context.Context, name string, push bool, ttl time.Duration) (*RegistryCredentials, error) {
	input := map[string]any{"name": name, "access": "PULL"}
	if push {
		input["access"] = "PUSH"
	}
	if ttl > 0 {
		input["ttlSeconds"] = int(ttl.Seconds())
	}
	var out struct {
		Credentials RegistryCredentials `json:"issueDockerRegistryCredentials"`
	}
	err := c.Do(ctx, `mutation($input: IssueDockerRegistryCredentialsInput!) { issueDockerRegistryCredentials(input: $input) { registry username password expiresAt pullSecretName } }`,
		map[string]any{"input": input}, &out)
	return &out.Credentials, err
}

// Secret is a set of key/value pairs apps can read as environment variables
// or files. Values are only sent, never returned.
type Secret struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Keys      []string `json:"-"`
	CreatedAt *string  `json:"creationTimestamp"`
}

type secretWire struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	CreatedAt *string `json:"creationTimestamp"`
	Data      []struct {
		Key string `json:"key"`
	} `json:"data"`
}

func (w secretWire) secret() Secret {
	s := Secret{Name: w.Name, Type: w.Type, CreatedAt: w.CreatedAt}
	for _, entry := range w.Data {
		s.Keys = append(s.Keys, entry.Key)
	}
	return s
}

const secretFields = `name type creationTimestamp data { key }` //nolint:gosec // a field list, not a credential

func secretData(values map[string]string) []map[string]any {
	data := make([]map[string]any, 0, len(values))
	for key, value := range values {
		data = append(data, map[string]any{"key": key, "value": value})
	}
	return data
}

// ListSecrets lists the tenant's secrets.
func (c *Client) ListSecrets(ctx context.Context) ([]Secret, error) {
	var out struct {
		Secrets []secretWire `json:"secrets"`
	}
	if err := c.Do(ctx, `query { secrets { `+secretFields+` } }`, nil, &out); err != nil {
		return nil, err
	}
	secrets := make([]Secret, 0, len(out.Secrets))
	for _, s := range out.Secrets {
		secrets = append(secrets, s.secret())
	}
	return secrets, nil
}

// GetSecret returns one secret, without its values.
func (c *Client) GetSecret(ctx context.Context, name string) (*Secret, error) {
	var out struct {
		Secret *secretWire `json:"secret"`
	}
	if err := c.Do(ctx, `query($name: String!) { secret(name: $name) { `+secretFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	if out.Secret == nil {
		return nil, notFound("secret", name)
	}
	secret := out.Secret.secret()
	return &secret, nil
}

// CreateSecret creates a secret from key/value pairs.
func (c *Client) CreateSecret(ctx context.Context, name string, values map[string]string) (*Secret, error) {
	var out struct {
		Secret *secretWire `json:"createSecret"`
	}
	err := c.Do(ctx, `mutation($input: CreateSecretInput!) { createSecret(input: $input) { `+secretFields+` } }`,
		map[string]any{"input": map[string]any{"name": name, "data": secretData(values)}}, &out)
	if err != nil || out.Secret == nil {
		return nil, err
	}
	secret := out.Secret.secret()
	return &secret, nil
}

// UpdateSecret replaces the key/value pairs of a secret.
func (c *Client) UpdateSecret(ctx context.Context, name string, values map[string]string) (*Secret, error) {
	var out struct {
		Secret secretWire `json:"updateSecret"`
	}
	err := c.Do(ctx, `mutation($input: UpdateSecretInput!) { updateSecret(input: $input) { `+secretFields+` } }`,
		map[string]any{"input": map[string]any{"name": name, "data": secretData(values)}}, &out)
	if err != nil {
		return nil, err
	}
	secret := out.Secret.secret()
	return &secret, nil
}

// DeleteSecret deletes a secret.
func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteSecret(name: $name) }`, map[string]any{"name": name}, nil)
}

// SSHKey is a public key in the tenant's key store, installed on new
// instances that ask for it.
type SSHKey struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PublicKey   string  `json:"publicKey"`
	Fingerprint string  `json:"fingerprint"`
	KeyType     string  `json:"keyType"`
	Comment     *string `json:"comment"`
	// Scope is "TENANT" for a key of the tenant and "PERSONAL" for one of
	// the caller's own.
	Scope     string `json:"scope"`
	CreatedAt string `json:"createdAt"`
	CreatedBy string `json:"createdBy"`
}

const sshKeyFields = `id name publicKey fingerprint keyType comment scope createdAt createdBy`

// ListSSHKeys lists the keys the caller can use: the tenant's and its own.
func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKey, error) {
	var out struct {
		Keys []SSHKey `json:"sshKeys"`
	}
	err := c.Do(ctx, `query { sshKeys { `+sshKeyFields+` } }`, nil, &out)
	return out.Keys, err
}

// GetSSHKey returns one key.
func (c *Client) GetSSHKey(ctx context.Context, id string) (*SSHKey, error) {
	var out struct {
		Key *SSHKey `json:"sshKey"`
	}
	if err := c.Do(ctx, `query($id: ID!) { sshKey(id: $id) { `+sshKeyFields+` } }`, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	if out.Key == nil {
		return nil, notFound("SSH key", id)
	}
	return out.Key, nil
}

// AddSSHKey adds a public key to the tenant's key store.
func (c *Client) AddSSHKey(ctx context.Context, name, publicKey string) (*SSHKey, error) {
	var out struct {
		Key SSHKey `json:"addSshKey"`
	}
	err := c.Do(ctx, `mutation($input: AddSshKeyInput!) { addSshKey(input: $input) { `+sshKeyFields+` } }`,
		map[string]any{"input": map[string]any{"name": name, "publicKey": publicKey}}, &out)
	return &out.Key, err
}

// RenameSSHKey renames a key of the tenant.
func (c *Client) RenameSSHKey(ctx context.Context, id, name string) (*SSHKey, error) {
	var out struct {
		Key SSHKey `json:"renameSshKey"`
	}
	err := c.Do(ctx, `mutation($id: ID!, $name: String!) { renameSshKey(id: $id, name: $name) { `+sshKeyFields+` } }`, map[string]any{"id": id, "name": name}, &out)
	return &out.Key, err
}

// DeleteSSHKey removes a key from the tenant's key store. Instances that
// already have it keep it.
func (c *Client) DeleteSSHKey(ctx context.Context, id string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteSshKey(id: $id) }`, map[string]any{"id": id}, nil)
}

// Volume is a block volume of the tenant.
type Volume struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	SizeGB    float64  `json:"sizeGb"`
	Status    *string  `json:"status"`
	UsedBy    []string `json:"usedBy"`
	Deletable bool     `json:"deletable"`
	CreatedAt *string  `json:"createdAt"`
}

const volumeFields = `id name sizeGb status usedBy deletable createdAt`

// ListVolumes lists the tenant's volumes.
func (c *Client) ListVolumes(ctx context.Context) ([]Volume, error) {
	var out struct {
		Volumes struct {
			Items []Volume `json:"items"`
		} `json:"volumes"`
	}
	err := c.Do(ctx, `query { volumes { items { `+volumeFields+` } } }`, nil, &out)
	return out.Volumes.Items, err
}

// CreateVolume creates a volume apps can mount.
func (c *Client) CreateVolume(ctx context.Context, name string, sizeGB int) (*Volume, error) {
	var out struct {
		Volume Volume `json:"createVolume"`
	}
	err := c.Do(ctx, `mutation($input: CreateVolumeInput!) { createVolume(input: $input) { `+volumeFields+` } }`,
		map[string]any{"input": map[string]any{"name": name, "sizeGb": sizeGB}}, &out)
	return &out.Volume, err
}

// DeleteVolume deletes a volume and its data.
func (c *Client) DeleteVolume(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteVolume(name: $name) }`, map[string]any{"name": name}, nil)
}

// VPC is a private network of the tenant.
type VPC struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	CIDR string `json:"cidr"`
}

// CreateVPC creates a private network with the given address range.
func (c *Client) CreateVPC(ctx context.Context, name, cidr string) (*VPC, error) {
	var out struct {
		VPC VPC `json:"createVpc"`
	}
	err := c.Do(ctx, `mutation($name: String!, $cidr: String!) { createVpc(name: $name, cidr: $cidr) { id name cidr } }`, map[string]any{"name": name, "cidr": cidr}, &out)
	return &out.VPC, err
}
