# K8S Platform — 多集群管理控制台

[![Go Version](https://img.shields.io/badge/Go-1.25.5-00ADD8?logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react)](https://react.dev/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.20%2B-326CE5?logo=kubernetes)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](./LICENSE)
[![CNCF Style](https://img.shields.io/badge/CNCF-Style-00ADD8?logo=cncf)](#community)

> 一个**插件化、可扩展**的 Kubernetes 多集群管理控制台。导入集群证书即可通过界面化管理多种 K8S 对象，
> 内置**版本历史 / 资源备份与恢复 / 命名空间配额 / Helm 应用商店 / RBAC / 操作审计** 等企业级能力。

---

<p align="center">
  <a href="#features">功能</a> •
  <a href="#architecture">架构</a> •
  <a href="#quick-start">快速开始</a> •
  <a href="#configuration">配置</a> •
  <a href="#deployment">部署</a> •
  <a href="#development">开发指南</a> •
  <a href="#contributing">贡献</a>
</p>

---

## Features

### 🏗️ 多集群管理
- **kubeconfig 证书导入** — 支持上传文件或粘贴内容，AES-256-GCM 加密存储
- **连接性检测** — 集群版本/节点数/状态/最近同步时间
- **全局上下文切换** — 所有资源操作基于当前选中的集群
- **证书更新** — 重新上传证书或修改 API Server 地址

### 📦 资源对象管理
- **工作负载** — Deployment / StatefulSet / DaemonSet / Job / CronJob 列表·详情·YAML 编辑·删除
- **配置与密钥** — ConfigMap / Secret（Opaque/TLS/Registry）
- **存储** — PersistentVolume / PersistentVolumeClaim / StorageClass
- **YAML 在线编辑器** — Monaco Editor 语法高亮与校验
- **批量创建 Drawer** — 通过 YAML 一键创建多种资源

### 🔄 版本历史管理
- 每次修改自动生成版本快照
- 保留最近 N 个版本（默认 5，可配置）
- **左右对照 Diff** 展示两个版本 YAML 差异
- 一键**回滚**（生成新版本记录，非破坏性）

### 💾 备份与恢复
- **单对象备份** 与 **命名空间级批量备份** 两种模式
- 多存储后端：本地磁盘 / **S3** / **NFS** （插件化实现）
- 支持 `ResourceSelector` 按 Kind/Label 过滤
- 恢复模式选择：`overwrite` 覆盖 / `create-new` 新建（带 `-restored` 后缀）
- **WebSocket 实时进度** 显示

### 🎯 命名空间配额管理
- **ResourceQuota** 可视化列表 + 使用量进度条（CPU/Memory/Storage/PVC/对象计数）
- **LimitRange** Tab 表单创建/编辑（默认请求/限制）

### 🏷️ Helm 应用集成
- Release 列表（通过 k8s secrets 查询 `owner=helm`）
- 安装/升级（Helm CLI + YAML values 编辑器）
- 卸载 / **修订历史 / 回滚** 到任意修订版本
- Helm 独立权限点（`helm:view / install / uninstall / rollback`）

### 🛡️ RBAC 与安全
- JWT 认证 + 刷新令牌
- **细粒度权限点** — `cluster:list / resource:create / backup:restore / helm:install ...`（共 30+）
- **内置角色** — `platform-admin` / `cluster-admin` / `namespace-developer` / `platform-viewer`
- 按钮级权限控制（前端 `usePermission` Hook + 后端 `RequirePermission` 中间件）

### 📝 操作审计
- 全量记录（用户/集群/命名空间/资源/动作/请求体/响应码/耗时/客户端 IP / UA）
- 多维筛选（用户/集群/命名空间/资源类型/操作类型/时间范围）
- 分页 + 详情模态框

### ⚡ WebSocket 实时通道
- **Pod 日志流** — 自动滚动 + follow 模式 + 可选 tailLines
- **任务进度** — 备份/恢复实时百分比（80% 对象处理 + 20% 存储完成）
- **集群事件** — 实时推送集群告警与事件
- 7 种连接状态（idle/connecting/open/closing/closed/reconnecting/error）

### 🔌 插件化架构
- 对象类型插件（新增 GVK 支持）
- 备份目标插件（Local/S3/NFS 已实现）
- 认证 / 审计 / 通知 / KMS 插件接口

---

## Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                        Browser (React 18)                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────────┐  │
│  │ 资源管理 │ │ 备份中心 │ │ Helm 商城│ │ 集群/RBAC/配额/审计  │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └──────────┬───────────┘  │
│       ▼             ▼            ▼                   ▼              │
│  ┌─────────────────────────────┐      ┌─────────────────────────┐  │
│  │        RTK Query / Axios    │      │  Reconnecting WebSocket │  │
│  │      (HTTP /api/v1)         │      │  (3 channels ring buf)  │  │
│  └─────────────┬───────────────┘      └────────────┬────────────┘  │
└────────────────┼───────────────────────────────────┼─────────────────┘
                 │ :3000 (Vite dev) / :80            │
                 ▼                                   ▼
┌────────────────────────────────────────────────────────────────────┐
│                     API Server (Go 1.25, Gin)                       │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────────┐ │
│  │ Auth / RBAC │  │ REST Handlers│  │  WS Hub (Gorilla WebSocket)│ │
│  │ JWT + Scope │  │ CRUD / Helm  │  │  task_progress/cluster_   │ │
│  │ middleware  │  │ / Backup API │  │  events/pod_logs dispatcher│ │
│  └──────┬──────┘  └──────┬───────┘  └──────────┬─────────────────┘ │
│         │                │                      ▼                   │
│  ┌──────▼────────────────▼────────────┐   ┌─────────┐  ┌─────────┐  │
│  │      Business Modules (Interface)  │   │  Redis  │  │  Redis  │  │
│  │ ClusterMgr/ResourceMgr/VersionMgr  │   │  Cache  │  │ Pub/Sub │◄─┤
│  │ BackupMgr/HelmMgr/AuditExporter    │   │ Queue   │  │  ↕ WS   │  │
│  └───────────────┬───────────────────┘   └────┬────┘  └─────────┘  │
└──────────────────┼────────────────────────────┼────────────────────┘
                   │                            │
                   ▼                            ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   Task Worker (独立进程，可水平扩展)                   │
│   ┌───────────────┐   ┌──────────────┐   ┌──────────────────────┐  │
│   │ Backup Worker │   │ Informer Sync│   │ Cluster Health Worker│  │
│   │ (Redis Stream)│   │  (Lazy List) │   │  (Periodic Ping)     │  │
│   └───────────────┘   └──────────────┘   └──────────────────────┘  │
└───────────────┬──────────────────────┬──────────────────────────────┘
                │                      │
                ▼                      ▼
       ┌────────────────┐      ┌───────────────┐
       │  PostgreSQL    │      │   K8S Clusters│  1..N via Kubeconfig
       │ (Meta + Audit) │      │ (Client-go +  │  with DynamicClient
       └────────────────┘      │  Informer)    │
                               └───────────────┘
```

> 详见 [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)

---

## Quick Start

### 前置条件

| 组件 | 版本要求 |
|------|----------|
| Go   | 1.25.5 |
| Node.js | 18+（推荐 20 LTS） |
| npm / pnpm | 任一 |
| PostgreSQL | 13+ |
| Redis | 6.0+ |
| make（Windows 可选 Git Bash / WSL） | 4.0+ |

### 一键开发环境

```bash
# 1. 安装依赖
make deps

# 2. 初始化配置（从示例生成 backend/configs/config.yaml）
make config
#  → 修改 DB/Redis/JWT 等真实值（至少要能连上本地 PG/Redis）

# 3. 一键启动（并行 3 进程）
make dev
#  前端     → http://localhost:3000
#  API      → http://localhost:8080
#  Worker   → http://localhost:8081/healthz
```

默认管理员账号：请在首次启动后，查看 Seed 日志获取初始密码，或直接改 seed.go 中的内置账号。

### 生产构建

```bash
make build
#  后端: backend/bin/api-server / backend/bin/task-worker
#  前端: frontend/dist/

# 运行生产二进制
make run-prod
```

---

## Configuration

所有配置通过环境变量前缀 `K8SPLATFORM_` 可覆盖 YAML。

| 模块 | 关键字段 | 说明 |
|------|----------|------|
| `server` | `port`/`worker_port` | API:8080 / Worker:8081 |
| `database` | `host/port/username/password/dbname/sslmode` | PostgreSQL |
| `redis` | `host/port/password/db` | 缓存 / 限流 / 任务队列 / PubSub |
| `auth` | `jwt_secret` / `access_token_ttl` | JWT 认证 |
| `kms` | `provider` / `local_aes_key_hex` | kubeconfig 加解密密钥（**生产务必改**） |
| `backup.default_storage` | `local` / `s3` / `nfs` | 默认备份后端 |
| `security.cors_allow_origins` | `["http://localhost:3000"]` | 生产需替换为实际域名 |
| `log.level` | `debug/info/warn/error` | 日志级别 |

完整示例见 [backend/configs/config.example.yaml](./backend/configs/config.example.yaml)。

---

## Deployment

### Kubernetes (推荐)

Helm Chart 正在开发中。临时部署参考：

```yaml
# 两个 Deployment + Service：api-server / task-worker
# 共享一个 ConfigMap（config.yaml）+ Secret（JWT/KMS key）
# frontend 打包成静态文件由 nginx Ingress 托管
```

### Docker Compose

> TODO：提供 `deploy/compose.yaml` 一键起 PG+Redis+API+Worker+Frontend。

### Systemd

```ini
# /etc/systemd/system/k8s-platform-api.service
[Service]
WorkingDirectory=/opt/k8s-platform
ExecStart=/opt/k8s-platform/bin/api-server
Restart=always
```

---

## Development

```bash
make help              # 所有目标一览

# 代码质量
make fmt               # go fmt ./...
make vet               # go vet ./...
make test              # go test -race + eslint
make test-backend      # 仅后端单元测试（含竞态）
make lint-frontend     # 仅前端 eslint

# 清理
make clean             # bin/ dist/ .vite cache
```

### 分支策略（GitHub Flow）
- `main` — 稳定发布分支，必须通过 CI（编译+测试）
- 功能分支 — `feat/<模块>/<描述>` / `fix/<issue>-<描述>`
- 所有代码必须 PR + 至少 1 个 Approve 方可合并

### Commit 规范（Conventional Commits）
```
feat(helm): add rollback drawer
fix(backup): restore progress stuck at 80%
docs(readme): add architecture section
refactor(cluster): extract GetRestConfig to manager
test(storage): add NFS integration tests
chore: bump go 1.25.5
```

### 本地调试
- **VSCode launch 配置** — 前端 `npm run dev`；后端分别调试 `cmd/api-server` 和 `cmd/task-worker`
- **前端代理** — Vite 已配置 `/api` → `http://localhost:8080`（含 WS），无需 CORS 额外配置

---

## Contributing

欢迎贡献！请先阅读：

- [CONTRIBUTING.md](./CONTRIBUTING.md) — 贡献流程、代码规范、提交流程
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) — 行为准则
- [ROADMAP.md](./ROADMAP.md) — 功能路线图与「想实现什么」

**起步方式**：
1. Fork 本仓库 → Clone 到本地
2. 选一个 [good-first-issue] 或从 TODO 列表挑一项
3. 开 PR → CI 通过 → Review → Merge

---

## Community

| 方式 | 链接 |
|------|------|
| Issue | [GitHub Issues](https://github.com/Mokaz111/k8s-platform/issues) |
| Discussion | [GitHub Discussions](https://github.com/Mokaz111/k8s-platform/discussions) |
| 安全披露 | 见 [SECURITY.md](./SECURITY.md) |

### 采用者

在 [ADOPTERS.md](./ADOPTERS.md) 中列出贵组织来支持项目 🌟

### 维护者

见 [MAINTAINERS.md](./MAINTAINERS.md)

---

## License

本项目基于 [Apache License 2.0](./LICENSE) 开源。
