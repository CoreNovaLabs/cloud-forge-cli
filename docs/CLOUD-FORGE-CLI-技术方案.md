# Cloud Forge CLI 技术方案文档

**版本**: v1.2  
**日期**: 2026-06  
**状态**: MVP 规划（Go + AWS SDK v2 + 阿里云 SDK）

---

## 目录

1. [项目概述](#1-项目概述)
2. [核心价值主张](#2-核心价值主张)
3. [系统架构](#3-系统架构)
4. [核心组件设计](#4-核心组件设计)
5. [模板商店设计](#5-模板商店设计)
6. [CLI 设计（Go + 多云 SDK）](#6-cli-设计go--多云-sdk)
7. [云平台 SDK 集成设计](#7-云平台-sdk-集成设计)
8. [IaC 模板设计（CFN / ROS）](#8-iac-模板设计cfn--ros)
9. [云平台支持矩阵](#9-云平台支持矩阵)
10. [商业模式](#10-商业模式)
11. [竞品分析](#11-竞品分析)
12. [实施路线图](#12-实施路线图)
13. [风险与应对](#13-风险与应对)
14. [附录](#附录)

---

## 1. 项目概述

### 1.1 产品定位

**Cloud Forge CLI** — 基于不可变 AMI 的一键部署平台，让开源应用像安装 App 一样简单。

```
用户视角：cloud-forge deploy gitea --cloud aws → 5 分钟拥有自己的 Gitea 服务器
         cloud-forge deploy gitea --cloud aliyun --region cn-hangzhou → 同上
技术视角：Go CLI + AWS SDK v2 / 阿里云 SDK + CFN/ROS 模板 + 收费镜像 = 自动化 IaC 部署
商业视角：镜像收费 + IaC 模板免费 + 模板商店 = 可持续商业模式
```

**MVP 技术栈**：CLI 使用 **Go 1.22+** 开发，**AWS** 交互通过 **AWS SDK for Go v2** 调用 CloudFormation，**阿里云** 交互通过 **阿里云 Go SDK** 调用 ROS（资源编排），均不依赖各云厂商 CLI 子进程。

### 1.2 解决的问题

| 痛点 | 现状 | Cloud Forge CLI 方案 |
|------|------|----------------------|
| 部署复杂 | 手动配置服务器、安装软件、配置域名 | CLI 一行命令，全自动 |
| 启动慢 | Docker 拉镜像 30 秒~2 分钟 | AMI 秒级启动（10~30 秒） |
| 安全性差 | Docker 镜像可能被篡改 | AMI 不可变 + 签名验证 |
| 成本高 | Coolify 需要一直运行的服务器 | 按需部署，不用不花钱 |
| 模板分散 | 各自写 Docker Compose | 统一 CFN/ROS 模板商店 |

### 1.3 核心指标（MVP 目标）

| 指标 | 目标 | 时间 |
|------|------|------|
| 部署时间 | < 5 分钟（从 CLI 到可访问） | MVP |
| 模板数量 | 10 个热门开源应用 | 3 个月 |
| 付费用户 | 100 个 | 6 个月 |
| MRR | $23,000/月 | 12 个月 |

### 1.4 技术选型

| 层级 | 选型 | 说明 |
|------|------|------|
| CLI 语言 | **Go 1.22+** | 单二进制分发，跨平台 |
| CLI 框架 | **Cobra + Viper** | 子命令、Flag、`--cloud` 云厂商选择 |
| AWS 客户端 | **AWS SDK for Go v2** | CloudFormation / EC2 / STS，不 shell 到 `aws` CLI |
| 阿里云客户端 | **阿里云 Go SDK（Darabonba）** | ROS / ECS / STS，不 shell 到 `aliyun` CLI |
| 模板渲染 | **text/template** | 标准库，按云注入 AMI/镜像 ID 与参数 |
| 模板验证 | SDK 线上校验 + 可选 cfn-lint | AWS 用 `ValidateTemplate`；阿里云用 ROS `ValidateTemplate` |
| 构建 | **GoReleaser** | 多平台交叉编译与发布 |

---

## 2. 核心价值主张

```
┌─────────────────────────────────────────────────────────┐
│                  Cloud Forge CLI 价值三角                  │
├─────────────────────────────────────────────────────────┤
│                                                           │
│                    ⚡ 极速部署                             │
│                   （AMI 秒级启动）                         │
│                         △                                 │
│                        / \                                │
│                       /   \                               │
│                      /     \                              │
│                     /       \                             │
│                    /         \                            │
│                   /           \                           │
│                  /             \                          │
│                 /               \                         │
│                /                 \                        │
│               /                   \                       │
│              /                     \                      │
│             /                       \                     │
│            /                         \                    │
│           /                           \                   │
│          /                             \                  │
│         /                               \                 │
│        /                                 \                │
│       /                                   \               │
│      /                                     \              │
│     /                                       \             │
│    /                                         \            │
│   /                                           \           │
│  /                                             \          │
│ /                                               \         │
│🛡️ 安全不可变 ────────────────────────────────── 💰 可持续   │
│  （AMI 签名验证）                                （AMI 收费）│
│                                                           │
└─────────────────────────────────────────────────────────┘
```

| 维度 | 说明 | 优势 |
|------|------|------|
| ⚡ 极速部署 | AMI 预装一切，启动即用 | 比 Docker 快 10 倍 |
| 🛡️ 安全不可变 | AMI 签名验证，防篡改 | 比 Docker 安全 100 倍 |
| 💰 可持续 | 镜像收费，IaC 模板免费 | 比 Coolify 商业模式更优 |

---

## 3. 系统架构

### 3.1 总体架构

```
┌──────────────────────────────────────────────────────────────┐
│                        用户层                                │
│   Cloud Forge CLI（Go 二进制）/ Web UI / API                 │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                     模板商店层（apps.json）                    │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │  Gitea  │  │   n8n   │  │ Uptime  │  │ NocoDB  │  ...   │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘        │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│              模板渲染层（Go text/template）                    │
│  CFN / ROS 模板 + 参数注入 → 最终 IaC 文件                    │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│              云平台 SDK 层（MVP：AWS + 阿里云）                 │
│  ┌─────────────────────┐  ┌─────────────────────────────┐  │
│  │ AWS SDK for Go v2   │  │ 阿里云 Go SDK（Darabonba）    │  │
│  │ CFN / EC2 / STS     │  │ ROS / ECS / STS              │  │
│  └─────────────────────┘  └─────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│              云适配层（pkg/cloud.Deployer 接口）              │
│  AWSDeployer │ AliyunDeployer │ （Phase 2+ 华为云/腾讯云）    │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                    后续扩展（Phase 2+）                        │
│  华为云 ROS │ 腾讯云 TIC │ GCP DM │ Azure Bicep              │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│                    基础设施层（镜像 / AMI）                      │
│  AWS AMI          │  阿里云自定义镜像（收费）                   │
│  Gitea $5/月      │  Gitea ¥35/月                           │
└──────────────────────────────────────────────────────────────┘
```

### 3.2 核心数据流

```
用户输入: cloud-forge deploy gitea --cloud aws --domain git.example.com
      或: cloud-forge deploy gitea --cloud aliyun --region cn-hangzhou --domain git.example.cn

     │
     ▼
┌─────────────────┐
│ 1. 加载模板      │ ← apps.json 找到 gitea，按 --cloud 选 CFN/ROS
│    apps.json    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 渲染 IaC     │ ← 注入镜像 ID、域名、实例规格等参数
│  aws/gitea.yaml │    aliyun/gitea.json
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. 模板校验      │ ← AWS: ValidateTemplate (SDK v2)
│  云平台 SDK      │    阿里云: ValidateTemplate (ROS SDK)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. Change Set   │ ← 两家均支持变更集预览（免费）
│    Preview      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 执行部署      │ ← AWS: CreateStack + Waiter
│  Deployer 接口  │    阿里云: CreateStack + GetStack 轮询
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 6. 输出结果      │ → https://git.example.com 🎉
│    URL + Stack  │
└─────────────────┘
```

---

## 4. 核心组件设计

### 4.1 组件清单

| 组件 | 技术栈 | 职责 | 代码量 |
|------|--------|------|--------|
| CLI | Go + Cobra + Viper | 用户入口，`--cloud` 路由到对应 Deployer | ~550 行 |
| 模板商店 | JSON + Go | apps.json 解析，多云镜像/模板路径 | ~250 行 |
| 模板渲染 | Go text/template | CFN / ROS 模板 + 参数注入 | ~350 行 |
| AWS 客户端 | AWS SDK for Go v2 | CFN / EC2 / STS API 封装 | ~600 行 |
| 阿里云客户端 | 阿里云 Go SDK | ROS / ECS / STS API 封装 | ~600 行 |
| 云适配器 | `Deployer` 接口 | AWS + 阿里云双实现，统一 validate/deploy/list | ~450 行 |
| 镜像工厂 | Packer + Ansible | AWS AMI + 阿里云自定义镜像打包 | ~500 行 |

**总计：~3,300 行 Go 代码 = MVP（AWS + 阿里云双云）**

### 4.2 项目结构

```
cloud-forge-cli/
├── cmd/
│   └── cloud-forge/
│       └── main.go              # Cobra root，注册 AWS / 阿里云 Deployer
├── internal/
│   ├── cmd/                     # 子命令（共用，内部按 --cloud 分发）
│   │   ├── deploy.go
│   │   ├── search.go
│   │   ├── show.go
│   │   ├── validate.go
│   │   ├── preview.go
│   │   ├── list.go
│   │   └── destroy.go
│   ├── store/
│   │   └── catalog.go           # 加载 apps.json，解析多云模板路径
│   ├── template/
│   │   └── renderer.go          # Go template 渲染 CFN / ROS
│   ├── aws/                     # AWS SDK for Go v2 封装
│   │   ├── client.go
│   │   ├── cloudformation.go
│   │   ├── ec2.go
│   │   └── sts.go
│   └── aliyun/                  # 阿里云 Go SDK 封装
│       ├── client.go            # OpenAPI Client 工厂，Region 配置
│       ├── ros.go               # CreateStack / ChangeSet / GetStack
│       ├── ecs.go               # DescribeImages / DescribeInstances
│       └── sts.go               # GetCallerIdentity，账号校验
├── pkg/
│   └── cloud/
│       ├── deployer.go          # Deployer 接口定义
│       ├── aws_deployer.go      # AWS 实现
│       └── aliyun_deployer.go   # 阿里云实现
├── templates/
│   ├── aws/                     # CloudFormation 模板
│   │   ├── gitea.yaml
│   │   ├── n8n.yaml
│   │   └── ...
│   └── aliyun/                  # ROS 模板（JSON/YAML）
│       ├── gitea.json
│       ├── n8n.json
│       └── ...
├── apps.json
├── packer/
│   ├── aws/gitea.pkr.hcl
│   └── aliyun/gitea.pkr.hcl
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 4.3 核心 Go 依赖（go.mod）

```go
require (
    // AWS
    github.com/aws/aws-sdk-go-v2 v1.32.0
    github.com/aws/aws-sdk-go-v2/config v1.28.0
    github.com/aws/aws-sdk-go-v2/service/cloudformation v1.56.0
    github.com/aws/aws-sdk-go-v2/service/ec2 v1.189.0
    github.com/aws/aws-sdk-go-v2/service/sts v1.33.0
    // 阿里云（Darabonba OpenAPI）
    github.com/alibabacloud-go/darabonba-openapi/v2 v2.0.10
    github.com/alibabacloud-go/ros-20190910/v3 v3.3.0
    github.com/alibabacloud-go/ecs-20140526/v4 v4.26.0
    github.com/alibabacloud-go/tea v1.2.2
    // CLI
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.19.0
)
```

### 4.4 Deployer 接口（多云统一入口）

```go
// pkg/cloud/deployer.go
type Deployer interface {
    Provider() string                                          // "aws" | "aliyun"
    ValidateCredentials(ctx context.Context) (AccountInfo, error)
    ValidateTemplate(ctx context.Context, body string) error
    PreviewChanges(ctx context.Context, input PreviewInput) (*ChangeSet, error)
    Deploy(ctx context.Context, input DeployInput) (*DeployOutput, error)
    ListStacks(ctx context.Context, filter ListFilter) ([]StackSummary, error)
    DestroyStack(ctx context.Context, stackName string) error
}
```

---

## 5. 模板商店设计

> **独立仓库**：模板商店维护在 [`cloud-forge-catalog`](../../cloud-forge-catalog/)（与 CLI 同级目录），CLI 通过 `pkg/store` 拉取 `index/apps.json`，按需下载模板文件。

### 5.0 仓库结构（cloud-forge-catalog）

```
cloud-forge-catalog/
├── index/apps.json              # CLI 拉取的聚合索引（make index 生成）
├── apps/<id>/manifest.json      # 单应用元数据（贡献时编辑）
├── apps/<id>/templates/         # CFN / ROS 模板
├── schema/app-v1.schema.json    # manifest JSON Schema
└── scripts/build-index.sh       # 从 manifest 生成索引 + sha256
```

**本地开发对接：**

```yaml
# ~/.cloud-forge/config.yaml
store:
  url: file:///path/to/cloud-forge-catalog/index/apps.json
  cache_ttl: 24h
```

**生产环境：**

```yaml
store:
  url: https://raw.githubusercontent.com/CoreNovaLabs/cloud-forge-catalog/main/index/apps.json
```

### 5.1 apps.json（核心资产）

> 当前实现以 `cloud-forge-catalog/schema/app-v1.schema.json`、`cloud-forge-catalog/index/apps.json` 与 CLI 的 `pkg/store/types.go` 为准。下面示例用于说明字段语义，实际索引使用 `catalog_version`、`base_url`、`images`、`templates.<cloud>.path/url/checksum` 等结构化字段。

```json
{
  "version": "2.0.0",
  "store": {
    "name": "Cloud Forge App Store",
    "description": "一键部署开源应用，基于不可变 AMI"
  },
  "apps": [
    {
      "id": "gitea",
      "name": "Gitea",
      "desc": "轻量 GitHub 替代，5 分钟部署",
      "icon": "🐦",
      "category": "devtools",
      "stars": 45000,
      "ami": {
        "aws": "ami-0a1b2c3d4e5f6g7h8",
        "aliyun": "m-0a1b2c3d4e5f6g7h8",
        "price": "$5/month"
      },
      "templates": {
        "aws": "templates/aws/gitea.yaml",
        "aliyun": "templates/aliyun/gitea.json"
      },
      "params": {
        "InstanceType": {
          "aws": { "default": "t3.medium", "options": ["t3.micro", "t3.small", "t3.medium", "t3.large"] },
          "aliyun": { "default": "ecs.c6.large", "options": ["ecs.t6-c1m1.large", "ecs.c6.large"] }
        },
        "DomainName": {
          "type": "string",
          "default": ""
        },
        "KeyName": { "type": "string" },
        "AdminPassword": { "type": "string", "secret": true }
      }
    },
    {
      "id": "n8n",
      "name": "n8n",
      "desc": "开源自动化工作流，Zapier 替代",
      "icon": "⚡",
      "category": "automation",
      "stars": 38000,
      "ami": {
        "aws": "ami-0i9j8k7l6m5n4o3p2",
        "aliyun": "m-0i9j8k7l6m5n4o3p2",
        "price": "$8/month"
      },
      "templates": {
        "aws": "templates/aws/n8n.yaml",
        "aliyun": "templates/aliyun/n8n.json"
      },
      "params": {
        "InstanceType": {
          "default": "t3.large",
          "options": ["t3.medium", "t3.large", "t3.xlarge"]
        },
        "DomainName": { "type": "string" },
        "WebhookURL": { "type": "string" }
      }
    },
    {
      "id": "uptime-kuma",
      "name": "Uptime Kuma",
      "desc": "优雅的监控工具",
      "icon": "🟢",
      "category": "monitoring",
      "stars": 28000,
      "ami": {
        "aws": "ami-0q1w2e3r4t5y6u7i8",
        "aliyun": "m-0q1w2e3r4t5y6u7i8",
        "price": "$3/month"
      },
      "templates": {
        "aws": "templates/aws/uptime-kuma.yaml",
        "aliyun": "templates/aliyun/uptime-kuma.json"
      },
      "params": {
        "InstanceType": {
          "default": "t3.micro",
          "options": ["t3.micro", "t3.small"]
        },
        "DomainName": { "type": "string" },
        "AlertEmail": { "type": "string" }
      }
    }
  ]
}
```

### 5.2 模板商店特性

| 特性 | 说明 |
|------|------|
| ✅ 开源 | apps.json 开源，社区可贡献 |
| ✅ 版本化 | version 字段，支持升级 |
| ✅ 分类 | category 字段，按类型筛选 |
| ✅ 评分 | stars 字段，社区投票 |
| ✅ 价格透明 | price 字段，明码标价 |
| ✅ 多云镜像 | 每个应用支持 AWS AMI + 阿里云自定义镜像 ID |
### 5.3 CLI Store 接口（pkg/store）

```go
type Store interface {
    Sync(ctx context.Context) error
    List(filter Filter) ([]App, error)
    Get(appID string) (*App, error)
    GetTemplate(ctx context.Context, appID, cloud string) (string, error)
}
```

- **Sync**：HTTP GET 或 `file://` 拉取 `index/apps.json`，缓存至 `~/.cloud-forge/cache/`
- **List / Get**：内存检索（query / category / cloud / tags）
- **GetTemplate**：按 `templates.<cloud>.url` 下载，校验 `checksum`，缓存至本地

---

## 6. CLI 设计（Go + 多云 SDK）

### 6.1 实现原则

| 原则 | 说明 |
|------|------|
| 纯 Go 二进制 | 不依赖 Python/Node/各云 CLI，用户只需安装 `cloud-forge` |
| SDK 直连云平台 | AWS 走 SDK v2；阿里云走 Darabonba Go SDK，均不 fork 子进程 |
| `--cloud` 统一入口 | 所有命令通过 `--cloud aws\|aliyun` 选择 Deployer，默认 `aws` |
| 接口抽象 | `pkg/cloud.Deployer` 隔离云差异，命令层无云厂商分支 |
| 上下文传递 | 所有 SDK 调用携带 `context.Context`，支持超时与取消 |
| 错误友好 | 分别包装 AWS `smithy.OperationError` 与阿里云 Tea 异常，输出可读建议 |

### 6.2 命令清单

| 命令 | 功能 | 示例 |
|------|------|------|
| `cloud-forge search` | 搜索模板商店 | `cloud-forge search --category devtools` |
| `cloud-forge show <app>` | 显示应用详情 | `cloud-forge show gitea` |
| `cloud-forge validate <app>` | 验证模板 | `cloud-forge validate gitea --cloud aliyun --region cn-hangzhou` |
| `cloud-forge preview <app>` | Change Set 预览 | `cloud-forge preview gitea --cloud aws --domain git.example.com` |
| `cloud-forge deploy <app>` | 一键部署 | `cloud-forge deploy gitea --cloud aliyun --region cn-hangzhou` |
| `cloud-forge list` | 列出已部署 | `cloud-forge list --cloud aws --region us-east-1` |
| `cloud-forge destroy <stack>` | 销毁栈 | `cloud-forge destroy cloud-forge-gitea-xxx --cloud aliyun` |
| `cloud-forge update` | 升级 CLI | `cloud-forge self-update` |

### 6.3 命令与 SDK 映射

| 命令 | AWS（SDK v2） | 阿里云（ROS SDK） |
|------|---------------|-------------------|
| 启动校验 | `sts.GetCallerIdentity` | STS `GetCallerIdentity` |
| `validate` | `cloudformation.ValidateTemplate` | `ros.ValidateTemplate` |
| `preview` | `CreateChangeSet` → `DescribeChangeSet` | `CreateChangeSet` → `GetChangeSet` |
| `deploy` | `CreateStack` → `WaitUntilStackCreateComplete` | `CreateStack` → `GetStack` 轮询 |
| `list` | `DescribeStacks`（Tag 过滤） | `ListStacks`（StackName 前缀过滤） |
| `destroy` | `DeleteStack` → Waiter | `DeleteStack` → 轮询至 `DELETE_COMPLETE` |

### 6.4 部署命令详解

**AWS：**

```bash
cloud-forge deploy gitea --cloud aws \
  --region us-east-1 \
  --instance-type t3.medium \
  --key my-key \
  --admin-password MyP@ss123 \
  --domain gitea.example.com \
  --hosted-zone-id Z3XXX \
  --disk-size 50 \
  --vpc vpc-12345 \
  --allowed-ip 1.2.3.4/32
```

凭证：`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_PROFILE` / `~/.aws/credentials`

**阿里云：**

```bash
cloud-forge deploy gitea --cloud aliyun \
  --region cn-hangzhou \
  --instance-type ecs.c6.large \
  --key-pair my-key \
  --admin-password MyP@ss123 \
  --domain gitea.example.cn \
  --disk-size 50 \
  --vpc vpc-bp1xxxxx \
  --vswitch vsw-bp1xxxxx \
  --allowed-ip 1.2.3.4/32
```

凭证：`ALIBABA_CLOUD_ACCESS_KEY_ID` / `ALIBABA_CLOUD_ACCESS_KEY_SECRET`，或 `~/.alibabacloud/credentials`

### 6.5 输出示例

**AWS：**

```
$ cloud-forge deploy gitea --cloud aws --domain git.example.com

📋 deploying gitea on aws (us-east-1)...
✅ Template validated (CloudFormation)
✅ Change Set created: cloud-forge-gitea-20240101
   + AWS::EC2::Instance
   + AWS::EC2::SecurityGroup
   + AWS::EC2::EIP

🚀 Stack created: cloud-forge-gitea-20240101
📍 URL: https://git.example.com
💰 Billing: $5/month (AMI) + $0.042/hour (EC2)
```

**阿里云：**

```
$ cloud-forge deploy gitea --cloud aliyun --region cn-hangzhou --domain git.example.cn

📋 deploying gitea on aliyun (cn-hangzhou)...
✅ Template validated (ROS)
✅ Change Set created: cloud-forge-gitea-20240101
   + ALIYUN::ECS::Instance
   + ALIYUN::ECS::SecurityGroup
   + ALIYUN::ECS::EIP

🚀 Stack created: cloud-forge-gitea-20240101
📍 URL: https://git.example.cn
💰 Billing: ¥35/month (镜像) + ¥0.28/小时 (ECS)
```

### 6.6 deploy 核心代码结构（示意）

```go
// internal/aws/cloudformation.go
func (c *Client) DeployStack(ctx context.Context, input DeployInput) (*DeployOutput, error) {
    _, err := c.cfn.ValidateTemplate(ctx, &cloudformation.ValidateTemplateInput{
        TemplateBody: aws.String(input.TemplateBody),
    })
    if err != nil {
        return nil, fmt.Errorf("validate template: %w", err)
    }

    _, err = c.cfn.CreateStack(ctx, &cloudformation.CreateStackInput{
        StackName:    aws.String(input.StackName),
        TemplateBody: aws.String(input.TemplateBody),
        Parameters:   input.Parameters,
        Capabilities: []types.Capability{types.CapabilityCapabilityNamedIam},
        Tags:         cloudForgeTags(input.AppID),
    })
    if err != nil {
        return nil, fmt.Errorf("create stack: %w", err)
    }

    waiter := cloudformation.NewStackCreateCompleteWaiter(c.cfn)
    if err := waiter.Wait(ctx, &cloudformation.DescribeStacksInput{
        StackName: aws.String(input.StackName),
    }, 15*time.Minute); err != nil {
        return nil, fmt.Errorf("wait stack: %w", err)
    }

    return c.readStackOutputs(ctx, input.StackName)
}
```

```go
// internal/aliyun/ros.go
func (c *Client) DeployStack(ctx context.Context, input DeployInput) (*DeployOutput, error) {
    _, err := c.ros.ValidateTemplate(&ros.ValidateTemplateRequest{
        TemplateBody: tea.String(input.TemplateBody),
    })
    if err != nil {
        return nil, fmt.Errorf("validate template: %w", err)
    }

    _, err = c.ros.CreateStack(&ros.CreateStackRequest{
        StackName:    tea.String(input.StackName),
        TemplateBody: tea.String(input.TemplateBody),
        Parameters:   input.Parameters,
        Tags:         cloudForgeTags(input.AppID),
    })
    if err != nil {
        return nil, fmt.Errorf("create stack: %w", err)
    }

    if err := c.waitStackComplete(ctx, input.StackName, "CREATE_COMPLETE", 15*time.Minute); err != nil {
        return nil, err
    }

    return c.readStackOutputs(ctx, input.StackName)
}
```

---

## 7. 云平台 SDK 集成设计

### 7.1 AWS SDK for Go v2

#### 客户端初始化

```go
// internal/aws/client.go
func NewClient(ctx context.Context, region string) (*Client, error) {
    cfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion(region),
    )
    if err != nil {
        return nil, err
    }

    return &Client{
        cfg: cfg,
        cfn: cloudformation.NewFromConfig(cfg),
        ec2: ec2.NewFromConfig(cfg),
        sts: sts.NewFromConfig(cfg),
    }, nil
}
```

### 7.2 AWS 使用的服务

| 服务 | SDK 包 | 用途 |
|------|--------|------|
| CloudFormation | `service/cloudformation` | 模板校验、Change Set、Stack 生命周期 |
| EC2 | `service/ec2` | 校验 AMI 是否存在、查询实例状态 |
| STS | `service/sts` | 启动时确认当前 AWS 账号与凭证 |

#### AWS 凭证与权限

CLI 启动时调用 `sts.GetCallerIdentity` 验证凭证。部署所需 IAM 权限最小集：

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cloudformation:ValidateTemplate",
        "cloudformation:CreateStack",
        "cloudformation:CreateChangeSet",
        "cloudformation:ExecuteChangeSet",
        "cloudformation:DescribeStacks",
        "cloudformation:DescribeStackEvents",
        "cloudformation:DeleteStack",
        "ec2:DescribeImages",
        "ec2:DescribeInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

Stack 内 EC2、SecurityGroup、EIP 等资源由 CloudFormation **服务角色**创建，用户 IAM 只需 Stack 级权限。

### 7.3 阿里云 Go SDK（ROS）

#### 客户端初始化

```go
// internal/aliyun/client.go
func NewClient(region string) (*Client, error) {
    cfg := &openapi.Config{
        AccessKeyId:     tea.String(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")),
        AccessKeySecret: tea.String(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")),
        RegionId:        tea.String(region),
        Endpoint:        tea.String("ros.aliyuncs.com"),
    }

    rosClient, err := ros20190910.NewClient(cfg)
    if err != nil {
        return nil, err
    }

    ecsCfg := *cfg
    ecsCfg.Endpoint = tea.String("ecs.aliyuncs.com")
    ecsClient, err := ecs20140526.NewClient(&ecsCfg)
    if err != nil {
        return nil, err
    }

    return &Client{ros: rosClient, ecs: ecsClient}, nil
}
```

#### 阿里云使用的服务

| 服务 | SDK 包 | 用途 |
|------|--------|------|
| ROS（资源编排） | `ros-20190910` | 模板校验、Change Set、Stack 生命周期（对标 CloudFormation） |
| ECS | `ecs-20140526` | 校验自定义镜像、查询实例状态 |
| STS | `sts-20150401` | 启动时确认 AccessKey 与账号 UID |

#### 阿里云 RAM 权限最小集

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ros:ValidateTemplate",
        "ros:CreateStack",
        "ros:CreateChangeSet",
        "ros:ExecuteChangeSet",
        "ros:GetStack",
        "ros:ListStacks",
        "ros:DeleteStack",
        "ros:GetChangeSet",
        "ecs:DescribeImages",
        "ecs:DescribeInstances"
      ],
      "Resource": "*"
    }
  ]
}
```

Stack 内 ECS、SecurityGroup、EIP 等资源由 ROS **服务角色（RamRole）** 创建，用户 RAM 只需 Stack 级权限。

### 7.4 AWS 与阿里云 API 对照

| 能力 | AWS SDK v2 | 阿里云 ROS SDK |
|------|------------|----------------|
| 模板校验 | `ValidateTemplate` | `ValidateTemplate` |
| 创建栈 | `CreateStack` | `CreateStack` |
| 变更集预览 | `CreateChangeSet` | `CreateChangeSet` |
| 执行变更集 | `ExecuteChangeSet` | `ExecuteChangeSet` |
| 查询栈 | `DescribeStacks` | `GetStack` |
| 等待完成 | SDK Waiter | 轮询 `GetStack.Status` |
| 删除栈 | `DeleteStack` | `DeleteStack` |

### 7.5 错误处理与重试

**AWS：**
- 使用 SDK 内置 `Retryer`，对 `Throttling` 自动退避
- `WaitUntilStackCreateComplete` 超时 15 分钟，失败时输出 `DescribeStackEvents` 最近 10 条

**阿里云：**
- 使用 Tea SDK 内置重试，对 `Throttling.User` / `ServiceUnavailable` 退避
- `GetStack` 轮询间隔 10s，超时 15 分钟，失败时输出 `GetStackEvents` 最近 10 条

**通用：**
- 禁止在日志中打印 `AdminPassword` 等敏感参数

---

## 8. IaC 模板设计（CFN / ROS）

### 8.1 设计原则

| 原则 | 说明 |
|------|------|
| ✅ 参数化 | 所有配置都通过 Parameters 传入 |
| ✅ 条件化 | 用 Conditions 控制可选资源（如域名） |
| ✅ 模块化 | 每个应用一个模板，互不干扰 |
| ✅ 可验证 | 通过 cfn-lint 免费验证 |
| ✅ 多云兼容 | AWS 用 CFN YAML；阿里云用 ROS JSON/YAML，逻辑一一对应 |
| ✅ 目录分离 | `templates/aws/` 与 `templates/aliyun/` 独立维护 |

### 8.2 AWS：Gitea CloudFormation 模板（支持域名）

```yaml
AWSTemplateFormatVersion: '2010-09-09'
Description: 'Gitea on Cloud Forge CLI - 一键部署'

Parameters:
  InstanceType:
    Type: String
    Default: t3.medium
    AllowedValues: [t3.micro, t3.small, t3.medium, t3.large]
  KeyName:
    Type: AWS::EC2::KeyPair::KeyName
  AdminPassword:
    Type: String
    NoEcho: true
  DomainName:
    Type: String
    Default: ""
  DiskSize:
    Type: Number
    Default: 20
    MinValue: 10
    MaxValue: 500
  VpcId:
    Type: AWS::EC2::VPC::Id
    Default: ""
  AllowedIP:
    Type: String
    Default: "0.0.0.0/0"

Conditions:
  HasDomain: !Not [!Equals [!Ref DomainName, ""]]
  HasVpc: !Not [!Equals [!Ref VpcId, ""]]

Resources:
  GiteaInstance:
    Type: AWS::EC2::Instance
    Properties:
      ImageId: ami-0a1b2c3d4e5f6g7h8    # 收费 AMI
      InstanceType: !Ref InstanceType
      KeyName: !Ref KeyName
      BlockDeviceMappings:
        - DeviceName: /dev/xvda
          Ebs:
            VolumeSize: !Ref DiskSize
            VolumeType: gp3
      SecurityGroupIds: [!Ref GiteaSG]
      UserData:
        Fn::Base64: |
          #!/bin/bash
          systemctl start gitea

  GiteaSG:
    Type: AWS::EC2::SecurityGroup
    Properties:
      GroupDescription: Gitea SG
      VpcId: !If [HasVpc, !Ref VpcId, !Ref AWS::NoValue]
      SecurityGroupIngress:
        - IpProtocol: tcp
          FromPort: 22
          ToPort: 22
          CidrIp: !Ref AllowedIP
        - IpProtocol: tcp
          FromPort: 3000
          ToPort: 3000
          CidrIp: !Ref AllowedIP

  GiteaEIP:
    Type: AWS::EC2::EIP
    Properties:
      InstanceId: !Ref GiteaInstance
      Domain: vpc

  GiteaDNS:
    Type: AWS::Route53::RecordSet
    Condition: HasDomain
    Properties:
      HostedZoneId: Z3XXXXXXXXXX
      Name: !Ref DomainName
      Type: A
      AliasTarget:
        DNSName: !GetAtt GiteaEIP.PublicDnsName
        HostedZoneId: !GetAtt GiteaEIP.PublicIp

Outputs:
  GiteaURL:
    Value: !If [HasDomain, !Sub "https://${DomainName}", !Sub "http://${GiteaEIP.PublicIp}:3000"]
  InstanceId:
    Value: !Ref GiteaInstance
```

### 8.3 AWS 参数覆盖表

| 参数 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| InstanceType | enum | t3.medium | ✅ | 实例规格 |
| KeyName | string | - | ✅ | SSH 密钥 |
| AdminPassword | secret | - | ✅ | 管理员密码 |
| DomainName | string | "" | ❌ | 自定义域名 |
| DiskSize | number | 20 | ❌ | 磁盘大小（GB） |
| VpcId | string | "" | ❌ | VPC ID |
| AllowedIP | cidr | 0.0.0.0/0 | ❌ | IP 白名单 |

### 8.4 阿里云：Gitea ROS 模板（支持域名）

```json
{
  "ROSTemplateFormatVersion": "2015-09-01",
  "Description": "Gitea on Cloud Forge CLI - 一键部署（阿里云）",
  "Parameters": {
    "InstanceType": {
      "Type": "String",
      "Default": "ecs.c6.large",
      "AllowedValues": ["ecs.t6-c1m1.large", "ecs.c6.large", "ecs.c6.xlarge"]
    },
    "ImageId": {
      "Type": "String",
      "Default": "m-0a1b2c3d4e5f6g7h8"
    },
    "KeyPairName": { "Type": "String" },
    "AdminPassword": { "Type": "String", "NoEcho": true },
    "DomainName": { "Type": "String", "Default": "" },
    "DiskSize": { "Type": "Number", "Default": 40, "MinValue": 20, "MaxValue": 500 },
    "VpcId": { "Type": "String" },
    "VSwitchId": { "Type": "String" },
    "AllowedIP": { "Type": "String", "Default": "0.0.0.0/0" }
  },
  "Conditions": {
    "HasDomain": { "Fn::Not": [{ "Fn::Equals": [{ "Ref": "DomainName" }, ""] }] }
  },
  "Resources": {
    "GiteaSG": {
      "Type": "ALIYUN::ECS::SecurityGroup",
      "Properties": {
        "VpcId": { "Ref": "VpcId" },
        "SecurityGroupIngress": [
          { "IpProtocol": "tcp", "PortRange": "22/22", "SourceCidrIp": { "Ref": "AllowedIP" } },
          { "IpProtocol": "tcp", "PortRange": "3000/3000", "SourceCidrIp": { "Ref": "AllowedIP" } }
        ]
      }
    },
    "GiteaInstance": {
      "Type": "ALIYUN::ECS::Instance",
      "Properties": {
        "ImageId": { "Ref": "ImageId" },
        "InstanceType": { "Ref": "InstanceType" },
        "KeyPairName": { "Ref": "KeyPairName" },
        "VSwitchId": { "Ref": "VSwitchId" },
        "SecurityGroupId": { "Ref": "GiteaSG" },
        "SystemDiskSize": { "Ref": "DiskSize" },
        "UserData": "#!/bin/bash\nsystemctl start gitea"
      }
    },
    "GiteaEIP": {
      "Type": "ALIYUN::VPC::EIP",
      "Properties": {
        "Bandwidth": 5,
        "InternetChargeType": "PayByTraffic"
      }
    },
    "GiteaEIPAssociation": {
      "Type": "ALIYUN::VPC::EIPAssociation",
      "Properties": {
        "AllocationId": { "Ref": "GiteaEIP" },
        "InstanceId": { "Ref": "GiteaInstance" }
      }
    }
  },
  "Outputs": {
    "GiteaURL": {
      "Value": {
        "Fn::If": [
          "HasDomain",
          { "Fn::Sub": "https://${DomainName}" },
          { "Fn::Join": ["", ["http://", { "Fn::GetAtt": ["GiteaEIP", "EipAddress"] }, ":3000"]] }
        ]
      }
    },
    "InstanceId": { "Value": { "Ref": "GiteaInstance" } }
  }
}
```

### 8.5 阿里云参数覆盖表

| 参数 | 类型 | 默认值 | 必填 | 说明 |
|------|------|--------|------|------|
| InstanceType | enum | ecs.c6.large | ✅ | ECS 规格 |
| ImageId | string | apps.json 注入 | ✅ | 收费自定义镜像 |
| KeyPairName | string | - | ✅ | SSH 密钥对 |
| AdminPassword | secret | - | ✅ | 管理员密码 |
| DomainName | string | "" | ❌ | 自定义域名（配合 DNS 解析） |
| DiskSize | number | 40 | ❌ | 系统盘大小（GB） |
| VpcId | string | - | ✅ | VPC ID |
| VSwitchId | string | - | ✅ | 交换机 ID |
| AllowedIP | cidr | 0.0.0.0/0 | ❌ | IP 白名单 |

---

## 9. 云平台支持矩阵

### 9.1 支持情况总览

| 云平台 | 镜像 | IaC | CLI 集成 | 市场 | Change Set | 优先级 |
|--------|------|-----|----------|------|------------|--------|
| **AWS** | ✅ AMI | CFN | **AWS SDK for Go v2** | ✅ Marketplace | ✅ | 🥇 **P0（MVP）** |
| **阿里云** | ✅ 自定义镜像 | ROS | **阿里云 Go SDK（ros-20190910）** | ✅ 镜像市场 | ✅ | 🥇 **P0（MVP）** |
| 华为云 | ✅ | ROS | Go SDK（Phase 2） | ✅ | ✅ | 🥈 P1 |
| 腾讯云 | ✅ | TIC/ROS | Go SDK（Phase 2） | ✅ | ✅ | 🥈 P1 |
| GCP | ✅ | DM | Go SDK（Phase 3） | ✅ | ⚠️ | 🥉 P2 |
| Azure | ✅ | Bicep | Go SDK（Phase 3） | ✅ | ⚠️ | 🥉 P2 |

> **MVP 范围**：**AWS + 阿里云** 双云并行。AWS 走 CloudFormation + SDK v2；阿里云走 ROS + Darabonba Go SDK。两家 API 语义高度一致，共用 `Deployer` 接口。

### 9.2 AWS 与阿里云 MVP 差异对照

| 维度 | AWS | 阿里云 |
|------|-----|--------|
| IaC 引擎 | CloudFormation | ROS（资源编排服务） |
| Go SDK | `aws-sdk-go-v2` | `alibabacloud-go/ros-20190910` |
| 镜像 | AMI | 自定义镜像（`m-xxx`） |
| 实例 | EC2 | ECS |
| 凭证环境变量 | `AWS_ACCESS_KEY_ID` | `ALIBABA_CLOUD_ACCESS_KEY_ID` |
| 默认 Region | `us-east-1` | `cn-hangzhou` |
| 市场上架 | AWS Marketplace | 阿里云镜像市场 |

### 9.3 后续扩展：ROS 统一模板（华为云/腾讯云，Phase 2）

```hcl
# 同一个模板，三家云通用
terraform {
  required_providers {
    alicloud = { source = "aliyun/alicloud" }
    huaweicloud = { source = "huaweicloud/huaweicloud" }
    tencentcloud = { source = "tencentcloudstack/tencentcloud" }
  }
}

resource "alicloud_instance" "gitea" {
  image_id = var.ami_id
  instance_type = var.instance_type
  # ...
}

# 换成 huaweicloud_instance，同样的逻辑
# 换成 tencentcloud_instance，同样的逻辑
```

**阿里云 MVP 已独立实现 ROS 模板；Phase 2 华为云/腾讯云可复用 90% 的 ROS 逻辑。**

---

## 10. 商业模式

### 10.1 收入来源

| 来源 | 说明 | 占比 |
|------|------|------|
| 💰 AMI 收费 | 核心收入，按月收费 | 90% |
| 💰 高级模板 | 企业级模板（带监控、备份） | 10% |
| 🆓 CFN 模板 | 永久免费 | - |
| 🆓 CLI 工具 | 永久免费（开源 MIT） | - |
| 🆓 模板商店 | 永久免费 | - |

### 10.2 定价策略

| 应用 | AMI 价格 | 目标用户 |
|------|----------|----------|
| Gitea | $5/月 | 开发者 |
| n8n | $8/月 | 自动化爱好者 |
| Uptime Kuma | $3/月 | 运维 |
| NocoDB | $6/月 | 产品经理 |
| Appwrite | $10/月 | 创业者 |
| WordPress | $8/月 | 博主 |

### 10.3 收入预测

| 阶段 | 用户数 | MRR | 说明 |
|------|--------|-----|------|
| MVP（3 个月） | 50 | $2,000 | 10 个模板，50 用户 |
| 成长期（6 个月） | 500 | $20,000 | 30 个模板，500 用户 |
| 成熟期（12 个月） | 2000 | $80,000 | 100 个模板，2000 用户 |

### 10.4 成本结构

| 成本项 | 月成本 | 说明 |
|--------|--------|------|
| AWS 基础设施 | $500 | AMI 打包、测试 |
| 阿里云基础设施 | $300 | ROS 测试 |
| 域名 + CDN | $50 | cloud-forge.io |
| **总计** | **$850/月** | |

| 指标 | 数值 |
|------|------|
| 毛利率 | 98%（$80,000 - $850） |
| 盈亏平衡 | 11 个付费用户 |
| 12 个月 MRR | $80,000 |

---

## 11. 竞品分析

| 维度 | Coolify | CapRover | Cloud Forge CLI | 胜出 |
|------|---------|----------|-----------------|------|
| 部署方式 | Docker Compose | Docker Swarm | AMI 不可变 | ✅ Cloud Forge CLI |
| 启动速度 | 30s~2min | 30s~2min | 10~30s | ✅ Cloud Forge CLI |
| 安全性 | ⚠️ 镜像可篡改 | ⚠️ 镜像可篡改 | ✅ AMI 签名 | ✅ Cloud Forge CLI |
| 模板验证 | ❌ | ❌ | ✅ SDK ValidateTemplate | ✅ Cloud Forge CLI |
| 域名配置 | 手动 Web UI | 手动 Web UI | CLI --domain | ✅ Cloud Forge CLI |
| 商业模式 | 开源免费 | 开源免费 | AMI 收费 | ✅ Cloud Forge CLI |
| 模板商店 | ❌ | ❌ | ✅ apps.json | ✅ Cloud Forge CLI |
| 多云支持 | ⚠️ 有限 | ❌ | ✅ 6 家云 | ✅ Cloud Forge CLI |
| Change Set | ❌ | ❌ | ✅ 免费预览 | ✅ Cloud Forge CLI |

**Cloud Forge CLI 在 9/10 维度完胜，唯一劣势是生态（初期模板少），但这正是模板商店要解决的！**

---

## 12. 实施路线图

### 12.1 阶段规划

```
Phase 1: MVP（0~3 个月）💰 目标：AWS + 阿里云（双云 Go SDK）
├── ✅ Go CLI 框架（Cobra）+ Deployer 接口
├── ✅ AWS SDK v2：ValidateTemplate / CreateStack / ChangeSet
├── ✅ 阿里云 SDK：ROS ValidateTemplate / CreateStack / ChangeSet
├── ✅ 5 组双云模板（Gitea/n8n/Uptime/NocoDB/Appwrite）
├── ✅ apps.json 模板商店（多云镜像 + 模板路径）
├── ✅ Packer 双云镜像打包（AWS AMI + 阿里云自定义镜像）
├── ✅ AWS Marketplace 上架
└── ✅ 阿里云镜像市场上架

Phase 2: 扩展（3~6 个月）💰 目标：华为云 + 腾讯云
├── ✅ ROS 模板（与阿里云 90% 相同）
├── ✅ 华为云 + 腾讯云镜像市场
├── ✅ 模板数量扩展到 30 个
└── ✅ 社区贡献机制

Phase 3: 全球化（6~12 个月）💰 目标：GCP + Azure
├── ✅ Deployment Manager 模板（GCP）
├── ✅ Bicep 模板（Azure）
├── ✅ Cloud Marketplace 上架
├── ✅ 模板数量扩展到 100 个
└── ✅ MRR $80,000
```

### 12.2 MVP 里程碑

| 周 | 任务 | 交付物 |
|----|------|--------|
| Week 1 | Go CLI + Deployer 接口 + AWS/阿里云 Client | `cloud-forge search` 可用，双云凭证校验通过 |
| Week 2 | AWS deploy（SDK CreateStack/Waiter） | `cloud-forge deploy gitea --cloud aws` 可用 |
| Week 3 | 阿里云 deploy（ROS CreateStack/轮询） | `cloud-forge deploy gitea --cloud aliyun` 可用 |
| Week 4 | Change Set 预览 + 5 组双云模板 | validate/preview/deploy 双云完整链路 |
| Week 5~6 | Packer 双云镜像 + 市场上架 | AWS + 阿里云均可收费 |

---

## 13. 风险与应对

| 风险 | 概率 | 影响 | 应对 |
|------|------|------|------|
| ⚠️ AMI 维护成本高 | 中 | 高 | Packer 自动化打包，每周自动更新 |
| ⚠️ 模板商店冷启动 | 高 | 中 | 自己先做 10 个，开源吸引社区 |
| ⚠️ 云 API 变动 | 低 | 中 | `internal/aws` / `internal/aliyun` 封装 SDK，隔离变化 |
| ⚠️ 双云维护成本 | 中 | 中 | Deployer 接口统一命令层；模板逻辑对齐，Packer 流水线复用 |
| ⚠️ 竞品模仿 | 中 | 低 | 先发优势 + 模板商店生态壁垒 |
| ⚠️ AMI 市场审核慢 | 低 | 中 | 提前准备，多云并行 |

---

## 附录

### A. 技术栈总览

| 层级 | 技术 | 原因 |
|------|------|------|
| CLI | **Go 1.22+ + Cobra + Viper** | 单二进制、跨平台、`--cloud` 多云路由 |
| AWS 集成 | **AWS SDK for Go v2** | CFN / EC2 / STS，不依赖 AWS CLI |
| 阿里云集成 | **alibabacloud-go（Darabonba）** | ROS / ECS / STS，不依赖 aliyun CLI |
| AWS IaC | CloudFormation YAML | `templates/aws/` |
| 阿里云 IaC | ROS JSON/YAML | `templates/aliyun/` |
| 模板渲染 | Go `text/template` | 标准库，按云注入镜像 ID |
| 验证 | 各云 SDK `ValidateTemplate` + 可选 cfn-lint | 线上校验准确 |
| 镜像打包 | Packer + Ansible | AWS AMI + 阿里云自定义镜像 |
| 模板商店 | JSON | 多云镜像 + 模板路径 |
| 许可证 | MIT | 最大限度采用 |

### B. 核心代码量估算

| 组件 | 行数 | 占比 |
|------|------|------|
| CLI（Cobra 命令 + --cloud 路由） | 550 | 17% |
| 模板商店 | 250 | 8% |
| 模板渲染 | 350 | 11% |
| AWS SDK 封装 | 600 | 18% |
| 阿里云 SDK 封装 | 600 | 18% |
| Deployer 接口 + 双实现 | 450 | 14% |
| 验证与错误处理 | 200 | 6% |
| 其他（配置/输出） | 300 | 9% |
| **总计** | **~3,300** | **100%** |

**~3,300 行 Go 代码 + AWS SDK v2 + 阿里云 SDK = 双云 MVP，3~4 周可完成！**

---

## 最终结论

| 问题 | 答案 |
|------|------|
| 能做吗？ | ✅ 100% 能，Go + 双云 SDK，~3,300 行，3~4 周双云 MVP |
| 赚钱吗？ | ✅ 镜像收费，毛利率 98%，11 个用户盈亏平衡 |
| 有壁垒吗？ | ✅ 模板商店 + 镜像生态，Coolify 做不到 |
| 先做哪个云？ | ✅ **AWS + 阿里云（P0 并行）**，华为云/腾讯云 Phase 2 |
| 现在开始吗？ | 🚀 今天就初始化 `go mod`，同时写 CFN 和 ROS 模板！ |

**Cloud Forge CLI 不是又一个部署工具，它是「AMI 时代的 App Store」—— 这个赛道目前是空白的，先入场就赢了！**
