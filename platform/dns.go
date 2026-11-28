package platform

import "context"

// Types of DNS records a tenant can manage.
const (
	DNSRecordA     = "A"
	DNSRecordAAAA  = "AAAA"
	DNSRecordCNAME = "CNAME"
	DNSRecordTXT   = "TXT"
	// DNSRecordMX is for zones of the tenant's own only. The content is a
	// priority and a mail host: "10 mail.example.com".
	DNSRecordMX = "MX"
)

// States of a DNS zone.
const (
	// DNSZonePending means the domain is not delegated to the platform's
	// name servers yet: nothing in the zone answers.
	DNSZonePending = "PENDING"
	DNSZoneActive  = "ACTIVE"
	// DNSZoneLost means the domain is no longer delegated to the platform.
	DNSZoneLost = "LOST"
)

// DNSZone is a zone the tenant may create records in: one of its own, or
// one the platform offers. The name identifies it.
type DNSZone struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// Nameservers are, for a zone of the tenant's own, the name servers to
	// set for the domain at its registrar.
	Nameservers []string `json:"nameservers"`
	// Delegated is true for a zone of the tenant's own.
	Delegated bool    `json:"delegated"`
	CreatedAt *string `json:"createdAt"`
}

const dnsZoneFields = `name status nameservers delegated createdAt`

// DNSRecord is a DNS record of the tenant.
type DNSRecord struct {
	ID   string `json:"id"`
	Zone string `json:"zone"`
	// Name is the full name of the record, inside the zone.
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	// TTL is nil where the platform's default applies.
	TTL       *int    `json:"ttl"`
	CreatedAt string  `json:"createdAt"`
	CreatedBy *string `json:"createdBy"`
	UpdatedAt string  `json:"updatedAt"`
}

const dnsRecordFields = `id zone name type content ttl createdAt createdBy updatedAt`

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
	err := c.Do(ctx, `query { dnsZones { `+dnsZoneFields+` } }`, nil, &out)
	return out.Zones, err
}

// GetDNSZone returns one zone by its domain name.
func (c *Client) GetDNSZone(ctx context.Context, name string) (*DNSZone, error) {
	var out struct {
		Zone DNSZone `json:"dnsZone"`
	}
	if err := c.Do(ctx, `query($name: String!) { dnsZone(name: $name) { `+dnsZoneFields+` } }`, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out.Zone, nil
}

// CreateDNSZone brings a domain of the tenant's own to the platform's DNS.
// The answer names the name servers to set at the domain's registrar; the
// zone is pending until that delegation is seen. Asking again for a zone
// the tenant has returns it.
func (c *Client) CreateDNSZone(ctx context.Context, domain string) (*DNSZone, error) {
	var out struct {
		Zone DNSZone `json:"createDnsZone"`
	}
	if err := c.Do(ctx, `mutation($domain: String!) { createDnsZone(domain: $domain) { `+dnsZoneFields+` } }`, map[string]any{"domain": domain}, &out); err != nil {
		return nil, err
	}
	return &out.Zone, nil
}

// DeleteDNSZone stops hosting a zone of the tenant's own: the domain no
// longer resolves through the platform. It is refused while the zone has
// records or published hosts. The zone's name is also sent as the
// confirmation the platform asks for.
func (c *Client) DeleteDNSZone(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($name: String!, $confirm: String!) { deleteDnsZone(name: $name, confirm: $confirm) }`, map[string]any{"name": name, "confirm": name}, nil)
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

// CreateDNSRecordInput describes a record to create.
type CreateDNSRecordInput struct {
	Zone string
	// Name is the full name of the record, inside the zone.
	Name    string
	Type    string
	Content string
	// TTL is the number of seconds a resolver may keep the answer, 60 to
	// 86400, for a record in a zone of the tenant's own. Zero leaves it to
	// the platform's default, and is the only value a zone the platform
	// offers accepts. The records of one name and type share one TTL. It
	// cannot be changed afterwards: delete the record and create another.
	TTL int
}

func (in CreateDNSRecordInput) variable() map[string]any {
	input := map[string]any{"zone": in.Zone, "name": in.Name, "type": in.Type, "content": in.Content}
	if in.TTL != 0 {
		input["ttl"] = in.TTL
	}
	return input
}

// CreateDNSRecord creates a record with the platform's default TTL. name is
// the full name inside the zone. CreateDNSRecordWith takes the optional
// settings too.
func (c *Client) CreateDNSRecord(ctx context.Context, zone, name, recordType, content string) (*DNSRecord, error) {
	return c.CreateDNSRecordWith(ctx, CreateDNSRecordInput{Zone: zone, Name: name, Type: recordType, Content: content})
}

// CreateDNSRecordWith creates a record from an input, which can carry a TTL.
func (c *Client) CreateDNSRecordWith(ctx context.Context, in CreateDNSRecordInput) (*DNSRecord, error) {
	var out struct {
		Record DNSRecord `json:"createDnsRecord"`
	}
	err := c.Do(ctx, `mutation($input: CreateDnsRecordInput!) { createDnsRecord(input: $input) { `+dnsRecordFields+` } }`,
		map[string]any{"input": in.variable()}, &out)
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
