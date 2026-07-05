# Changelog

All notable changes to Cloud Forge CLI are documented in this file.

## [Unreleased]

### Added

- `AdminPassword` auto-generation and `--admin-password` deploy flag
- `scripts/verify-admin-password-start.sh` local verification helper

### Changed

- README (EN/zh-CN) and SMOKE-TEST: catalog-centric app discovery instead of fixed app list
- `verify-aws-apps.sh` and `verify-aliyun-apps.sh` select apps via catalog `list-verify-apps.sh` (default: `certified` tier)
- Supports `CLOUD_FORGE_VERIFY_TIERS` and `CLOUD_FORGE_VERIFY_SAMPLE` for community random sampling

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
