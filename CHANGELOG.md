# Changelog

All notable changes to this module are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the module
follows [Semantic Versioning](https://semver.org/).

## Unreleased

### Added

- `platform` package: a client for the RSO Cloud platform API.
- Sign-in with a service account, a ready access token, or a person's
  session (device sign-in with renewal).
- `Discover`: the sign-in endpoints and client are read from the gateway's
  configuration document when the caller sets neither, with the public
  cloud's values as the fallback.
- Typed operations for apps, instances, PostgreSQL clusters and backups,
  object storage, queues, topics, webhooks, scheduled jobs and messages,
  ingress routes, external services, middlewares, DNS records, container
  registries, secrets, SSH keys, volumes, Kubernetes clusters, service
  accounts, roles, scoped access and quotas.
- Waiters, paging helpers, retries for reads and error kinds.
- Cost estimates, personal SSH keys, ingress routes without a certificate
  and with transport settings per service.
