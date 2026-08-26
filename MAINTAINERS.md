# 维护者（MAINTAINERS）

> 本文件定义 K8S Platform 项目的**维护者治理结构**、角色职责、
> 选举与除名流程。参考 CNCF 项目最佳实践（Kubernetes / Prometheus 模式）。
> 当前处于早期维护阶段，流程会随社区扩大而迭代。

---

## 1. 角色一览

| 角色 | 缩写 | 权限 | 数量 | 任职 |
|------|------|------|------|------|
| **BDFL / 项目负责人** | Lead | 拥有最终决定权；治理规则修改裁决；维护者提名/罢免 | 1 | 长期（选举产生）|
| **核心维护者（Core Maintainer）** | CM | 仓库合并权（Squash Merge）；Issue 分类；维护者评审 | ≥2 | 长期，季度评估 |
| **领域维护者（Domain Maintainer）** | DM | 领域 Approve 权；对应模块 Review；CI/CD 配置 | ≥1/模块 | 长期 |
| **评审者（Reviewer）** | R | 代码 Review（非强制 Approve）；good-first-issue 辅导 | 不限 | 6 个月自动续任，贡献不足降级 |
| **贡献者（Contributor）** | C | 提交 PR / Issue / Discussion 的任何人 | 不限 | 无限期（欢迎 👏）|

### 权限矩阵

| 权限 | Lead | CM | DM | R | C |
|------|:----:|:--:|:--:|:-:|:-:|
| 合并 PR 到 `main`（含 Squash Merge） | ✅ | ✅ | ❌ | ❌ | ❌ |
| Approve 所在模块 PR（强制 Required Approvals） | ✅ | ✅ | ✅ | ❌ | ❌ |
| 打 Release Tag / 发 GitHub Release | ✅ | ✅ | ❌ | ❌ | ❌ |
| 管理 GitHub 项目（Labels/Milestones/Projects）| ✅ | ✅ | ✅ | ❌ | ❌ |
| 管理 Branch Protection / CI Secrets | ✅ | ✅ | ❌ | ❌ | ❌ |
| 加入 Security Response Team（处理漏洞） | ✅ | ✅ | 视情况 | ❌ | ❌ |
| Review PR（无强制 Approve） | ✅ | ✅ | ✅ | ✅ | ✅ |
| 提交 PR / Issue | ✅ | ✅ | ✅ | ✅ | ✅ |

---

## 2. 当前维护者

### BDFL / 项目负责人

| GitHub | 职责范围 | 邮箱（安全举报用） | 时区 | 自何时 |
|--------|----------|--------------------|------|--------|
| **@Mokaz111** | 总体方向、架构决策、最终裁决 | _（请通过 GitHub Security Advisory 或邮件联系，见 SECURITY.md）_ | UTC+8 | 2026 |

### 核心维护者（Core Maintainers）

| GitHub | 主要领域 | 模块 | 自何时 |
|--------|----------|------|--------|
| @Mokaz111 | 全模块 | — | 2026 |

> **招募中**：我们欢迎长期贡献者晋升至 CM/DM，参见 §4 晋升流程。

### 领域维护者（Domain Maintainers）

| GitHub | 领域（Domain） | 代码范围 | 自何时 |
|--------|----------------|----------|--------|
| (虚位以待) | 后端 / ClusterMgr & ResourceMgr | `internal/cluster` `internal/resource` | — |
| (虚位以待) | 后端 / Backup & Storage Plugins | `internal/backup` `internal/plugins/storage` | — |
| (虚位以待) | 后端 / Helm & RBAC | `internal/helm` `internal/auth` `internal/models` | — |
| (虚位以待) | 后端 / WebSocket & Worker | `internal/websocket` `internal/worker` | — |
| (虚位以待) | 前端 / UI & ProComponents | `frontend/src/**/*.tsx` | — |
| (虚位以待) | SRE / CI-CD / 部署 / Helm Chart | `.github/` `deploy/` `Makefile` | — |
| (虚位以待) | 文档站 & 中英翻译 | `README.md` `docs/` `ADOPTERS.md` | — |

### 评审者（Reviewer）

| GitHub | 专注领域 | 自何时 |
|--------|----------|--------|
| (虚位以待) | — | — |

### 荣誉 / 前维护者（Emeritus）

| GitHub | 任期 | 贡献摘要 |
|--------|------|----------|
| (暂无) | — | — |

---

## 3. 职责

### 核心维护者（Core Maintainer）每周最少投入
1. 至少处理 5 个 Issue（分类、回复、指派）
2. 至少 Review 3 个 PR（24h 内响应首评，除非注明 PTO）
3. 维护里程碑（Milestone）与项目看板（Project Board）
4. 每月至少 1 次合并 PR 操作（保证 Release 流水线有人接管）
5. 加入 Security Response Team：响应安全漏洞邮件（48h 内 ACK）
6. **遵守维护者 CoC**：对所有 Review 对象平等友好，不进行人格攻击

### 领域维护者（Domain Maintainer）
1. 对应领域 PR 在 48h 内被 DM Review（至少 comment）
2. 至少每月贡献 1 条代码或 Review 10 条 PR（否则自动降级为 Reviewer）
3. 模块架构决策：撰写模块 RFC、决定接口签名、批准插件实现

