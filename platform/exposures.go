package platform

import "context"

// How the platform talks to the target of a published host.
const (
	ExposureHTTP  = "HTTP"
	ExposureHTTPS = "HTTPS"
)

// Certificates of a published host.
const (
	// ExposureTLSAuto has the platform obtain and renew a certificate.
	ExposureTLSAuto = "AUTO"
	// ExposureTLSNone serves the host over plain HTTP.
	ExposureTLSNone = "NONE"
)

// States of a published host.
const (
	ExposurePending = "PENDING"
	ExposureActive  = "ACTIVE"
)

// ExposureTarget is where the traffic of a published host goes.
type ExposureTarget struct {
	// Address is an IPv4 address in the tenant's own private network.
	Address string `json:"address"`
	Port    int    `json:"port"`
	// Scheme is ExposureHTTP or ExposureHTTPS.
	Scheme string `json:"scheme"`
	// InsecureSkipVerify says whether the platform skips checking the
	// certificate an HTTPS target presents. A target is an address, which a
	// certificate rarely names, so a new host with an HTTPS target has to
	// say true (or false, to have it checked). Left nil when publishing, a
	// host that exists keeps its setting. Ignored for HTTP.
	InsecureSkipVerify *bool `json:"insecureSkipVerify"`
}

// ExposureReplacedRecord is a DNS record that was removed to publish a host
// (TakeOver).
type ExposureReplacedRecord struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	RemovedAt string `json:"removedAt"`
}

// Exposure is a hostname that is reachable from the internet and where its
// traffic goes. The host identifies it.
type Exposure struct {
	Host string `json:"host"`
	// Namespace holds the host: the tenant's id for a tenant.
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	TLS       string `json:"tls"`
	// DNSManaged is true when the platform keeps the host's DNS record.
	DNSManaged bool `json:"dnsManaged"`
	// DNSTarget is what to point the host at in a DNS of your own: a host
	// name for a CNAME record, or addresses for A records.
	DNSTarget []string `json:"dnsTarget"`
	// Domain is the domain of the tenant's own the host is under, if any.
	Domain         *string `json:"domain"`
	DomainVerified *bool   `json:"domainVerified"`
	// Target is nil for a host the platform published.
	Target *ExposureTarget `json:"target"`
	// TargetOutOfRange means the target is no longer in the tenant's
	// network: publish the host again with its new address.
	TargetOutOfRange bool `json:"targetOutOfRange"`
	// Proxied says whether the host's DNS record goes through the DNS
	// proxy, as last applied. Nil when the platform keeps no record for
	// the host.
	Proxied *bool `json:"proxied"`
	// ReplacedRecords are the DNS records take-overs removed at this name,
	// oldest first. Platform administrators only; empty for everyone else.
	ReplacedRecords   []ExposureReplacedRecord `json:"replacedRecords"`
	ManagedByPlatform bool                     `json:"managedByPlatform"`
	BasicAuthUser     *string                  `json:"basicAuthUser"`
	CreatedAt         string                   `json:"createdAt"`
	CreatedBy         *string                  `json:"createdBy"`
	UpdatedAt         string                   `json:"updatedAt"`
}

const exposureFields = `host namespace status tls dnsManaged dnsTarget domain domainVerified target { address port scheme insecureSkipVerify } targetOutOfRange proxied replacedRecords { type name content removedAt } managedByPlatform basicAuthUser createdAt createdBy updatedAt`

// ExposureBasicAuth makes visitors of a host sign in.
type ExposureBasicAuth struct {
	Username string
	// Password is 8 to 256 characters. It cannot be read back.
	Password string
}

// ExposeHostInput publishes a host or changes where it goes.
type ExposeHostInput struct {
	// Host is lower case, without a trailing dot.
	Host   string
	Target ExposureTarget
	// TLS is ExposureTLSAuto or ExposureTLSNone; empty is the platform's
	// default.
	TLS string
	// BasicAuth sets the sign-in. Left nil, the host keeps the one it has,
	// unless RemoveBasicAuth or Replace is set.
	BasicAuth       *ExposureBasicAuth
	RemoveBasicAuth bool
	// Replace makes the input the whole wanted state: whatever is left out
	// is removed. For controllers that always send everything.
	Replace bool
	// Namespace, Proxied and TakeOver are for platform administrators.
	// Namespace names whom the host is for; empty is the caller's tenant.
	Namespace string
	Proxied   *bool
	TakeOver  bool
}

func (in ExposeHostInput) variable() map[string]any {
	target := map[string]any{"address": in.Target.Address, "port": in.Target.Port, "scheme": in.Target.Scheme}
	if in.Target.InsecureSkipVerify != nil {
		target["insecureSkipVerify"] = *in.Target.InsecureSkipVerify
	}
	input := map[string]any{"host": in.Host, "target": target}
	if in.TLS != "" {
		input["tls"] = in.TLS
	}
	switch {
	case in.BasicAuth != nil:
		input["basicAuth"] = map[string]any{"username": in.BasicAuth.Username, "password": in.BasicAuth.Password}
	case in.RemoveBasicAuth:
		input["basicAuth"] = nil
	}
	if in.Replace {
		input["replace"] = true
	}
	if in.Proxied != nil {
		input["proxied"] = *in.Proxied
	}
	if in.TakeOver {
		input["takeOver"] = true
	}
	return input
}

// ExposuresAvailable reports whether publishing hosts is switched on.
func (c *Client) ExposuresAvailable(ctx context.Context) (bool, error) {
	var out struct {
		Available bool `json:"exposuresAvailable"`
	}
	err := c.Do(ctx, `query { exposuresAvailable }`, nil, &out)
	return out.Available, err
}

