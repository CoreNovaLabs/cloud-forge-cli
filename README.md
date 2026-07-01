<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

Cloud Forge CLI is the command-line client for the Cloud Forge catalog. The current foundation supports catalog search, app inspection, and template download. Cloud deployers are intentionally kept behind the catalog boundary and will be added after AWS and Aliyun templates are validated end to end.

## Build

```bash
go build ./cmd/cloud-forge
```

## Catalog

By default the CLI reads:

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

If the default mirror is unavailable, the CLI falls back to the GitHub raw catalog URL.

For local development:

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## Commands

```bash
cloud-forge search hello --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
```

## Telemetry

The CLI sends anonymous, non-blocking usage events to:

```text
https://telemetry.corenovacloud.com/v1/events
```

Telemetry does not include cloud credentials, account IDs, domains, local paths, or template parameter values.

Disable it when needed:

```bash
export CLOUD_FORGE_TELEMETRY=0
```

Use a different endpoint for local testing:

```bash
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"
```

## Development

```bash
go test ./...
```
