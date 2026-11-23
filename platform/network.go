package platform

import "context"

// ListOptions pages and filters the list operations that support it. The
// zero value asks for the first page in the platform's default order.
type ListOptions struct {
	Limit  int
	Skip   int
	Search string
	// Sort names a field; Descending reverses the order.
	Sort       string
	Descending bool
}

func (o ListOptions) variable() any {
	list := map[string]any{}
	if o.Limit > 0 {
		list["limit"] = o.Limit
	}
	if o.Skip > 0 {
		list["skip"] = o.Skip
	}
	if o.Search != "" {
		list["search"] = o.Search
	}
	if o.Sort != "" {
		list["sort"] = o.Sort
		if o.Descending {
			list["order"] = "DESC"
		} else {
			list["order"] = "ASC"
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list
}

// Page is one page of a list and the total number of items there are.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// All collects every page of a list operation. fetch is one of the
// ...Page methods; pageSize zero means 100.
func All[T any](ctx context.Context, pageSize int, fetch func(context.Context, ListOptions) (*Page[T], error)) ([]T, error) {
	if pageSize <= 0 {
		pageSize = 100
	}
	var all []T
	for {
		page, err := fetch(ctx, ListOptions{Limit: pageSize, Skip: len(all)})
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.Total {
			return all, nil
		}
	}
}

// ServicePort is a port of an external service.
type ServicePort struct {
	Name       *string `json:"name,omitempty"`
	Port       int     `json:"port"`
	TargetPort int     `json:"targetPort"`
	// Protocol is "TCP" or "UDP".
	Protocol string `json:"protocol"`
}

// ExternalService names something outside the app platform, such as an
// instance, so that ingress routes can send traffic to it.
type ExternalService struct {
	Name string `json:"name"`
	// ExternalName is the address traffic is sent to.
	ExternalName string        `json:"externalName"`
	Ports        []ServicePort `json:"ports"`
}

const externalServiceFields = `name externalName ports { name port targetPort protocol }`

// ExternalServicesPage returns one page of the tenant's external services.
func (c *Client) ExternalServicesPage(ctx context.Context, o ListOptions) (*Page[ExternalService], error) {
	var out struct {
		Page Page[ExternalService] `json:"externalServices"`
	}
	err := c.Do(ctx, `query($list: List) { externalServices(list: $list) { total items { `+externalServiceFields+` } } }`, map[string]any{"list": o.variable()}, &out)
	return &out.Page, err
}

// ListExternalServices lists all of the tenant's external services.
func (c *Client) ListExternalServices(ctx context.Context) ([]ExternalService, error) {
	return All(ctx, 0, c.ExternalServicesPage)
}

// GetExternalService returns one external service by name.
func (c *Client) GetExternalService(ctx context.Context, name string) (*ExternalService, error) {
	var out struct {
		Service ExternalService `json:"externalService"`
	}
	err := c.Do(ctx, `query($id: ID!) { externalService(id: $id) { `+externalServiceFields+` } }`, map[string]any{"id": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out.Service, nil
}

// CreateExternalService creates an external service.
func (c *Client) CreateExternalService(ctx context.Context, in ExternalService) (*ExternalService, error) {
	var out struct {
		Service ExternalService `json:"createExternalService"`
	}
	err := c.Do(ctx, `mutation($input: CreateExternalServiceInput!) { createExternalService(input: $input) { `+externalServiceFields+` } }`, map[string]any{"input": in}, &out)
	return &out.Service, err
}

// DeleteExternalService deletes an external service.
func (c *Client) DeleteExternalService(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteExternalService(id: $id) }`, map[string]any{"id": name}, nil)
}

// RouteService is where a rule of an ingress route sends traffic: a port of
// an external service.
type RouteService struct {
	// Name is the name of the external service.
	Name string `json:"name"`
	Port int    `json:"port"`
	// Scheme is "http" or "https"; empty lets the platform decide.
	Scheme *string `json:"scheme,omitempty"`
}

// RouteRule is one rule of an ingress route.
type RouteRule struct {
	// Match is the rule traffic must satisfy, for example
	// Host(`shop.example.com`) && PathPrefix(`/api`).
	Match string `json:"match"`
	// Middlewares names middlewares applied in order.
	Middlewares []string       `json:"-"`
	Services    []RouteService `json:"services"`
}

// IngressRoute publishes services of the tenant under hostnames.
type IngressRoute struct {
	Name string `json:"name"`
	// EntryPoints and CertResolver choose the listener and how the
	// certificate is obtained; empty takes the platform's defaults.
	EntryPoints  []string    `json:"entryPoints,omitempty"`
	CertResolver *string     `json:"certResolver,omitempty"`
	Routes       []RouteRule `json:"routes"`
}

type routeWire struct {
	Match       string `json:"match"`
	Middlewares []struct {
		Name string `json:"name"`
	} `json:"middlewares"`
	Services []RouteService `json:"services"`
}

type ingressRouteWire struct {
	Name         string      `json:"name"`
	EntryPoints  []string    `json:"entryPoints"`
	CertResolver *string     `json:"certResolver"`
	Routes       []routeWire `json:"routes"`
}

func (w ingressRouteWire) route() IngressRoute {
	route := IngressRoute{Name: w.Name, EntryPoints: w.EntryPoints, CertResolver: w.CertResolver}
	for _, rule := range w.Routes {
		names := make([]string, 0, len(rule.Middlewares))
		for _, m := range rule.Middlewares {
			names = append(names, m.Name)
		}
		route.Routes = append(route.Routes, RouteRule{Match: rule.Match, Middlewares: names, Services: rule.Services})
	}
	return route
}

const ingressRouteFields = `name entryPoints certResolver routes { match middlewares { name } services { name port scheme } }`

// IngressRoutesPage returns one page of the tenant's ingress routes.
func (c *Client) IngressRoutesPage(ctx context.Context, o ListOptions) (*Page[IngressRoute], error) {
	var out struct {
		Page Page[ingressRouteWire] `json:"ingressRoutes"`
	}
	if err := c.Do(ctx, `query($list: List) { ingressRoutes(list: $list) { total items { `+ingressRouteFields+` } } }`, map[string]any{"list": o.variable()}, &out); err != nil {
		return nil, err
	}
	page := &Page[IngressRoute]{Total: out.Page.Total}
	for _, item := range out.Page.Items {
		page.Items = append(page.Items, item.route())
	}
	return page, nil
}

// ListIngressRoutes lists all of the tenant's ingress routes.
func (c *Client) ListIngressRoutes(ctx context.Context) ([]IngressRoute, error) {
	return All(ctx, 0, c.IngressRoutesPage)
}

// GetIngressRoute returns one ingress route by name.
func (c *Client) GetIngressRoute(ctx context.Context, name string) (*IngressRoute, error) {
	var out struct {
		Route ingressRouteWire `json:"ingressRoute"`
	}
	if err := c.Do(ctx, `query($id: ID!) { ingressRoute(id: $id) { `+ingressRouteFields+` } }`, map[string]any{"id": name}, &out); err != nil {
		return nil, err
	}
	route := out.Route.route()
	return &route, nil
}

// CreateIngressRoute creates an ingress route. Every service it names must
// be an external service of the tenant.
func (c *Client) CreateIngressRoute(ctx context.Context, in IngressRoute) (*IngressRoute, error) {
	routes := make([]map[string]any, 0, len(in.Routes))
	for _, rule := range in.Routes {
		middlewares := make([]map[string]any, 0, len(rule.Middlewares))
		for _, name := range rule.Middlewares {
			middlewares = append(middlewares, map[string]any{"name": name})
		}
		services := make([]map[string]any, 0, len(rule.Services))
		for _, service := range rule.Services {
			entry := map[string]any{"name": service.Name, "externalServiceId": service.Name, "port": service.Port}
			if service.Scheme != nil {
				entry["scheme"] = *service.Scheme
			}
			services = append(services, entry)
		}
		routes = append(routes, map[string]any{"match": rule.Match, "middlewares": middlewares, "services": services})
	}
	input := map[string]any{"name": in.Name, "routes": routes}
	if len(in.EntryPoints) > 0 {
		input["entryPoints"] = in.EntryPoints
	}
	if in.CertResolver != nil {
		input["certResolver"] = *in.CertResolver
	}
	var out struct {
		Route ingressRouteWire `json:"createIngressRoute"`
	}
	if err := c.Do(ctx, `mutation($input: CreateIngressRouteInput!) { createIngressRoute(input: $input) { `+ingressRouteFields+` } }`, map[string]any{"input": input}, &out); err != nil {
		return nil, err
	}
	route := out.Route.route()
	return &route, nil
}

// DeleteIngressRoute deletes an ingress route.
func (c *Client) DeleteIngressRoute(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteIngressRoute(id: $id) }`, map[string]any{"id": name}, nil)
}

// Kinds of middleware.
const (
	MiddlewareBasicAuth      = "basicAuth"
	MiddlewareRedirectRegex  = "redirectRegex"
	MiddlewareRedirectScheme = "redirectScheme"
	MiddlewareStripPrefix    = "stripPrefix"
)

// BasicAuth asks for a username and a password. The password is only sent,
// never returned.
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RedirectRegex redirects requests whose URL matches Regex to Replacement.
type RedirectRegex struct {
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Permanent   bool   `json:"permanent"`
}

// RedirectScheme redirects requests to another scheme, usually https.
type RedirectScheme struct {
	Scheme    string  `json:"scheme"`
	Port      *string `json:"port,omitempty"`
	Permanent bool    `json:"permanent"`
}

// StripPrefix removes path prefixes before a request is passed on.
type StripPrefix struct {
	Prefixes []string `json:"prefixes"`
}

// Middleware changes requests on their way through an ingress route. Type
// says which one of the settings applies.
type Middleware struct {
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	BasicAuth      *BasicAuth      `json:"basicAuth,omitempty"`
	RedirectRegex  *RedirectRegex  `json:"redirectRegex,omitempty"`
	RedirectScheme *RedirectScheme `json:"redirectScheme,omitempty"`
	StripPrefix    *StripPrefix    `json:"stripPrefix,omitempty"`
}

type middlewareWire struct {
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	RedirectRegex  *RedirectRegex  `json:"redirectRegex"`
	RedirectScheme *RedirectScheme `json:"redirectScheme"`
	StripPrefix    *StripPrefix    `json:"stripPrefix"`
}

func (w middlewareWire) middleware() Middleware {
	return Middleware{Name: w.Name, Type: w.Type, RedirectRegex: w.RedirectRegex, RedirectScheme: w.RedirectScheme, StripPrefix: w.StripPrefix}
}

const middlewareFields = `name type redirectRegex { regex replacement permanent } redirectScheme { scheme port permanent } stripPrefix { prefixes }`

// MiddlewaresPage returns one page of the tenant's middlewares.
func (c *Client) MiddlewaresPage(ctx context.Context, o ListOptions) (*Page[Middleware], error) {
	var out struct {
		Page Page[middlewareWire] `json:"middlewares"`
	}
	if err := c.Do(ctx, `query($list: List) { middlewares(list: $list) { total items { `+middlewareFields+` } } }`, map[string]any{"list": o.variable()}, &out); err != nil {
		return nil, err
	}
	page := &Page[Middleware]{Total: out.Page.Total}
	for _, item := range out.Page.Items {
		page.Items = append(page.Items, item.middleware())
	}
	return page, nil
}

// ListMiddlewares lists all of the tenant's middlewares.
func (c *Client) ListMiddlewares(ctx context.Context) ([]Middleware, error) {
	return All(ctx, 0, c.MiddlewaresPage)
}

// GetMiddleware returns one middleware by name.
func (c *Client) GetMiddleware(ctx context.Context, name string) (*Middleware, error) {
	var out struct {
		Middleware middlewareWire `json:"middleware"`
	}
	if err := c.Do(ctx, `query($id: ID!) { middleware(id: $id) { `+middlewareFields+` } }`, map[string]any{"id": name}, &out); err != nil {
		return nil, err
	}
	middleware := out.Middleware.middleware()
	return &middleware, nil
}

// CreateMiddleware creates a middleware.
func (c *Client) CreateMiddleware(ctx context.Context, in Middleware) (*Middleware, error) {
	var out struct {
		Middleware middlewareWire `json:"createMiddleware"`
	}
	if err := c.Do(ctx, `mutation($input: CreateMiddlewareInput!) { createMiddleware(input: $input) { `+middlewareFields+` } }`, map[string]any{"input": in}, &out); err != nil {
		return nil, err
	}
	middleware := out.Middleware.middleware()
	return &middleware, nil
}

// DeleteMiddleware deletes a middleware.
func (c *Client) DeleteMiddleware(ctx context.Context, name string) error {
	return c.Do(ctx, `mutation($id: ID!) { deleteMiddleware(id: $id) }`, map[string]any{"id": name}, nil)
}