// ExposuresPage returns one page of the published hosts. namespace is for
// platform administrators: one namespace, or empty for every namespace.
// A tenant leaves it empty and gets its own hosts.
func (c *Client) ExposuresPage(ctx context.Context, namespace string, o ListOptions) (*Page[Exposure], error) {
	var out struct {
		Page Page[Exposure] `json:"exposures"`
	}
	err := c.Do(ctx, `query($namespace: String, $list: List) { exposures(namespace: $namespace, list: $list) { total items { `+exposureFields+` } } }`,
		map[string]any{"namespace": optional(namespace), "list": o.variable()}, &out)
	return &out.Page, err
}

// ListExposures lists all published hosts; see ExposuresPage for namespace.
func (c *Client) ListExposures(ctx context.Context, namespace string) ([]Exposure, error) {
	return All(ctx, 0, func(ctx context.Context, o ListOptions) (*Page[Exposure], error) {
		return c.ExposuresPage(ctx, namespace, o)
	})
}

// GetExposure returns one published host. namespace is for platform
// administrators; a tenant leaves it empty.
func (c *Client) GetExposure(ctx context.Context, host, namespace string) (*Exposure, error) {
	var out struct {
		Exposure Exposure `json:"exposure"`
	}
	err := c.Do(ctx, `query($host: String!, $namespace: String) { exposure(host: $host, namespace: $namespace) { `+exposureFields+` } }`,
		map[string]any{"host": host, "namespace": optional(namespace)}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Exposure, nil
}

// ExposeHost publishes a host or changes where it goes. Asking again with
// the same input changes nothing. A host held by someone else is refused
// with a conflict.
func (c *Client) ExposeHost(ctx context.Context, in ExposeHostInput) (*Exposure, error) {
	var out struct {
		Exposure Exposure `json:"exposeHost"`
	}
	err := c.Do(ctx, `mutation($input: ExposeHostInput!, $namespace: String) { exposeHost(input: $input, namespace: $namespace) { `+exposureFields+` } }`,
		map[string]any{"input": in.variable(), "namespace": optional(in.Namespace)}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Exposure, nil
}

// UnexposeHost withdraws a host. namespace is for platform administrators;
// a tenant leaves it empty.
func (c *Client) UnexposeHost(ctx context.Context, host, namespace string) error {
	return c.Do(ctx, `mutation($host: String!, $namespace: String) { unexposeHost(host: $host, namespace: $namespace) }`,
		map[string]any{"host": host, "namespace": optional(namespace)}, nil)
}

// States of a domain of the tenant's own.
const (
	// DomainPending means the TXT record has not been seen yet.
	DomainPending  = "PENDING"
	DomainVerified = "VERIFIED"
	// DomainLapsed means the TXT record went away: nothing new can be
	// published under the domain until it is verified again.
	DomainLapsed = "LAPSED"
)

// Domain is a domain of the tenant's own and the TXT record that proves it:
// publish a TXT record named ChallengeName with the value ChallengeValue
// and keep it there.
type Domain struct {
	Domain         string  `json:"domain"`
	Status         string  `json:"status"`
	ChallengeName  string  `json:"challengeName"`
	ChallengeValue string  `json:"challengeValue"`
	VerifiedAt     *string `json:"verifiedAt"`
	LastCheckedAt  *string `json:"lastCheckedAt"`
	CreatedAt      string  `json:"createdAt"`
}

const domainFields = `domain status challengeName challengeValue verifiedAt lastCheckedAt createdAt`

// DomainsAvailable reports whether domains of your own are switched on.
func (c *Client) DomainsAvailable(ctx context.Context) (bool, error) {
	var out struct {
		Available bool `json:"domainsAvailable"`
	}
	err := c.Do(ctx, `query { domainsAvailable }`, nil, &out)
	return out.Available, err
}

// ListDomains lists the domains the tenant registered.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out struct {
		Domains []Domain `json:"domains"`
	}
	err := c.Do(ctx, `query { domains { `+domainFields+` } }`, nil, &out)
	return out.Domains, err
}

// RegisterDomain registers a domain of the tenant's own, or repeats the
// registration: the answer names the TXT record to publish.
func (c *Client) RegisterDomain(ctx context.Context, domain string) (*Domain, error) {
	var out struct {
		Domain Domain `json:"registerDomain"`
	}
	if err := c.Do(ctx, `mutation($domain: String!) { registerDomain(domain: $domain) { `+domainFields+` } }`, map[string]any{"domain": domain}, &out); err != nil {
		return nil, err
	}
	return &out.Domain, nil
}

// VerifyDomain looks for the domain's TXT record in public DNS. It fails
// with the code CodeVerificationFailed until the record is there.
func (c *Client) VerifyDomain(ctx context.Context, domain string) (*Domain, error) {
	var out struct {
		Domain Domain `json:"verifyDomain"`
	}
	if err := c.Do(ctx, `mutation($domain: String!) { verifyDomain(domain: $domain) { `+domainFields+` } }`, map[string]any{"domain": domain}, &out); err != nil {
		return nil, err
	}
	return &out.Domain, nil
}

// RemoveDomain gives a domain up. It is refused while hosts are published
// under the domain.
func (c *Client) RemoveDomain(ctx context.Context, domain string) error {
	return c.Do(ctx, `mutation($domain: String!) { removeDomain(domain: $domain) }`, map[string]any{"domain": domain}, nil)
}
