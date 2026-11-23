package platform

import "context"

// Types of DNS records a tenant can manage.
const (
	DNSRecordA     = "A"
	DNSRecordAAAA  = "AAAA"
	DNSRecordCNAME = "CNAME"
	DNSRecordTXT   = "TXT"
)

// DNSZone is a zone the tenant may create records in.
type DNSZone struct {
	Name string `json:"name"`
}

// DNSRecord is a DNS record of the tenant.
type DNSRecord struct {
	ID   string `json:"id"`
	Zone string `json:"zone"`
	// Name is the full name of the record, inside the zone.
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Content   string  `json:"content"`
	CreatedAt string  `json:"createdAt"`
	CreatedBy *string `json:"createdBy"`
	UpdatedAt string  `json:"updatedAt"`
}

const dnsRecordFields = `id zone name type content createdAt createdBy updatedAt`

// DNSAvailable reports whether the tenant has any zone to manage records in.
func (c *Client) DNSAvailable(ctx context.Context) (bool, error) {
	var out struct {
		Available bool `json:"dnsAvailable"`
	}
	err := c.Do(ctx, `query { dnsAvailable }`, nil, &out)
	return out.Available, err
}

// ListDNSZones lists the zones the tenant may create records in.
func (c *Client) ListDNSZones(ctx context.Context) ([]DNSZone, error) {
	var out struct {
		Zones []DNSZone `json:"dnsZones"`
	}
	err := c.Do(ctx, `query { dnsZones { name } }`, nil, &out)
	return out.Zones, err
}

// ListDNSRecords lists the tenant's records; an empty zone means all zones.
func (c *Client) ListDNSRecords(ctx context.Context, zone string) ([]DNSRecord, error) {
	var out struct {
		Records []DNSRecord `json:"dnsRecords"`
	}
	err := c.Do(ctx, `query($zone: String) { dnsRecords(zone: $zone) { `+dnsRecordFields+` } }`, map[string]any{"zone": optional(zone)}, &out)
	return out.Records, err
}

// GetDNSRecord returns one record by id.
func (c *Client) GetDNSRecord(ctx context.Context, id string) (*DNSRecord, error) {
	var out struct {
		Record DNSRecord `json:"dnsRecord"`
	}
	if err := c.Do(ctx, `query($id: ID!) { dnsRecord(id: $id) { `+dnsRecordFields+` } }`, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return &out.Record, nil
}

// CreateDNSRecord creates a record. name is the full name inside the zone.
func (c *Client) CreateDNSRecord(ctx context.Context, zone, name, recordType, content string) (*DNSRecord, error) {
	var out struct {
		Record DNSRecord `json:"createDnsRecord"`
	}
	err := c.Do(ctx, `mutation($input: CreateDnsRecordInput!) { createDnsRecord(input: $input) { `+dnsRecordFields+` } }`,
		map[string]any{"input": map[string]any{"zone": zone, "name": name, "type": recordType, "content": content}}, &out)
	return &out.Record, err
}

// UpdateDNSRecord changes what a record points at.
func (c *Client) UpdateDNSRecord(ctx context.Context, id, content string) (*DNSRecord, error) {
	var out struct {
		Record DNSRecord `json:"updateDnsRecord"`
	}
	err := c.Do(ctx, `mutation($id: ID!, $input: UpdateDnsRecordInput!) { updateDnsRecord(id: $id, input: $input) { `+dnsRecordFields+` } }`,
		map[string]any{"id": id, "input": map[string]any{"content": content}}, &out)
	return &out.Record, err
}

// DeleteDNSRecord deletes a record.
func (c *Client) DeleteDNSRecord(ctx context.Context, id string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteDnsRecord(id: $id) }`, map[string]any{"id": id}, nil)
}
