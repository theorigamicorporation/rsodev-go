// Package platform is the client for the RSO Cloud platform API: the API the
// console uses, scoped to one tenant.
//
// A Client signs in with a service account, a person's session or a ready
// access token, talks GraphQL to the platform and offers typed operations
// per product: apps, instances, PostgreSQL, object storage, queues and
// topics, scheduled jobs, networking, DNS, Kubernetes clusters, secrets and
// access management. Anything without a typed operation yet is reachable
// through Client.Do.
//
//	c, err := platform.New(
//		platform.WithTenant("my-tenant"),
//		platform.WithServiceAccount(name, secret),
//	)
//	if err != nil { ... }
//	q, err := c.CreateQueue(ctx, "orders")
//
// Every operation takes a context. Reads are retried on transient failures
// (see RetryPolicy); writes are sent once. Refusals come back as *Error and
// are sorted by KindOf. Objects the platform creates in the background have
// a Wait... method.
//
// The platform decides where things run: a caller names a tenant and,
// optionally, a region, never a cluster or a host.
package platform
