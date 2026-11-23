package platform

import "context"

// TenantMembership is a tenant the caller belongs to.
type TenantMembership struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Region string   `json:"region"`
	Roles  []string `json:"myRoles"`
}

// Account is the signed-in person or service account.
type Account struct {
	ID            string             `json:"id"`
	Username      string             `json:"username"`
	Name          string             `json:"name"`
	Email         string             `json:"email"`
	PlatformRoles []string           `json:"platformRoles"`
	Tenants       []TenantMembership `json:"tenants"`
}

// Me returns the caller and the tenants it belongs to. It works on a client
// without a tenant.
func (c *Client) Me(ctx context.Context) (*Account, error) {
	var out struct {
		Me *Account `json:"myUser"`
	}
	err := c.Do(ctx, `query { myUser { id username name email platformRoles tenants { id name region myRoles } } }`, nil, &out)
	if err != nil {
		return nil, err
	}
	if out.Me == nil {
		return nil, ErrNotSignedIn
	}
	return out.Me, nil
}

// Permissions returns what the caller may do in the tenant, as permission
// names such as "apps.read".
func (c *Client) Permissions(ctx context.Context) ([]string, error) {
	var out struct {
		Permissions []string `json:"myPermissions"`
	}
	err := c.Do(ctx, `query { myPermissions }`, nil, &out)
	return out.Permissions, err
}

// QuotaUsage is how much of one resource the tenant holds against its limit.
type QuotaUsage struct {
	Name  string  `json:"name"`
	Used  float64 `json:"used"`
	Limit float64 `json:"limit"`
}

// Quota returns the tenant's usage against its quotas.
func (c *Client) Quota(ctx context.Context) ([]QuotaUsage, error) {
	var out struct {
		Quota []QuotaUsage `json:"quota"`
	}
	err := c.Do(ctx, `query { quota { name used limit } }`, nil, &out)
	return out.Quota, err
}

// EventType is a kind of platform event a tenant can subscribe to.
type EventType struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// EventsInfo names the topic platform events are published to and the
// event types there are.
type EventsInfo struct {
	Available bool        `json:"available"`
	Topic     string      `json:"topic"`
	Types     []EventType `json:"types"`
}

// PlatformEvents returns the events topic and the event types.
func (c *Client) PlatformEvents(ctx context.Context) (*EventsInfo, error) {
	var out struct {
		Events EventsInfo `json:"platformEvents"`
	}
	err := c.Do(ctx, `query { platformEvents { available topic types { type description } } }`, nil, &out)
	return &out.Events, err
}
