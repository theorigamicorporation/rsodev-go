package platform

import (
	"context"
	"sort"
)

// Role is a named bundle of permissions. Built-in roles cannot be changed.
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	BuiltIn     bool     `json:"builtIn"`
	Permissions []string `json:"permissions"`
}

const roleFields = `name description builtIn permissions`

func roleVariables(r Role) map[string]any {
	input := map[string]any{"name": r.Name, "permissions": orEmpty(r.Permissions)}
	if r.Description != "" {
		input["description"] = r.Description
	}
	return map[string]any{"input": input}
}

// ListRoles returns the tenant's roles, built-in ones included.
func (c *Client) ListRoles(ctx context.Context) ([]Role, error) {
	var out struct {
		Roles []Role `json:"iamRoles"`
	}
	err := c.Do(ctx, `query { iamRoles { `+roleFields+` } }`, nil, &out)
	return out.Roles, err
}

// FindRole looks a role up by name: nil without an error when there is none.
func (c *Client) FindRole(ctx context.Context, name string) (*Role, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].Name == name {
			return &roles[i], nil
		}
	}
	return nil, nil
}

// CreateRole creates a custom role.
func (c *Client) CreateRole(ctx context.Context, r Role) error {
	return c.Do(ctx, `mutation($input: IamRoleInput!) { createIamRole(input: $input) { name } }`, roleVariables(r), nil)
}

// UpdateRole replaces a custom role's description and permissions.
func (c *Client) UpdateRole(ctx context.Context, r Role) error {
	return c.Do(ctx, `mutation($input: IamRoleInput!) { updateIamRole(input: $input) { name } }`, roleVariables(r), nil)
}

// DeleteRole deletes a custom role.
func (c *Client) DeleteRole(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!) { deleteIamRole(name: $name) }`, map[string]any{"name": name}, nil)
}

// Resource types that carry labels and that access can be scoped to.
const (
	ResourceBucket            = "bucket"
	ResourceQueue             = "queue"
	ResourceTopic             = "topic"
	ResourceApp               = "app"
	ResourcePostgresCluster   = "postgresCluster"
	ResourceScheduledJob      = "scheduledJob"
	ResourceRegistry          = "registry"
	ResourceSecret            = "secret"
	ResourceVolume            = "volume"
	ResourceKubernetesCluster = "kubernetesCluster"
	ResourceAIModel           = "aiModel"
)

// GetResourceLabels returns the labels of one object; an object without
// labels has an empty map.
func (c *Client) GetResourceLabels(ctx context.Context, resourceType, name string) (map[string]string, error) {
	var out struct {
		Items []struct {
			Name   string `json:"name"`
			Labels []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"labels"`
		} `json:"resourceLabels"`
	}
	err := c.Do(ctx, `query($type: String!, $name: String) { resourceLabels(resourceType: $type, name: $name) { name labels { key value } } }`,
		map[string]any{"type": resourceType, "name": name}, &out)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{}
	for _, item := range out.Items {
		if item.Name == name {
			for _, l := range item.Labels {
				labels[l.Key] = l.Value
			}
		}
	}
	return labels, nil
}

// SetResourceLabels replaces the labels of one object; an empty map removes
// them all. A label can bring an object into the scope of someone's access,
// so the platform treats this as an access-management action.
func (c *Client) SetResourceLabels(ctx context.Context, resourceType, name string, labels map[string]string) error {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		items = append(items, map[string]any{"key": k, "value": labels[k]})
	}
	return c.Do(ctx, `mutation($type: String!, $name: String!, $labels: [ResourceLabelInput!]!) { setResourceLabels(resourceType: $type, name: $name, labels: $labels) { name } }`,
		map[string]any{"type": resourceType, "name": name, "labels": items}, nil)
}
