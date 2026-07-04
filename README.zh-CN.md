<p align="center">
  <img src="assets/cloud-forge-logo.png" alt="Cloud Forge CLI logo" width="160" />
</p>

<h1 align="center">Cloud Forge CLI</h1>

<p align="center">
  让开源应用可以一键部署到云端。当前 AWS 部署基于 CloudFormation 和安全加固 AMI，并内置授权向导。
</p>

<p align="center">
  <a href="README.md">English</a>
  ·
  <a href="#快速开始">快速开始</a>
  ·
  <a href="#aws-凭证">AWS 凭证</a>
  ·
  <a href="#清理资源">清理资源</a>
</p>

<p align="center">
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml"><img alt="Test CLI" src="https://github.com/CoreNovaLabs/cloud-forge-cli/actions/workflows/test.yml/badge.svg" /></a>
  <a href="https://github.com/CoreNovaLabs/cloud-forge-cli/releases"><img alt="Release" src="https://img.shields.io/github/v/release/CoreNovaLabs/cloud-forge-cli?sort=semver" /></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-green.svg" /></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/CoreNovaLabs/cloud-forge-cli" /></a>
  <img alt="AWS" src="https://img.shields.io/badge/AWS-deploy%20%7C%20delete-ff9900" />
</p>

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
cloud-forge auth aws
cloud-forge deploy hello-nginx --cloud aws --allowed-ip <YOUR_IP>/32
```

Cloud Forge 的目标，是把大量开源应用整理成可以一键部署的云端应用目录。Cloud Forge CLI 是这个目录的命令行入口：你可以用它查找应用、查看模板、部署 AWS CloudFormation 栈，并在用完后清理这些资源。

| 能力 | 说明 |
| --- | --- |
| 查找应用 | 浏览持续扩展的应用目录，包括 `hello-nginx`、`gitea`、`n8n`、`uptime-kuma` 等应用。 |
| AWS 部署 | 创建或更新 CloudFormation 栈，并在终端查看进度。 |
| 内置授权 | 通过浏览器登录，或在不安装 AWS CLI 的情况下配置 Access Key。 |
| 清理资源 | 删除 CloudFormation 栈，释放它创建的 AWS 资源。 |

**当前支持范围：** `hello-nginx`、`gitea`、`n8n`、`uptime-kuma` 已支持 AWS 部署和删除。Aliyun 模板可以列出和下载，但部署功能当前只支持 AWS。

## 功能概览

Cloud Forge CLI 把目录应用转成可重复执行的部署流程。长期目标是覆盖更多开源应用；当前 CLI 先聚焦 AWS 部署。

当前在 AWS 上可以：

- 查找应用
- 查看应用元数据
- 输出应用对应的 CloudFormation 模板
- 创建或更新 CloudFormation 栈
- 删除 CloudFormation 栈
- 在部署和删除时显示 CloudFormation 资源进度
- 复用本地 SSH key 访问 EC2
- 如果模板定义了 Outputs，打印服务 URL、公网 IP、实例 ID、AMI ID 和区域等信息

部署到 AWS 时默认使用 `us-east-1`。

## 安装

推荐一行安装：

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-cli@main/scripts/install.sh | bash
```

安装脚本会把 `cloud-forge` 放到 `~/.local/bin`。如果 shell 找不到这个命令，把该目录加入 `PATH`。

如果当前网络访问 CDN 有问题，可以改用 GitHub raw 地址：

```bash
curl -fsSL https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-cli/main/scripts/install.sh | bash
```

也可以从 GitHub Releases 手动下载：

```text
https://github.com/CoreNovaLabs/cloud-forge-cli/releases
```

解压后，把 `cloud-forge` 二进制文件移动到 `PATH` 中的目录。

验证安装：

```bash
cloud-forge version
```

