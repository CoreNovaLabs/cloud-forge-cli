# Changelog

All notable changes to Cloud Forge CLI are documented in this file.

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
- Aliyun templates remain browsable; deploy and delete are AWS-only in v1

### Notes

- Deploy and delete require AWS credentials (`cloud-forge auth aws`)
- Aliyun ROS templates are catalog-only until a future release
