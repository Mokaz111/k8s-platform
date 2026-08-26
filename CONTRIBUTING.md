# Contributing Guide

首先感谢你对 **K8S Platform** 项目的兴趣！本指南涵盖贡献流程、分支策略、
代码规范和常见问题。如果你发现有遗漏，欢迎提 Issue 或直接改进本文件 💖

---

## 目录

- [行为准则](#行为准则)
- [我有问题，应该怎么提问？](#我有问题应该怎么提问)
- [我可以贡献什么？](#我可以贡献什么)
- [贡献流程（Pull Request）](#贡献流程pull-request)
- [分支策略](#分支策略)
- [Commit 规范](#commit-规范)
- [代码规范](#代码规范)
- [本地开发环境](#本地开发环境)
- [测试与 CI](#测试与-ci)
- [签署 DCO / CLA](#签署-dco--cla)
- [社区资源](#社区资源)

---

## 行为准则

参与本项目前，请阅读并遵守 [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md)。
任何形式的骚扰、侮辱、人身攻击都会被立即处理。

## 我有问题，应该怎么提问？

- **Bug 报告** → [GitHub Issues](https://github.com/Mokaz111/k8s-platform/issues/new?template=bug-report.yaml)
  - 必带信息：版本、环境、复现步骤、期望/实际表现、日志（脱敏）
- **功能建议** → [GitHub Discussions / Ideas](https://github.com/Mokaz111/k8s-platform/discussions/new?category=ideas)
- **使用问题** → [GitHub Discussions / Q&A](https://github.com/Mokaz111/k8s-platform/discussions/new?category=q-a)
- **安全问题** → **不要公开发 Issue**，直接邮件联系维护者（见 [SECURITY.md](./SECURITY.md)）

## 我可以贡献什么？

### 🥇 入门友好（good first issue）
- 文档错误 / 拼写修正
- 注释补充、日志信息优化
- 前端 i18n 未翻译文案
- 新增单元测试（优先 coverage 缺口的模块）

### 🥈 常规贡献
- 修复 Issue 中标记为 `bug` / `kind/bug` 的问题
- 实现 `kind/feature` / `kind/enhancement` 的新功能
- 性能优化、错误处理完善
- 插件化新后端（存储/认证/通知/KMS）

### 🥇 大型变更
**请先开 Issue 或 RFC 讨论再编码**，避免无效工作：
- 架构级重构
- 破坏性 API 变更
- 依赖重大升级（如 Go 主版本、React 主版本）
- 新增官方插件 / 核心模块

---

## 贡献流程（Pull Request）

```
┌─────────────┐   ┌──────────────┐   ┌───────────┐   ┌─────────┐   ┌────────────┐
│ 1. Fork 仓库 │ → │ 2. 本地开发  │ → │ 3. 跑测试 │ → │ 4. PR  │ → │ 5. Review  │
└─────────────┘   └──────────────┘   └───────────┘   └─────────┘   └────┬───────┘
                                                                         ▼
                                                                  ┌───────────┐
                                                                  │ 6. Merge  │
                                                                  └───────────┘
```

### Step 1：Fork & 分支
```bash
# ① Fork 到你自己的 GitHub 账户
gh repo fork Mokaz111/k8s-platform --clone=true
cd k8s-platform

# ② 从最新 main 切功能分支
git fetch origin
git checkout -b feat/<module>/<short-desc> origin/main
```

### Step 2：本地开发 + 验证
```bash
make deps         # 首次
make config       # 初始化配置
make dev          # 启动三进程验证功能

make fmt vet      # 格式化 + vet
make test         # 跑全部测试
```

### Step 3：提交 & PR
```bash
git commit -a -m "feat(helm): add rollback drawer"   # 遵循 Conventional Commits
git push origin feat/<module>/<short-desc>

# 浏览器打开 PR 链接，填写 PR 模板（必需：关联 Issue、变更描述、截图/验证）
```

### Step 4：Review & 修改
- 至少需要 **1 位 Maintainer Approve**（大变更需 2 位）
- Review 意见用 `fixup!` / `squash!` commit 回复，**不要 rebase 历史**
- CI 通过（编译 + 测试 + lint）才会被考虑合并
- 最终合并时由 Maintainer 使用 **Squash Merge**

---

## 分支策略

本项目采用 **GitHub Flow**：

| 分支 | 用途 | 规则 |
|------|------|------|
| `main` | 稳定主干，任何时候可发布 | 强制 PR，保护分支，禁止直接 push |
| `feat/<模块>/<描述>` | 新功能 | 来自 `main`，回 `main` |
| `fix/<issue>-<描述>` | Bug 修复 | 来自 `main`，回 `main`；紧急修复可 cherry-pick |
| `docs/<描述>` | 文档变更 | 同上 |
| `chore/<描述>` | 依赖升级 / 脚手架 / CI 变更 | 同上 |
| `release/vX.Y` | 发布分支（可选） | 仅用于候选版本冻结，不用于日常开发 |

> **禁止长期开发分支**：功能过大请拆分多个可独立合并的小 PR。

---

## Commit 规范

所有 commit 遵循 **[Conventional Commits](https://www.conventionalcommits.org/zh-hans/v1.0.0/)**，
这也是 Release Notes、自动版本号计算的基础。

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type 必填

| Type | 含义 |
|------|------|
| `feat` | 新功能（用户可见） → 对应 MINOR 版本 |
| `fix` | Bug 修复 → 对应 PATCH 版本 |
| `docs` | 文档变更（README、docs/ 目录） |
| `style` | 代码格式、空格、缺失分号等（无代码逻辑变更） |
| `refactor` | 重构（非新增功能、非修复 bug） |
| `perf` | 性能优化 |
| `test` | 新增或修正测试 |
| `build` | 构建系统、外部依赖变更（npm / go.mod / dockerfile） |
| `ci` | CI 配置变更（GitHub Actions、Argo 流水线） |
| `chore` | 杂项（如 `.gitignore`、Makefile 等脚手架） |
| `revert` | 回滚之前的 commit（`revert: <hash>`） |

### Scope（推荐）
对应模块：`helm / backup / cluster / resource / version / quota / rbac / audit / websocket /
plugin / api / ui / infra / deps`

### Subject（必填）
- **50 字以内**，动词现在时、祈使句开头
- **不**以点号 `.` 结尾
- 中文：用「新增 / 修复 / 优化 / 重构」开头；英文：用 `Add` / `Fix` / `Improve` 开头

### Body（详细说明）
- 72 字符换行
- 回答「为什么这样做 → 怎么做 → 影响范围」
- 若是破坏性变更，开头加 **`BREAKING CHANGE:`**

### Footer
- `Fixes #123` — 自动关联 Issue
- `Closes #456, #789` — 合并时自动关闭
- `Ref: <discussion 链接>` — 引用讨论

**示例**：
```
feat(backup): support NFS mount point validation

Add mount point existence check for NFS storage plugin; reject
configuration when server is specified but mount_point is not
actually mounted. Also add path traversal protection for
Download/Delete/List.

Fixes #234
Ref: https://github.com/Mokaz111/k8s-platform/discussions/42
```

---

## 代码规范

### Go（后端）
- 遵循 [Effective Go](https://go.dev/doc/effective_go) + [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- 提交前必须：`make fmt vet`
- 错误包装：`fmt.Errorf("xxx failed: %w", err)` 用 `%w`，**不要丢弃原始错误**
- 日志：使用 `pkg/logger`（zap 封装），避免 `fmt.Println`
- 所有导出类型/函数**必须有 doc comment**
- 公共错误用 `pkg/errcode`，禁止裸 `errors.New`

### TypeScript / React（前端）
- 遵循 [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)
- 严格模式：`tsc --noEmit` 必须 0 错误
- 组件拆分：Container（逻辑）/ Presentational（UI）分离
- 状态：全局状态走 Redux Toolkit / RTK Query，局部状态用 `useState` / `useReducer`
- 不要用 `any`，用 `unknown` + 类型守卫，或泛型
- Ant Design Pro 组件优先，避免重复造轮子

### 通用
- **优先写注释解释 WHY**，WHAT 让代码自解释
- **先设计接口（interface）再实现**；结构体字段考虑零值
- **幂等**：所有有副作用的接口必须考虑重试场景
- **安全**：
  - 路径拼接必须用 `filepath.Join` + 校验（防止穿越）
  - kubeconfig / 密钥不得出现在日志
  - 审计日志 request body 截断防止过大

---

## 本地开发环境

### 启动依赖（PostgreSQL + Redis）
```bash
# 方法 1：Docker
docker run -d --name k8s-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:15
docker run -d --name k8s-redis -p 6379:6379 redis:7

# 方法 2：Make + Docker Compose（TODO：提供 deploy/compose.yaml）
```

### 启动项目
```bash
make deps && make config && make dev
# 前端 → http://localhost:3000
# API  → http://localhost:8080/healthz
```

### 调试
- **后端**：VSCode 启动 `cmd/api-server` 和 `cmd/task-worker`，Go Debug
- **前端**：Vite 原生 HMR，Redux DevTools
- **WS 调试**：Chrome DevTools → Network → WS 查看三通道消息

---

## 测试与 CI

### 本地测试
```bash
make test             # 后端 -race + 前端 eslint
make test-backend     # 只跑后端（推荐开发中频繁跑）
make lint-frontend    # 只跑前端
```

### 单元测试要求
- Go：新功能必须有单元测试，**覆盖率新增代码 ≥ 60%**
  - Table-driven tests 优先（参考 `internal/models/audit_log_test.go`）
  - 外部依赖（Redis/DB/K8S）可 mock（参见 `gomock` / `testify`）
- TS：复杂逻辑（工具函数、Reducer）必须有测试；UI 组件以手动验收为主

### CI（GitHub Actions）
所有 PR 自动触发：
1. `go build ./...` + `tsc --noEmit` + `vite build`（**必须通过**）
2. `go test -race ./...`
3. `go vet ./...`
4. `npm run lint`
5. 许可头检查（TODO：`addlicense`）

---

## 签署 DCO / CLA

本项目目前采用 **DCO（Developer Certificate of Origin）**，即你在 commit 时
加 `Signed-off-by: Name <email>` 行即可表示你同意：

```
git commit -s -m "feat(xxx): ..."
```

> 含义：你是这段代码的作者，或你有权以同样开源协议贡献此代码。
> 详见 <https://developercertificate.org/>

若后续项目进入 CNCF Sandbox，将切换为 CNCF CLA（一次性签署）。

---

## 社区资源

| 资源 | 链接 |
|------|------|
| 设计文档 | [k8s-platform-system-design.md](./k8s-platform-system-design.md) |
| 架构文档 | [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) |
| 路线图 | [ROADMAP.md](./ROADMAP.md) |
| 维护者 | [MAINTAINERS.md](./MAINTAINERS.md) |
| 行为准则 | [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) |
| 安全披露 | [SECURITY.md](./SECURITY.md) |

---

**最后**：本指南永远可以改进。如果有任何建议，请直接提 Issue 或 PR 修正，
这也是贡献的一种形式 ✨
