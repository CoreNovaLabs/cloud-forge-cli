# Changelog

All notable changes to Cloud Forge CLI are documented in this file.

## [Unreleased]

## [0.3.7] - 2026-07-06

### Added

- Windows PowerShell installer via `scripts/install.ps1`.
- Windows ARM64 release archive.

### Fixed

- CLI upgrade guidance now shows a PowerShell install command on Windows.

## [0.3.6] - 2026-07-06

### Fixed

- Windows browser launch now opens Cloud Forge through PowerShell with `-NoExit`, keeping deploy progress and startup errors visible.

## [0.3.5] - 2026-07-06

### Fixed

- macOS browser launch now opens a temporary `.command` file in Terminal instead of controlling Terminal through AppleScript automation permissions.

## [0.3.4] - 2026-07-06

### Added

- Browser launcher deep-link support via `cloud-forge launch-url` and `cloud-forge install-protocol`
- Catalog `min_cli_version` enforcement before template fetch or deploy

### Fixed

- Scope index and template caches by catalog URL so `--store-url` and `CLOUD_FORGE_STORE_URL` cannot reuse stale default catalog data
- `show --cloud` now filters parameter required/default/option display to the selected cloud

## [0.3.3] - 2026-07-05

### Added

- Catalog `ami_role` and `service_port` fields in `show`
- TCP readiness probes for non-HTTP service URLs such as PostgreSQL, MySQL, Redis, MongoDB, and MQTT
- AWS AMI discovery hook for database-role catalog apps when no `LatestAmiId` is provided

### Changed

- AWS deploy parameter building preserves catalog/default AMI IDs and explicit `--latest-ami-id` / `--image-id` overrides before attempting database AMI discovery

## [0.3.2] - 2026-07-05

### Added

- End-to-end custom domain binding: `--domain` flows through IaC, bootstrap, Caddy, and `ServiceURL`
- Aliyun automatic DNS A records via `--dns-domain` (ROS `ALIYUN::DNS::DomainRecord`, aligned with AWS Route53 + `--hosted-zone-id`)
- `--caddy-email` for optional ACME contact email
- Domain validation and deploy hints (Route53 / Alidns / manual DNS warnings)
- Deploy progress prints public IP as soon as the EIP resource completes (AWS and Aliyun)
- Bootstrap wait logs show Public IP, Service URL, and probe targets before polling starts

### Changed

- Catalog templates pass `CLOUD_FORGE_DOMAIN_NAME` to bootstrap; domain mode uses Caddy `auto` TLS instead of IP certificates
- `CaddyTlsMode` catalog options include `auto`

## [0.3.1] - 2026-07-05

### Added

- `AdminPassword` auto-generation and `--admin-password` deploy flag
- Aliyun deploy auto-discovers VPC/VSwitch/KeyPair when `--vpc-id`, `--vswitch-id`, and `--key` are omitted (imports `cloud-forge-default` when needed)

### Changed

- README (EN/zh-CN) restructured; Quick Start no longer requires `--allowed-ip` by default
- `verify-aws-apps.sh` and `verify-aliyun-apps.sh` select apps via catalog `list-verify-apps.sh` (default: `certified` tier)
- Aliyun region defaults to `cn-hongkong` but other regions are allowed; mainland China bootstrap warnings documented
- Catalog search refreshes the index when cached results are empty
- Version bumped to `0.3.1`

## [0.3.0] - 2026-07-04

### Added

- `cloud-forge auth aliyun` and `cloud-forge auth aliyun status` for AccessKey credentials
- Aliyun ROS deploy and delete via `internal/aliyundeploy` (Hong Kong `cn-hongkong` only)
- `--vswitch-id` deploy flag for Aliyun VSwitchId
- Aliyun-specific deploy warnings, progress output, and error mapping
- AWS and Aliyun deploy wait for app bootstrap by default after stack `CREATE_COMPLETE` (`--wait-ready`, default true; `--no-wait-ready` to skip)
- HTTP polling of stack `ServiceURL` `/health` and `/` until ready or `--timeout`
- `scripts/verify-aws-apps.sh` and `scripts/verify-aliyun-apps.sh` for live end-to-end smoke tests

### Changed

- `deploy` and `delete` route to AWS CloudFormation or Aliyun ROS based on `--cloud`
- AWS deploy now polls the service endpoint after `CREATE_COMPLETE`, matching the Aliyun behavior; `--wait-ready`/`--no-wait-ready`/`--timeout` apply to both clouds
- Version bumped to `0.3.0`

### Notes

- Aliyun v1 bootstraps Docker/Caddy on public OS images; first deploy may take 8–15 minutes
- Requires catalog `0.3.0` templates and bootstrap scripts

## [0.2.0] - 2026-07-03

### Added

- `cloud-forge delete` (alias `destroy`) to remove AWS CloudFormation stacks from the CLI
- One-line install script at `scripts/install.sh`
- Command-specific help via `cloud-forge help <command>` and `<command> --help`
- User-friendly error messages for common AWS auth and permission failures
- Deploy warnings for AWS billing, Marketplace software fees, and open SSH CIDRs
- Enhanced `cloud-forge show` output with parameter defaults and options
- MIT License

### Changed

- First public release supports four AWS catalog apps: `hello-nginx`, `gitea`, `n8n`, `uptime-kuma`
- Deploy success output now includes a `cloud-forge delete` cleanup hint

### Notes

- Deploy and delete require AWS credentials (`cloud-forge auth aws`)
