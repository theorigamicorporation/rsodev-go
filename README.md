# rsodev-go

[![Tests](https://github.com/theorigamicorporation/rsodev-go/actions/workflows/test.yml/badge.svg)](https://github.com/theorigamicorporation/rsodev-go/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/theorigamicorporation/rsodev-go/platform.svg)](https://pkg.go.dev/github.com/theorigamicorporation/rsodev-go/platform)

The Go SDK for [RSO Cloud](https://console.cloud.rso.dev). It talks to the
same API the console uses and offers typed operations for everything a
tenant can manage. The Terraform provider and the `rso` command line tool
are built on it.

- Standard library only; one package to import.
- A `context.Context` on every call.
- Reads are retried on transient failures; writes are sent once.
- Waiters for objects that are created in the background.
- Errors sorted into a few kinds you can act on.

## Install

```sh
go get github.com/theorigamicorporation/rsodev-go
```

Go 1.25 or newer.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/theorigamicorporation/rsodev-go/platform"
)

func main() {
	c, err := platform.New(
		platform.WithTenant(os.Getenv("RSO_TENANT")),
		platform.WithServiceAccount(os.Getenv("RSO_SERVICE_ACCOUNT_NAME"), os.Getenv("RSO_SERVICE_ACCOUNT_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	_, err = c.CreateApp(ctx, platform.AppInput{
		Name: "web",
		Template: platform.AppTemplate{Spec: platform.AppSpec{Containers: []platform.AppContainer{{
			Image: "registry.example.com/web:1.0.0",
			Ports: []platform.ContainerPort{{ContainerPort: 8080}},
		}}}},
	})
	if err != nil {
		log.Fatal(err)
	}
	app, err := c.WaitAppReady(ctx, "web", platform.WaitOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("serving at", *app.Status.URL)
}
```

## Signing in

### Service account

For automation. Create a service account in the console (Access, Service
accounts) or with `CreateServiceAccount`, then:

```go
c, err := platform.New(
	platform.WithTenant("my-tenant"),
	platform.WithServiceAccount(name, secret),
)
```

The client signs in on the first call and renews the token before it
expires.

### A workload, without a secret

For a program running in Kubernetes, where the cluster and the client id
have been set up for it with the platform. The program's projected
service-account token is exchanged for an access token; nothing is stored.
The file is read again for every exchange, so a rotated token is picked up.

```go
c, err := platform.New(
	platform.WithTenant("my-tenant"),
	platform.WithWorkloadIdentity("/var/run/secrets/tokens/platform"),
	platform.WithClientID(clientID),
)
```

### A person, on a device

For tools people run. The person confirms a code in the browser; the
session renews itself and `Save` lets you keep it between runs.

```go
oauth := platform.OAuth{}
code, err := oauth.StartDevice(ctx)
// show code.VerificationURIComplete and code.UserCode
session, err := oauth.WaitDevice(ctx, code)

c, err := platform.New(
	platform.WithTenant("my-tenant"),
	platform.WithTokenSource(&platform.RefreshingSource{
		OAuth: oauth,
		Set:   session,
		Save:  func(s platform.TokenSet) error { return store(s) },
	}),
)
```

A client without `WithTenant` can ask who signed in and which tenants they
belong to (`Me`), which is how a tool lets the person pick one.

### A ready access token

```go
c, err := platform.New(platform.WithTenant("my-tenant"), platform.WithAccessToken(token))
```

The client cannot renew it.

## What it covers

| Product | Operations |
|---|---|
| Apps | `ListApps`, `GetApp`, `FindApp`, `CreateApp`, `UpdateApp`, `DeleteApp`, `AppLogs`, `WaitAppReady` |
| Instances | `ListInstances`, `GetInstance`, `FindInstanceByName`, `ListInstanceTemplates`, `CreateInstance`, `CreateInstanceAndWait`, `StartInstance`, `StopInstance`, `RebootInstance`, `ResizeInstance`, `ResizeInstanceDisk`, `DeleteInstance`, `WaitInstanceStatus` |
| PostgreSQL | `ListPostgresClusters`, `GetPostgresCluster`, `CreatePostgresCluster`, `UpdatePostgresCluster`, `DeletePostgresCluster`, `WaitPostgresClusterReady`, databases (`ListPostgresDatabases`, `CreatePostgresDatabase`, `UpdatePostgresDatabase`, `DeletePostgresDatabase`), backups (`GetPostgresBackups`, `EnablePostgresBackups`, `SetPostgresBackupSchedule`, `RemovePostgresBackupSchedule`, `BackupPostgresCluster`, `RestorePostgresCluster`) |
| Object storage | `ListBuckets`, `FindBucket`, `CreateBucket`, `DeleteBucket`, `ObjectStorageInfo`, access keys (`ListStorageKeys`, `CreateStorageKey`, `DeleteStorageKey`), bucket notifications (`ListBucketNotifications`, `CreateBucketNotification`, `DeleteBucketNotification`) |
| Queues and topics | `CreateQueue`, `ListQueues`, `GetQueue`, `PurgeQueue`, `DeleteQueue`, `CreateTopic`, `SubscribeQueue`, credentials, `Publish`, `PublishToTopic`, `Receive`, waiters |
| Webhooks | `ListWebhooks`, `GetWebhook`, `CreateWebhook`, `UpdateWebhook`, `ResumeWebhook`, `DeleteWebhook` |
| Scheduled jobs | `ListScheduledJobs`, `GetScheduledJob`, `CreateScheduledJob`, `UpdateScheduledJob`, `SetScheduledJobSuspended`, `RunScheduledJob`, `ListScheduledJobRuns`, `ScheduledJobLogs`, `DeleteScheduledJob` |
| Scheduled messages | `ListScheduledMessages`, `GetScheduledMessage`, `CreateScheduledMessage`, `UpdateScheduledMessage`, `RunScheduledMessage`, `DeleteScheduledMessage` |
| Networking | external services, ingress routes and middlewares: `List…`, `…Page`, `Get…`, `Create…`, `Delete…`; `CreateVPC` |
| DNS | `DNSAvailable`, `ListDNSZones`, `ListDNSRecords`, `GetDNSRecord`, `CreateDNSRecord`, `CreateDNSRecordWith` (with a TTL), `UpdateDNSRecord`, `DeleteDNSRecord`; zones of your own (preview): `GetDNSZone`, `CreateDNSZone`, `DeleteDNSZone` |
| Published hosts (preview) | `ExposuresAvailable`, `ListExposures`, `ExposuresPage`, `GetExposure`, `ExposeHost`, `UnexposeHost` |
| Domains of your own (preview) | `DomainsAvailable`, `ListDomains`, `RegisterDomain`, `VerifyDomain`, `RemoveDomain` |
| Kubernetes | `ListKubernetesClusters`, `GetKubernetesCluster`, `CreateKubernetesCluster`, `DeleteKubernetesCluster`, `ScaleNodePool`, `AddNodePool`, `RemoveNodePool`, `UpgradeKubernetesCluster`, `WaitKubernetesClusterReady`, `Kubeconfig`, `ClusterToken` |
| Container registries | `ListRegistries`, `GetRegistry`, `CreateRegistry`, `DeleteRegistry`, `IssueRegistryCredentials` |
| Secrets | `ListSecrets`, `GetSecret`, `CreateSecret`, `UpdateSecret`, `DeleteSecret` |
| SSH keys | `ListSSHKeys`, `GetSSHKey`, `AddSSHKey`, `RenameSSHKey`, `DeleteSSHKey` |
| Volumes | `ListVolumes`, `CreateVolume`, `DeleteVolume` |
| Access | service accounts, roles, `ListPrincipals`, `SetPrincipalAccess` (per-object scopes), `GrantPermissions`, `RevokePermissions`, resource labels |

Operations marked preview are not switched on everywhere yet: where they are
not, `ExposuresAvailable` and `DomainsAvailable` answer false and the other
calls fail with `NOT_CONFIGURED`.
| Account | `Me`, `Permissions`, `Quota`, `PlatformEvents` |
| AI models | `ListAIModels`, `Chat` (streaming) |

Anything else the API offers is one call away:

```go
var out struct {
	Quota []struct {
		Name  string  `json:"name"`
		Used  float64 `json:"used"`
		Limit float64 `json:"limit"`
	} `json:"quota"`
}
err := c.Do(ctx, `query { quota { name used limit } }`, nil, &out)
```

## Errors

Refusals come back as `*platform.Error`. `KindOf` sorts any error:

```go
_, err := c.CreateBucket(ctx, "assets")
switch platform.KindOf(err) {
case platform.KindConflict:   // already exists
case platform.KindQuota:      // the tenant's quota does not allow it
case platform.KindForbidden:  // this identity may not do it
case platform.KindNotFound, platform.KindInvalid, platform.KindUnauthenticated, platform.KindUnavailable:
}
```

`IsNotFound`, `IsConflict`, `IsQuotaExceeded`, `IsForbidden` and
`IsUnauthenticated` are shorthands.

## Paging, waiting and retries

- Lists that page have a `…Page(ctx, ListOptions)` method returning a
  `Page[T]` with the total; `platform.All` collects every page, and the
  `List…` methods do that for you.
- `Wait…` methods poll until an object is usable. `WaitOptions` sets the
  interval and the timeout (one second and two minutes by default).
- Reads are tried three times with a short backoff. `WithRetry` changes
  that; `RetryPolicy{Attempts: 1}` turns it off.

## Configuration

| Option | Purpose |
|---|---|
| `WithTenant` | the tenant every call is scoped to |
| `WithRegion` | a region of the tenant; the platform has a default |
| `WithServiceAccount`, `WithWorkloadIdentity`, `WithAccessToken`, `WithTokenSource` | credentials |
| `WithWorkloadIdentityScope` | scope the workload sign-in asks for |
| `WithEndpoint`, `WithTokenURL`, `WithClientID` | other deployments of the platform |
| `WithHTTPClient` | your own `*http.Client` (60 second timeout by default) |
| `WithUserAgent` | your tool's product token, sent before the SDK's |
| `WithRetry` | the retry policy for reads |

The platform decides where things run. You name a tenant and, optionally, a
region.

## Development

```sh
just check   # formatting, vet, lint, tests
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
