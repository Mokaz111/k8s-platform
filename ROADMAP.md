# K8S Platform 路线图（ROADMAP）

> 本文件记录 K8S Platform 多集群控制台的**已交付里程碑**与**未来规划**。
> 版本号遵循 [Semantic Versioning 2.0](https://semver.org/lang/zh-CN/)，
> 但项目在 v1.0.0 GA 之前 API 不做兼容性保证。

状态图例：
- ✅ **已完成** — 代码已合入 `main` 并通过 CI
- 🚧 **进行中** — 有活跃的 Issue / PR
- 📝 **规划中** — 已立项，优先级排期
- 💡 **设想** — Idea 阶段，欢迎讨论 / RFC

---

## 版本总览

| 版本 | 代号 | 发布时间 | 主题 | 状态 |
|------|------|----------|------|------|
| v0.1.0 | MVP Core | 2026 Q1 | 集群+资源+备份 MVP 骨架 | ✅ |
| v0.2.0 | RBAC + Audit | 2026 Q2 | 前端 RBAC 接入 + 审计日志页 | ✅ |
| v0.3.0 | Storage + Batch | 2026 Q2 | NFS 存储 + 命名空间批量备份 + 编译验证 | ✅ |
| v0.4.0 | Modules P2–P5 | 2026 Q3 | 集群/资源/版本/备份前端修复 | ✅ |
| v0.5.0 | WS + Restore Enhance | 2026 Q3 | WebSocket 三通道 + 备份恢复增强 | ✅ |
| v0.6.0 | Quota + Helm | 2026 Q3 | 命名空间配额管理 + Helm 应用集成 | ✅ |
| v0.7.0 | Project Hygiene | 2026 Q3 | 开源文档集 + Makefile 工作流 | ✅ |
| v1.0.0 | GA Release | 2026 Q4 | 生产就绪 + Helm Chart + 文档站 | 📝 |
| v1.1.0 | Extensions | 2027 Q1 | 插件市场 + SSO/OIDC | 💡 |
| v2.0.0 | Multi-Tenant | 2027 H1 | 租户隔离 + 自助申请命名空间 | 💡 |

---

## ✅ 已完成里程碑

### v0.7.0 — Project Hygiene（2026-08-26）
开源社区合规化，补齐 CNCF 风格的文档与构建工具链。

- [x] 标准 `Makefile`：help/deps/config/build/dev/run-prod/test/fmt/vet/clean
- [x] `README.md`：功能总览 + ASCII 架构图 + 快速开始 + 部署 + 开发指南
- [x] `LICENSE`：Apache 2.0 完整文本
- [x] `CONTRIBUTING.md`：GitHub Flow + Conventional Commits + 代码规范 + DCO
- [x] `CODE_OF_CONDUCT.md`：Contributor Covenant v2.1（映射 CNCF CoC）
- [x] `SECURITY.md`：负责任披露 + Embargo 政策 + CVSS 分级 + 威胁模型
- [x] `ROADMAP.md`：版本总览 + 已完成 + 规划中
- [x] `docs/ARCHITECTURE.md`：模块/数据流/部署拓扑
- [x] `MAINTAINERS.md` + `ADOPTERS.md`：标准模板
- [x] 编译验证：`go build ./...` / `tsc --noEmit` / `vite build`

### v0.6.0 — Quota + Helm（2026-08-25）
补齐命名空间治理与应用商店能力，后端引入 Helm Manager。

**方向 A — 备份恢复增强**
- [x] 恢复 Modal 新增恢复模式选择器：`overwrite` 覆盖 / `create-new` 新建（带后缀）
- [x] 备份列表区分任务类型：`backup_type=restore` 行显示「恢复」Tag
- [x] 恢复任务实时进度：恢复提交后以 taskId 接入 `useBackupProgress`

**方向 C — 命名空间配额管理**
- [x] `/resources/quota` 页面：ResourceQuota 列表 + CPU/Memory/Storage 使用量进度条
- [x] LimitRange Tab：表单创建/编辑（Container/Pod/PVC 三级默认请求/限制）
- [x] 资源概览：Tab 中合并展示

**方向 D — Helm 集成**
- [x] 后端 `helm.Manager`：k8s secrets 查询 `owner=helm` 列出 releases；helm CLI 执行 install/upgrade/uninstall/rollback
- [x] 集群临时 kubeconfig：Manager 导出 `GetRestConfig` / `GetKubeConfig`，解密后写临时文件
- [x] API Handler/Routes：`/clusters/:code/helm/releases` 全套 REST
- [x] RBAC Seed：`helm:view / helm:install / helm:uninstall / helm:rollback` 权限点
- [x] 前端 Helm 应用页：ProTable 列表 + 安装 Drawer（Monaco YAML values）+ 详情/回滚 Drawer
- [x] 前端集成：`helmApi` reducer、`/helm/list` 路由、BasicLayout 菜单组 + 权限映射

### v0.5.0 — WS + Restore Enhance（2026-08-24）
实时通道搭建，备份恢复体验闭环。

- [x] Pod 日志流：`PodLogsViewer` 组件（auto-scroll / follow / tailLines=500）
- [x] 任务进度：Backup Worker 通过 Redis PubSub → WSHub → 前端 `useBackupProgress`
- [x] 集群事件：独立通道，环形缓冲 `cluster_event[200]`
- [x] 7 状态连接指示：idle / connecting / open / closing / closed / reconnecting / error
- [x] 重连策略：Reconnecting-WebSocket 指数退避（1s→30s）
- [x] 认证：`?token=<JWT>` query 参数

### v0.4.0 — Modules P2–P5（2026-08-24）
前端细节打磨：TS 隐式 any 修复 + YAML 创建 + 参数调整 + 恢复 Hooks。

- [x] 集群管理页：TypeScript 严格模式修复
- [x] 资源对象管理：YAML 创建 Drawer（Monaco）
- [x] 版本管理：服务参数 path → query
- [x] 备份中心：下载 Hook + 按钮级权限控制（`backup:restore / backup:delete`）

### v0.3.0 — Storage + Batch（2026-08-24）
NFS 存储插件落地 + 命名空间批量备份 + 前端严格编译。

- [x] NFS Storage Plugin：`Upload/Download/Delete/List` 全量实现
  - 路径穿越保护
  - mount point 强校验（填了 `server` 就必须已挂载）
  - 开发降级模式（仅填 `base_path` 当本地目录）
- [x] 备份模式：`single_object` + `namespace_batch`（支持 kinds + labels 过滤）
- [x] 进度计算：80% 对象处理 + 20% 存储完成
- [x] 前端备份创建页：存储类型切换（Local/NFS）+ 模式切换
- [x] tsc --noEmit + vite build 0 错误

### v0.2.0 — RBAC + Audit（2026-08-23）
RBAC 前端对接，审计日志页上线。

- [x] Envelope 解包 + RBAC Service + 用户权限点 + `usePermission` Hook
- [x] 审计列表页：多维筛选 + 分页 + 详情 Modal
- [x] 用户管理：CRUD / 状态切换 / 重置密码 / 角色绑定 Drawer
- [x] 角色管理：CRUD / 状态切换 / 删除保护 / 权限树 Drawer
- [x] 路由/菜单/Store：权限对应到菜单与按钮

### v0.1.0 — MVP Core（2026-Q1）
最小可用版本，包含架构骨架与基础能力。

- [x] API Server / Task Worker 双进程架构
- [x] 集群导入（kubeconfig AES-GCM 加密存储）
- [x] Informer Manager（懒加载 + List 限流）
- [x] 资源对象 CRUD（Workload / Config / Storage）
- [x] 备份：单对象本地存储
- [x] 版本历史：每对象保留 5 个快照
- [x] PostgreSQL + Redis 基础设施
- [x] Gin + Redis Limiter + Audit 中间件

---

## 📝 v1.0.0 — GA Release（目标：2026 Q4）
生产就绪版本，完成**可运维性 + 稳定性 + 可部署性**三件套：

### 部署就绪
- [ ] **Helm Chart** — `deploy/helm/k8s-platform/`，支持：
  - `values.yaml` 配置所有模块
  - PostgreSQL / Redis 外部连接（生产）或 sub-chart 一键（开发）
  - HorizontalPodAutoscaler 模板（API/Worker 独立扩缩）
  - PodDisruptionBudget + PriorityClass
  - Ingress 示例（frontend/API/WS 路由分流）
  - 带 Prometheus ServiceMonitor
- [ ] **Dockerfile** — 多阶段构建（api-server/task-worker/frontend 各自最小镜像）
  - 基于 `gcr.io/distroless/static-debian12`（Go）+ `nginx:alpine-slim`（前端）
  - 镜像支持 UID=1000 非 root，可审计
- [ ] `deploy/compose.yaml` — Docker Compose 一键本地环境

### 可观测性
- [ ] `/metrics` 端点（Prometheus）：
  - API：QPS / 延迟分位 / 错误率（按路由）
  - Worker：任务成功/失败计数、队列深度
  - Informer：已同步 GVK 数量、Watch 断开重连次数
- [ ] 结构化日志 `trace_id` 全链路传递（WS/Worker/Redis/K8S Client）
- [ ] 健康检查细化：`/readyz` / `/livez` / `/healthz`

### 稳定性
- [ ] 单元测试覆盖率 ≥ **70%**（当前 ~30%）
- [ ] 集成测试套件：启动 kind 集群，跑完整的导入→备份→恢复→Helm 安装
- [ ] 后端熔断参数压力测试（连续失败 → 熔断 → half-open 恢复）
- [ ] 所有 WebSocket 通道 reconnection stress test（1000 次重连无泄漏）

### 文档
- [ ] `docs/` 文档站（Docusaurus / VitePress），中文为主 + 英文骨架
  - 安装指南（Docker Compose / Helm / 二进制）
  - 使用手册（每个模块图文教程）
  - 运维指南（升级、备份恢复、故障排查）
  - 插件开发教程（存储/认证/通知/KMS 四类插件模板）
- [ ] `CHANGELOG.md` 按版本自动生成（基于 Conventional Commits）

---

## 💡 v1.1.0+ — 扩展与生态（2027 Q1 起）

### 插件市场（插件生态平台化）
- [ ] **插件 Registry** — 发现、安装、启用、卸载第三方插件
- [ ] **插件 SDK** — `plugins/sdk/` 脚手架 + 示例插件（5 个）
- [ ] 内置插件包：
  - OIDC / SSO 认证（OIDC / LDAP / SAML）
  - 钉钉 / 企业微信 / 飞书通知 Webhook
  - Harbor / 阿里云 ACR 镜像扫描集成
  - 阿里云 OSS / 腾讯云 COS / MinIO S3 备份后端
  - Vault KMS（替换本地 AES）

### 多集群高级能力
- [ ] **联邦调度**（Karmada / Clusternet 集成）：跨集群分发 Deployment
- [ ] **多集群视角的 Topology 图**：节点→Pod→Service→Ingress 可视化
- [ ] **Policy Engine 集成**：Kyverno / OPA Gatekeeper 策略分发与合规报表

---

## 💡 v2.0.0 — 多租户（2027 H1）

- [ ] **租户（Organization/Team）模型**：平台级多租户隔离
  - 每个 Team 一组命名空间 / 一组集群绑定权限
  - 资源、备份、Helm Release 均归属 Team
- [ ] **命名空间自助申请**：开发者 → 提交申请表 → Team Admin / 平台审批
- [ ] **Quota 分级**：Platform Quota → Team Quota → Namespace Quota
- [ ] **计费/用量报表**：按 Team/Namespace 聚合 Pod 请求量、存储 GB、备份大小

---

## 想参与实现某一项？

欢迎以下形式（任选）：

1. 打开 [Discussions](https://github.com/Mokaz111/k8s-platform/discussions) 选对应条目回复
2. 对「📝 规划中 / 💡 设想」项提 RFC Issue（详细写 Use Case + 设计草稿）
3. 直接认领 Issue 标记 `good first issue` / `help wanted`

参见 [CONTRIBUTING.md](./CONTRIBUTING.md) 了解完整提交流程。