### 评审者（Reviewer）
1. 至少每季度有 1 次 Review 行为
2. 引导新手：`good-first-issue` 标记的 PR 优先响应
3. Review 时给出建设性意见（不仅说"这里不对"，而要说"建议怎么做"）

---

## 4. 晋升流程（Contributor → Reviewer → DM → CM）

### 4.1 Contributor → Reviewer
- **门槛**：最近 90 天内 ≥ 5 个被合并的 PR（含修复或 feature，文档也算）
- **流程**：
  1. 任意 CM/DM 在 #maintainers 内部频道提名（附 PR 清单）
  2. **过半数 CM + 领域 DM** 同意即可通过
  3. 通过后：GitHub 添加到 Reviewers 团队；README 致谢
  4. 若 6 个月无 Review 行为 → 自动 inactive（可随时恢复）

### 4.2 Reviewer → Domain Maintainer
- **门槛**：
  - 至少 3 个月连续 Reviewer
  - 对目标模块有 ≥ 10 个 merged PR（**原创代码**，非文档）
  - 至少 Review 过 15 个对应模块 PR（且被作者采纳率 ≥ 70%）
  - 无 CoC 违规记录
- **流程**：CM 提名 → 全体 CM 匿名投票 → **75% 通过**
- **通过后**：添加 CODEOWNERS 权限、加入领域邮件列表、MAINTAINERS.md 更新

### 4.3 DM → Core Maintainer
- **门槛**：
  - 至少 6 个月连续 DM
  - 合并 PR 数 ≥ 30，Review 数 ≥ 60
  - 主导过至少 1 个重要 feature（可对照 ROADMAP 里程碑）
  - 无安全披露违规（如泄漏漏洞）
- **流程**：BDFL 或 ≥2 名 CM 提名 → 全体 CM 匿名投票 → **2/3 多数 + BDFL 同意**

### 4.4 BDFL 更换
- 触发条件：BDFL 主动辞职 / 连续 6 个月无法履职 / CM 全票不信任
- 选举：全体 CM + 至少一半 DM 参与投票（每人 1 票），简单多数获胜
- 选举结果提交 GitHub 仓库所有权更新（组织 Transfer / Owner 调整）

---

## 5. 降级 / 除名流程

### 非活跃降级（自动）
- Reviewer：6 个月无 Review → inactive（保留名单 + 可随时恢复）
- DM：连续 2 个月未完成 §3 职责 → CM 投票过半数降为 Reviewer
- CM：连续 3 个月未完成 §3 职责 → 全体 CM 2/3 投票降级

### 违规除名（强制）
触发情形：
- **严重** CoC 违规（人身攻击、骚扰、人肉搜索）
- 故意泄漏安全漏洞 Embargo 内容
- 有证据的提交/评审舞弊（如伪造 review、合入关联方恶意代码）
- 法律判决导致项目声誉严重受损

流程：
1. 至少 2 名 CM 提交书面指控 + 证据（私密 Google Doc / GitHub Security Advisory）
2. 被指控人有 14 天时间陈述申辩
3. 全体 CM（除当事人）匿名投票：**2/3 多数 → 立即除名；过半数 → 30 天察看**
4. 投票结果与摘要理由向全体维护者邮件通知（不公开证据细节）
5. 当事人可在 30 天后提出申诉

---

## 6. 决策流程

| 决策类型 | 审批方 | 投票规则 | 周期 |
|----------|--------|----------|------|
| 合并 PR（除重大变更） | 1 CM approve + CI pass | — | 持续 |
| 小版本发布（v0.6 → v0.7） | ≥2 CM 同意 Tag | 简单多数 | 按需 |
| 主版本/破坏性变更（v0 → v1） | CM 全员 + BDFL | 2/3 多数 + BDFL 同意 | 规划中 |
| 新维护者晋升 | CM（对应层级） | 按 §4 | 随提名 |
| 维护者除名 | CM（除当事人） | 2/3 多数 | 随事件 |
| 治理规则修改（本文件） | 全体 CM + BDFL | 全票通过（Veto 制） | 季度 |
| 进入 CNCF Sandbox / Incubation | 维护者公投 | ≥80% 通过 | 重大里程碑 |

---

## 7. 联系方式

### 公开沟通（优先）
- **GitHub Issue/PR/Discussions**：技术问题、功能讨论、设计 RFC
- **Code Review**：直接在行内评论

### 私密沟通
- **安全漏洞举报**：参见 [SECURITY.md](./SECURITY.md) 邮件流程
- **维护者内部沟通**：维护者邮件列表 / 私有频道（加入 CM/DM 后自动邀请）
- **举报维护者本人违规**：直接邮件举报至 BDFL（GitHub Profile 中的联系方式）
  若举报对象即 BDFL，则抄送所有 CM，由 CM 集体处理

---

## 8. 历史

| 版本 | 日期 | 变更 |
|------|------|------|
| v1.0 | 2026-08-26 | 初始版本：确立角色、职责、晋升/降级流程 |

> 本文件的所有变更以 PR 形式提交，修改规则见 §6「治理规则修改」。