也可以从源码构建，见 [从源码构建](#从源码构建)。

## AWS 凭证

Cloud Forge CLI 通过 AWS SDK for Go v2 调用 AWS。它内置了浏览器登录流程，因此配置凭证时不需要调用 AWS CLI。

你不需要安装 AWS CLI，但需要有 AWS 身份或 Access Key。

通过浏览器登录 AWS：

```bash
cloud-forge auth aws
```

默认情况下，`cloud-forge auth aws` 会打开 AWS 登录页面。授权完成后，它会写入本地 profile，并保存临时凭证。这个流程由 Cloud Forge 内部通过 AWS Sign-In OAuth + PKCE 完成，不依赖 AWS CLI。

如果浏览器没有自动打开，CLI 会输出登录 URL，你可以复制到浏览器里打开。需要只打印 URL 并手动粘贴授权码时，可以使用 `--no-browser`。

如果设置了 `AWS_PROFILE`，授权命令默认使用该 profile。也可以用 `--profile NAME` 检查或写入指定 profile。

查看当前授权状态：

```bash
cloud-forge auth aws status
```

状态输出会显示 AWS 账号、ARN、区域、profile，以及 AWS SDK 最终选中的凭证来源。

支持的凭证来源包括：

- `~/.aws/credentials`
- `~/.aws/config`
- `AWS_ACCESS_KEY_ID` 和 `AWS_SECRET_ACCESS_KEY`
- `AWS_PROFILE`
- AWS SSO 或 assume-role profile（由 AWS SDK 支持）
- EC2/ECS instance 或 task role

使用 AWS profile：

```bash
export AWS_PROFILE=default
```

使用环境变量：

```bash
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

AWS 部署默认使用 `us-east-1`。需要时可以指定其他区域：

```bash
cloud-forge deploy hello-nginx --cloud aws --region us-west-2
```

生产环境建议使用最小权限的 IAM user 或 role，避免使用 AWS 根账号凭证。

浏览器登录是默认方式，下面的显式写法等价：

```bash
cloud-forge auth aws --method browser
```

只打印登录 URL，不自动打开浏览器：

```bash
cloud-forge auth aws --method browser --no-browser
```

手动配置 Access Key：

```bash
cloud-forge auth aws --method access-key
```

## 快速开始

查找应用：

```bash
cloud-forge search nginx --cloud aws
```

查看应用详情：

```bash
cloud-forge show hello-nginx
```

预览 AWS 模板：

```bash
cloud-forge template hello-nginx --cloud aws
```

只校验部署，不创建资源：

```bash
cloud-forge deploy hello-nginx --cloud aws --dry-run
```

部署到 AWS：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32
```

部署过程中，CLI 会打印 CloudFormation 进度：

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

栈创建完成后，CLI 会打印模板输出和清理命令：

```text
To remove later: cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

## 清理资源

删除 Cloud Forge 创建的栈：

```bash
cloud-forge delete cloud-forge-hello-nginx --cloud aws --region us-east-1
```

等待删除完成（默认行为）：

```bash
cloud-forge delete cloud-forge-gitea --cloud aws --wait
```

发起删除后立即返回：

```bash
cloud-forge delete cloud-forge-n8n --cloud aws --no-wait
```

删除栈会清理模板创建的 EC2 实例、Elastic IP、安全组和相关资源。

可复用的本地私钥会保留在本机：

```text
~/.cloud-forge/keys/aws/cloud-forge-default.pem
```

## SSH Key 行为

默认情况下，AWS 部署会使用一个可复用的本地 SSH key：

```text
~/.cloud-forge/keys/aws/cloud-forge-default.pem
```

首次使用时，CLI 会创建这个私钥并设置 `0600` 权限。如果目标 AWS 区域中还没有 `cloud-forge-default` 密钥对，CLI 会把对应公钥导入 EC2。同一个本地私钥会跨区域复用。

改用已有的 EC2 密钥对：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --key-name my-key
```

禁用 SSH key 注入：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --ssh-key none
```

使用自定义私钥路径：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --ssh-key-path ~/.cloud-forge/keys/aws/custom.pem
```

## 常用选项

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --region us-east-1 \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32 \
  --progress plain
```

关闭进度输出：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --progress none
```

模板参数可以用专用参数传入，也可以重复使用 `--param`：

```bash
cloud-forge deploy gitea --cloud aws \
  --region us-east-1 \
  --param KeyName=my-key \
  --param ImageId=ami-0123456789abcdef0
```

当前支持的 AWS 专用参数包括 `--instance-type`、`--key`、`--key-name`、`--ssh-key`、`--ssh-key-path`、`--progress`、`--domain`、`--hosted-zone-id`、`--disk-size`、`--vpc`、`--subnet`、`--allowed-ip`、`--image-id`、`--latest-ami-id` 和 `--caddy-tls-mode`。

## 应用目录来源

默认情况下，CLI 从这里读取应用目录索引：

```text
https://cdn.jsdelivr.net/gh/CoreNovaLabs/cloud-forge-catalog@main/index/apps.json
```

如果默认镜像不可用，CLI 会回退到 GitHub raw 地址。

本地开发时可以改用自己的应用目录：

```bash
export CLOUD_FORGE_STORE_URL="file:///absolute/path/to/cloud-forge-catalog/index/apps.json"
```

## 命令参考

```bash
cloud-forge search hello --cloud aws
cloud-forge auth aws status
cloud-forge show hello-nginx
cloud-forge template hello-nginx --cloud aws
cloud-forge deploy hello-nginx --cloud aws --dry-run
cloud-forge delete cloud-forge-hello-nginx --cloud aws
cloud-forge help deploy
```

Aliyun 模板可以通过 `search` 和 `template` 查看，但 `deploy` 和 `delete` 当前只支持 AWS。

## AWS 部署

AWS 部署使用 AWS SDK for Go v2 和 CloudFormation，不会在底层调用 AWS CLI。

凭证会从标准 AWS SDK 凭证查找链加载。AWS 部署默认使用 `us-east-1`；需要时可以用 `--region` 覆盖。

```bash
export AWS_PROFILE=default
```

只校验模板，不创建资源：

```bash
cloud-forge deploy hello-nginx --cloud aws --dry-run
```

创建或更新栈：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --stack-name cloud-forge-hello-nginx \
  --instance-type t3.micro \
  --allowed-ip 1.2.3.4/32
```

默认情况下，等待部署完成时会打印 CloudFormation 资源事件：

```text
[12:01:08] AWS::EC2::SecurityGroup HelloSecurityGroup CREATE_COMPLETE
[12:01:15] AWS::EC2::Instance HelloInstance CREATE_IN_PROGRESS
```

关闭进度输出：

```bash
cloud-forge deploy hello-nginx --cloud aws \
  --progress none
```

## 使用统计

默认情况下，CLI 会发送匿名、非阻塞的使用事件到：

```text
https://telemetry.corenovacloud.com/v1/events
```

使用统计不会包含云凭证、账号 ID、域名、本地路径或模板参数值。

需要时可以关闭使用统计：

```bash
export CLOUD_FORGE_TELEMETRY=0
```

本地测试时可以改用其他地址：

```bash
export CLOUD_FORGE_TELEMETRY_ENDPOINT="http://127.0.0.1:8787/v1/events"
```

## 从源码构建

```bash
go build ./cmd/cloud-forge
```

## 开发

```bash
go test ./...
```
