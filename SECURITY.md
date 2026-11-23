# Security

## Reporting a vulnerability

Please do not open a public issue for a security problem. Use GitHub's
private vulnerability reporting on this repository ("Security" tab, "Report
a vulnerability"). Include what is affected, how to reproduce it and what
an attacker gains.

You will get an acknowledgement, and a fix or a plan once the report is
confirmed. Please give us reasonable time before disclosing.

## Scope

This repository is the Go client. Problems in the platform API itself are
welcome through the same channel.

## Handling credentials

- Service account secrets, access tokens and refresh tokens are sent only
  to the configured token endpoint and API endpoint, over HTTPS by default.
- The SDK never logs credentials. Errors carry the API's message, not the
  request.
- Secrets the platform shows once (service account secrets, storage keys,
  queue credentials) are returned to the caller and not kept.
