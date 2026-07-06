<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

<p align="center">
  让开源应用可以一键部署到云端。支持 AWS（CloudFormation + 安全加固 AMI）与 Aliyun（ROS），并内置授权向导。
</p>

<p align="center">
  <a href="README.md">English</a>
  ·
  <a href="#安装">安装</a>
  ·
  <a href="#快速开始">快速开始</a>
  ·
  <a href="#aws-部署">AWS 部署</a>
  ·
  <a href="#aliyun-部署">Aliyun 部署</a>
  ·
  <a href="#命令参考">命令参考</a>
</p>

<p align="center">
  <img alt="AWS" src="https://img.shields.io/badge/AWS-deploy%20%7C%20delete-ff9900" />
  <img alt="Aliyun" src="https://img.shields.io/badge/Aliyun-deploy%20%7C%20delete-0089FF" />
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/releases"><img alt="Release" src="https://img.shields.io/github/v/release/CoreNovaLabs/cloud-forge-cli?sort=semver" /></a>
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml"><img alt="Test CLI" src="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml/badge.svg" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-green.svg" /></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/CoreNovaLabs/cloud-forge-cli" /></a>
</p>

```bash
# macOS / Linux
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
cloud-forge auth aws
cloud-forge deploy hello-nginx --cloud aws
```

```powershell
# Windows PowerShell
irm https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.ps1 | iex
cloud-forge auth aws
cloud-forge deploy hello-nginx --cloud aws
```

