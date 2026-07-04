# Changelog

All notable changes to Cloud Forge CLI are documented in this file.

## [Unreleased]

### Added

- Aliyun deploy waits for app bootstrap by default after stack `CREATE_COMPLETE` (`--wait-ready`, default true; `--no-wait-ready` to skip)
- HTTP polling of stack `ServiceURL` `/health` and `/` until ready or `--timeout`

## [0.3.0] - 2026-07-04

### Added

- `cloud-forge auth aliyun` and `cloud-forge auth aliyun status` for AccessKey credentials
- Aliyun ROS deploy and delete via `internal/aliyundeploy` (Hong Kong `cn-hongkong` only)
- `--vswitch-id` deploy flag for Aliyun VSwitchId
- Aliyun-specific deploy warnings, progress output, and error mapping

### Changed

- `deploy` and `delete` route to AWS CloudFormation or Aliyun ROS based on `--cloud`
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
