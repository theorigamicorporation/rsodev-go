package platform

import (
	"context"
	"time"
)

// ServiceAccount is a non-human identity of the tenant, for automation.
type ServiceAccount struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Active    bool    `json:"isActive"`
	ExpiresAt *string `json:"expiresAt"`
}

// ServiceAccountSecret is a service account with its secret, which the
// platform shows once.
type ServiceAccountSecret struct {
	ServiceAccount ServiceAccount `json:"serviceAccount"`
	Secret         string         `json:"token"`
}

const serviceAccountFields = `id name status isActive expiresAt`

// ListServiceAccounts lists the tenant's service accounts.
func (c *Client) ListServiceAccounts(ctx context.Context) ([]ServiceAccount, error) {
	var out struct {
		Accounts []ServiceAccount `json:"serviceAccounts"`
	}
	err := c.Do(ctx, `query { serviceAccounts { `+serviceAccountFields+` } }`, nil, &out)
	return out.Accounts, err
}

// CreateServiceAccount creates a service account that expires at the given
// time. The secret is in the answer and cannot be read again.
func (c *Client) CreateServiceAccount(ctx context.Context, name string, expiresAt time.Time) (*ServiceAccountSecret, error) {
	var out struct {
		Created ServiceAccountSecret `json:"createServiceAccount"`
	}
	err := c.Do(ctx, `mutation($input: CreateServiceAccountInput!) { createServiceAccount(input: $input) { token serviceAccount { `+serviceAccountFields+` } } }`,
		map[string]any{"input": map[string]any{"name": name, "expiresAt": expiresAt.UTC().Format(time.RFC3339)}}, &out)
	return &out.Created, err
}

// RotateServiceAccountSecret replaces the secret of a service account; the
// old one stops working.
func (c *Client) RotateServiceAccountSecret(ctx context.Context, id string) (*ServiceAccountSecret, error) {
	var out struct {
		Rotated ServiceAccountSecret `json:"rotateServiceAccountToken"`
	}
	err := c.Do(ctx, `mutation($id: ID!) { rotateServiceAccountToken(id: $id) { token serviceAccount { `+serviceAccountFields+` } } }`, map[string]any{"id": id}, &out)
	return &out.Rotated, err
}

// SetServiceAccountActive switches a service account on or off.
func (c *Client) SetServiceAccountActive(ctx context.Context, id string, active bool) (*ServiceAccount, error) {
	var out struct {
		Account ServiceAccount `json:"setServiceAccountActive"`
	}
	err := c.Do(ctx, `mutation($input: SetServiceAccountActiveInput!) { setServiceAccountActive(input: $input) { `+serviceAccountFields+` } }`,
		map[string]any{"input": map[string]any{"id": id, "isActive": active}}, &out)
	return &out.Account, err
}

// DeleteServiceAccount deletes a service account.
func (c *Client) DeleteServiceAccount(ctx context.Context, id string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteServiceAccount(id: $id) }`, map[string]any{"id": id}, nil)
}

// Selector picks the objects a scoped grant covers: by exact name, by name
// prefix, or by labels (all given labels must match).
type Selector struct {
	Names    []string   `json:"names,omitempty"`
	Prefixes []string   `json:"prefixes,omitempty"`
	Labels   []KeyValue `json:"labels,omitempty"`
}

// ScopedAccess grants roles and permissions on some objects of one type,
// for example on the queues whose names start with "orders-".
type ScopedAccess struct {
	// ResourceType is a name from ListResourceTypes, such as "queue".
	ResourceType string   `json:"resourceType"`
	Selector     Selector `json:"selector"`
	Roles        []string `json:"roles,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
}

// Principal is a person or a service account with access to the tenant.
type Principal struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	// Type is "USER", "SERVICE_ACCOUNT" or "EXTERNAL_SERVICE_ACCOUNT".
	Type string `json:"type"`
	// Restricted principals hold only what they are granted here, without
	// the access their tenant role would give them.
	Restricted  bool           `json:"restricted"`
	Roles       []string       `json:"roles"`
	Permissions []string       `json:"permissions"`
	Scoped      []ScopedAccess `json:"scoped"`
	// EffectivePermissions is everything the principal may do tenant-wide.
	EffectivePermissions []string `json:"effectivePermissions"`
}

const principalFields = `id username name email type restricted roles permissions effectivePermissions scoped { resourceType roles permissions selector { names prefixes labels { key value } } }`

// ListPrincipals lists who has access to the tenant. A non-empty permission
// narrows the list to those who hold it.
func (c *Client) ListPrincipals(ctx context.Context, permission string) ([]Principal, error) {
	var out struct {
		Principals []Principal `json:"iamPrincipals"`
	}
	err := c.Do(ctx, `query($permission: String) { iamPrincipals(permission: $permission) { `+principalFields+` } }`, map[string]any{"permission": optional(permission)}, &out)
	return out.Principals, err
}

// FindPrincipal returns the principal with the given username, or nil.
func (c *Client) FindPrincipal(ctx context.Context, username string) (*Principal, error) {
	principals, err := c.ListPrincipals(ctx, "")
	if err != nil {
		return nil, err
	}
	for i := range principals {
		if principals[i].Username == username {
			return &principals[i], nil
		}
	}
	return nil, nil
}

// AccessInput replaces what a principal is granted. A nil field is left as
// it is; an empty list clears it.
type AccessInput struct {
	Username    string          `json:"username"`
	Roles       *[]string       `json:"roles,omitempty"`
	Permissions *[]string       `json:"permissions,omitempty"`
	Scoped      *[]ScopedAccess `json:"scoped,omitempty"`
	Restricted  *bool           `json:"restricted,omitempty"`
}

// SetPrincipalAccess sets the roles, permissions and scoped grants of a principal.
func (c *Client) SetPrincipalAccess(ctx context.Context, in AccessInput) (*Principal, error) {
	var out struct {
		Principal Principal `json:"setPrincipalAccess"`
	}
	err := c.Do(ctx, `mutation($input: SetPrincipalAccessInput!) { setPrincipalAccess(input: $input) { `+principalFields+` } }`, map[string]any{"input": in}, &out)
	return &out.Principal, err
}

// GrantPermissions adds tenant-wide permissions to a principal.
func (c *Client) GrantPermissions(ctx context.Context, username string, permissions []string) (*Principal, error) {
	var out struct {
		Principal Principal `json:"grantPermissions"`
	}
	err := c.Do(ctx, `mutation($username: String!, $permissions: [String!]!) { grantPermissions(username: $username, permissions: $permissions) { `+principalFields+` } }`,
		map[string]any{"username": username, "permissions": permissions}, &out)
	return &out.Principal, err
}

// RevokePermissions takes tenant-wide permissions away from a principal.
func (c *Client) RevokePermissions(ctx context.Context, username string, permissions []string) (*Principal, error) {
	var out struct {
		Principal Principal `json:"revokePermissions"`
	}
	err := c.Do(ctx, `mutation($username: String!, $permissions: [String!]!) { revokePermissions(username: $username, permissions: $permissions) { `+principalFields+` } }`,
		map[string]any{"username": username, "permissions": permissions}, &out)
	return &out.Principal, err
}

// ListResourceTypes lists the object types scoped access can be granted on.
func (c *Client) ListResourceTypes(ctx context.Context) ([]string, error) {
	var out struct {
		Types []struct {
			Name string `json:"name"`
		} `json:"resourceTypes"`
	}
	if err := c.Do(ctx, `query { resourceTypes { name } }`, nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Types))
	for _, t := range out.Types {
		names = append(names, t.Name)
	}
	return names, nil
}