Cloud Forge CLI 是 [Cloud Forge Catalog](https://github.com/CoreNovaLabs/cloud-forge-catalog) 的命令行入口：查找应用、查看模板、在 AWS 或 Aliyun 上部署栈，并在用完后清理资源。

| 能力 | 说明 |
| --- | --- |
| 查找应用 | 用 `search` / `show` 浏览应用；参数与云支持来自各应用 manifest。 |
| AWS 部署 | 创建或更新 CloudFormation 栈，并在终端查看进度。 |
| Aliyun 部署 | 通过 ROS 创建 ECS + EIP 并引导应用容器（默认区域 `cn-hongkong`）。 |
| 内置授权 | AWS 浏览器登录或 Access Key；Aliyun AccessKey 配置。 |
| 清理资源 | 删除 Cloud Forge 创建的栈，释放关联云资源。 |

**Catalog 说明**

- 索引：[cloud-forge-catalog/index/apps.json](https://github.com/CoreNovaLabs/cloud-forge-catalog/blob/main/index/apps.json)
- 凡 `clouds` 含 `aws` 或 `aliyun` 的应用均可部署；参数以 `cloud-forge show <app>` 为准
- `certified` 应用经过更完整云端验证，`community` 应用迭代更快
- Aliyun v1 使用公共系统镜像 + UserData 引导，首次启动约 8～15 分钟

默认区域：AWS `us-east-1`，Aliyun `cn-hongkong`（可用 `--region` 覆盖；中国大陆区域可能因网络限制导致 bootstrap 失败）。

## 安装

macOS 和 Linux 上，上方一行命令会把 `cloud-forge` 安装到 `~/.local/bin`。若 shell 找不到命令，请将该目录加入 `PATH`。

Windows 请在 PowerShell 中运行：

```powershell
irm https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.ps1 | iex
```

Windows 安装器会把 `cloud-forge.exe` 写入 `%LOCALAPPDATA%\Programs\CloudForge`，并将该目录加入用户 `PATH`。

若 CDN 不可用，可改用 GitHub raw 地址：

```bash
curl -fsSL https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-cli/main/scripts/install.sh | bash
```

```powershell
irm https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-cli/main/scripts/install.ps1 | iex
```

也可从 [GitHub Releases](https://github.com/CoreNovaLabs/cloud-forge-cli/releases) 手动下载：解压后将二进制放到 `PATH` 中的目录。

验证安装：

```bash
cloud-forge version
```

本地编译见 [从源码构建](#从源码构建)。

## AWS 凭证

Cloud Forge CLI 通过 AWS SDK for Go v2 调用 AWS，内置浏览器登录流程，无需安装 AWS CLI，但需要有 AWS 身份或 Access Key。

```bash
cloud-forge auth aws
cloud-forge auth aws status
```

默认 `cloud-forge auth aws` 会打开 AWS 登录页，并将临时凭证写入本地 profile（AWS Sign-In OAuth + PKCE）。使用 `--no-browser` 可只打印 URL 并手动粘贴授权码。使用 `--profile NAME` 可指定 profile。

其他支持的凭证来源：`~/.aws/credentials`、`~/.aws/config`、`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`、`AWS_PROFILE`、AWS SSO 或 assume-role profile、EC2/ECS instance 或 task role。

```bash
export AWS_PROFILE=default
# 或
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

需要时可覆盖默认部署区域：

```bash
cloud-forge deploy hello-nginx --cloud aws --region us-west-2
```

授权方式变体：

```bash
cloud-forge auth aws --method browser          # 默认
cloud-forge auth aws --method browser --no-browser
cloud-forge auth aws --method access-key
```

生产环境建议使用最小权限 IAM user 或 role，避免使用 AWS 根账号凭证。

## 快速开始

```bash
cloud-forge search nginx --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge deploy hello-nginx --cloud aws
```

未指定 `--allowed-ip` 时，SSH 默认对 `0.0.0.0/0` 开放，CLI 会打印安全警告。需要限制访问时使用 `--allowed-ip <your-ip>/32`。

栈创建完成后，CLI 会打印 Outputs 和清理提示：

```text
To remove later: cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

Aliyun 部署见 [Aliyun 部署](#aliyun-部署)。

## 清理资源

```bash
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
cloud-forge delete cloud-forge-<app-id> --cloud aws --wait      # 默认：等待完成
cloud-forge delete cloud-forge-<app-id> --cloud aws --no-wait   # 立即返回
```

删除栈会清理模板创建的 EC2 实例、Elastic IP、安全组和相关资源。

## AWS 部署

AWS 部署使用 AWS SDK for Go v2 与 CloudFormation，底层不调用 AWS CLI。凭证来自标准 AWS SDK 链。默认区域 `us-east-1`（可用 `--region` 覆盖）。

创建或更新栈：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro
```

默认会打印 CloudFormation 资源事件：

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

```bash
cloud-forge deploy hello-nginx --cloud aws --progress none   # 关闭进度输出
```

栈达到 `CREATE_COMPLETE` 后，`deploy` 会继续轮询 `ServiceURL`（`/health` 与 `/`），直到应用响应或达到 `--timeout`。首次启动仍需拉取镜像并申请 TLS，用于填平「栈完成」与「服务可访问」之间的空档。加 `--no-wait-ready` 可在栈创建完成后立即返回。

### SSH Key 行为

默认使用本地可复用密钥 `~/.cloud-forge/keys/aws/cloud-forge-default.pem`。首次使用时 CLI 以 `0600` 权限创建该文件；若目标区域尚无 `cloud-forge-default` 密钥对，会将公钥导入 EC2。同一本地密钥跨区域复用。删除栈不会删除该文件。

```bash
cloud-forge deploy hello-nginx --cloud aws --key-name my-key
cloud-forge deploy hello-nginx --cloud aws --ssh-key none
cloud-forge deploy hello-nginx --cloud aws --ssh-key-path ~/.cloud-forge/keys/aws/custom.pem
```

## Aliyun 部署

Aliyun 使用 ROS 创建 ECS + EIP，并通过 UserData 引导 Docker/Caddy 与应用容器。与 AWS 预烘焙 AMI 不同，首次可用约 **8～15 分钟**。

**区域：** 默认 **`cn-hongkong`**。可用 `--region` 指定其他区域（如 `ap-southeast-1`）。**中国大陆区域**（`cn-hangzhou`、`cn-shanghai` 等）可能因 Docker Hub 与 catalog CDN 网络受限导致 bootstrap 失败或极慢，建议默认使用香港。

**网络：** 省略 `--vpc-id`、`--vswitch-id`、`--key` 时，CLI 会在当前区域自动选用 VPC/VSwitch，并复用已有 KeyPair（或像 AWS 一样导入 `cloud-forge-default`）。可用 flag 或环境变量 `ALIYUN_VPC_ID`、`ALIYUN_VSWITCH_ID`、`ALIYUN_KEY_NAME` 覆盖。

```bash
cloud-forge auth aliyun
cloud-forge auth aliyun status

cloud-forge deploy hello-nginx --cloud aliyun --timeout 20m

# 显式指定网络（可选）
cloud-forge deploy hello-nginx --cloud aliyun \
  --vpc-id vpc-xxx --vswitch-id vsw-xxx --key my-key

# 其他区域
cloud-forge deploy hello-nginx --cloud aliyun --region ap-southeast-1

# 仅创建栈，不等待应用就绪
cloud-forge deploy hello-nginx --cloud aliyun --no-wait-ready

cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
```

ROS `CREATE_COMPLETE` 后，`deploy` 与 AWS 相同方式轮询 `ServiceURL`。加 `--no-wait-ready` 可跳过应用就绪等待。容器镜像使用 Docker Hub 短名；香港 ECS 通常可直连 Docker Hub，其他区域可能需要自行配置镜像加速或私有仓库。

## 常用选项

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --region us-east-1 \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32 \
  --progress plain
```

模板参数可用专用 flag 或重复 `--param` 传入：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --param KeyName=my-key \
  --param InstanceType=t3.micro
```

常用 deploy flag（各应用可能有更多，以 `cloud-forge show <app>` 为准）：

- **实例：** `--instance-type`、`--disk-size`、`--image-id`、`--latest-ami-id`
- **网络：** `--vpc` / `--vpc-id`、`--subnet` / `--subnet-id`、`--vswitch-id`、`--allowed-ip`
- **SSH / 密钥：** `--key` / `--key-name`、`--ssh-key`、`--ssh-key-path`
- **DNS / TLS：** `--domain`、`--hosted-zone-id`（AWS Route53）、`--dns-domain`（阿里云 Alidns）、`--caddy-tls-mode`、`--caddy-email`
- **其他：** `--progress`、`--admin-password`

## 应用密码（AdminPassword）

部分应用（如 `code-server`、`minio`）需要管理员密码。`cloud-forge show <app>` 会列出 `AdminPassword optional secret`。

```bash
cloud-forge deploy minio --cloud aws --admin-password 'MyStr0ngPass'
# 或
cloud-forge deploy minio --cloud aws --param AdminPassword='MyStr0ngPass'
```

未指定时，CLI 自动生成 24 位随机密码，写入 IaC 参数，并在**成功 deploy 后打印一次**（`--dry-run` 仅提示将自动生成）。密码不会写入 Stack Outputs 或使用统计。

## 自定义域名

使用 `--domain` 绑定 HTTPS 访问域名。DNS 可自动创建或手动配置。

**AWS（Route53）：**

```bash
cloud-forge deploy gitea --cloud aws \
  --domain git.example.com \
  --hosted-zone-id Z1234567890 \
  --caddy-email ops@example.com
```

**阿里云（Alidns）：** 根域名须已在[阿里云云解析 DNS](https://dns.console.aliyun.com/) 托管；部署 RAM 用户需 DNS 写权限（如 `AliyunDNSFullAccess`）。

```bash
cloud-forge deploy gitea --cloud aliyun --region cn-hongkong \
  --domain git.example.com \
  --dns-domain example.com \
  --caddy-email ops@example.com
```

**手动 DNS：** 仅传 `--domain`（省略 `--hosted-zone-id` 或 `--dns-domain`），CLI 会提示手动添加指向 EIP 的 A 记录。DNS 传播与 Let's Encrypt 证书签发可能需要数分钟；域名部署建议 `--timeout` ≥ 15m 并配合 `--wait-ready`。

未指定 `--domain` 时，默认仍为 Let's Encrypt **IP** 证书（`ip-letsencrypt`），`ServiceURL` 使用弹性 IP。

## 应用目录来源

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

默认 CDN 镜像不可用时，CLI 会回退到 GitHub raw 地址。

本地开发：

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## 命令参考

```bash
cloud-forge search hello --cloud aws
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge deploy hello-nginx --cloud aliyun --region cn-hongkong --dry-run
cloud-forge auth aws status
cloud-forge auth aliyun status
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
cloud-forge delete cloud-forge-hello-nginx --cloud aliyun --region cn-hongkong
cloud-forge help deploy
```

## 使用统计

默认向 `https://telemetry.corenovacloud.com/v1/events` 发送匿名、非阻塞的使用事件。不包含云凭证、账号 ID、域名、本地路径或模板参数值。

```bash
export CLOUD_FORGE_TELEMETRY=0
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"   # 本地测试
```

## 从源码构建

```bash
go build ./cmd/cloud-forge
```

## 开发

```bash
go test ./...
```
